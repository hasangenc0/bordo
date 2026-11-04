// Package sandbox runs shell commands against a throwaway copy of a project's workspace.
//
// It is meant to run in its own container, on a network that cannot reach the control plane,
// with the agent's workspace mounted read-only. Each request copies the working tree (without
// .git) to a scratch directory, runs the command there as an unprivileged user with a scrubbed
// environment and a hard timeout, and deletes the copy. Nothing the command does persists or
// reaches the real repository: the agent edits files through code_edit_file, and uses this only
// to build, test and inspect.
//
// Teardown follows what multi-tenant runners such as Piston and isolate do. Each concurrent run gets
// its own slot with a dedicated UID, so when the command's shell exits everything still running as
// that UID (background jobs, nohup'd or setsid'd daemons, build-tool daemons) is stopped and killed,
// and files and System V IPC objects it left behind are removed. The scratch copy is deleted as that UID,
// because the server runs as root without CAP_DAC_OVERRIDE and cannot delete files owned by another
// user. Leftover copies from a previous process are swept on startup.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const (
	defaultTimeout = 120 * time.Second
	maxTimeout     = 600 * time.Second
	maxOutput      = 64 << 10
	maxCopyBytes   = 1 << 30
)

var projectRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Config configures a sandbox server.
type Config struct {
	WorkspaceRoot string // read-only mount holding one directory per project
	WorkRoot      string // scratch space for the per-request copies
	CacheDir      string // persistent dependency caches (module/npm downloads), one directory per slot
	// Commands in slot i run as UID+i:GID, a user nothing else in the container uses, so teardown can
	// kill every process of that user without touching other runs. DropPrivileges is off only in
	// tests, which do not run as root.
	DropPrivileges bool
	UID, GID       uint32
	MaxParallel    int
	// CacheMaxBytes caps each slot's dependency cache on disk; above it the janitor deletes the
	// largest tool caches until it fits. 0 means no cap.
	CacheMaxBytes int64
	// SharedTmp lists world-writable directories outside the scratch copy (tmpfs mounts) where
	// commands may leave files; teardown removes the slot user's files from them.
	SharedTmp []string
}

// Request is the body of POST /exec.
type Request struct {
	Project        string `json:"project"`
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// Result is the response of POST /exec.
type Result struct {
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	Truncated  bool   `json:"truncated,omitempty"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// Server is the sandbox HTTP handler.
type Server struct {
	cfg   Config
	slots chan int // free slot indexes; a slot is a UID and a cache directory
	// busy[i] is held by the run using slot i, and by the janitor while it trims that slot's cache.
	busy []sync.Mutex

	mu     sync.Mutex
	active map[string]bool // scratch directories of runs in progress
}

// New returns a sandbox Server.
func New(cfg Config) *Server {
	n := cfg.MaxParallel
	if n <= 0 {
		n = 2
	}
	s := &Server{cfg: cfg, slots: make(chan int, n), busy: make([]sync.Mutex, n), active: map[string]bool{}}
	for i := 0; i < n; i++ {
		s.slots <- i
	}
	return s
}

func (s *Server) slotUID(slot int) int { return int(s.cfg.UID) + slot }

// Prepare readies the server before it takes requests: it kills processes and deletes scratch
// copies left behind by a previous instance, and creates each slot's cache directory. Only a work
// root it cannot create is fatal; cache problems are logged and runs fall back to a cache inside
// their copy.
func (s *Server) Prepare() error {
	if err := os.MkdirAll(s.cfg.WorkRoot, 0o755); err != nil {
		return err
	}
	for i := 0; i < cap(s.slots); i++ {
		s.reap(s.slotUID(i), "")
	}
	s.sweep()
	if s.cfg.CacheDir != "" {
		if err := s.prepareCache(); err != nil {
			log.Printf("bordo-sandbox: dependency cache unavailable, runs will not share one: %v", err)
		}
	}
	return nil
}

func (s *Server) prepareCache() error {
	if err := os.MkdirAll(s.cfg.CacheDir, 0o755); err != nil {
		return err
	}
	// Caches used to be shared by all runs at the top of CacheDir; they are per slot now. Remove
	// them as their owner while the old owner still owns CacheDir too.
	for _, legacy := range []string{"go-build", "go", "npm", "pip"} {
		dir := filepath.Join(s.cfg.CacheDir, legacy)
		if _, err := os.Lstat(dir); err == nil {
			_ = s.removeTree(dir, ownerOf(dir))
		}
	}
	if s.cfg.DropPrivileges {
		// Older versions handed CacheDir to the unprivileged user. Root has no CAP_DAC_OVERRIDE, so
		// it must own CacheDir to create the slot directories in it.
		if err := os.Lchown(s.cfg.CacheDir, 0, 0); err != nil {
			return err
		}
		if err := os.Chmod(s.cfg.CacheDir, 0o755); err != nil {
			return err
		}
	}
	for i := 0; i < cap(s.slots); i++ {
		dir := filepath.Join(s.cfg.CacheDir, "slot-"+strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if s.cfg.DropPrivileges {
			if err := chownTreeIfNeeded(dir, s.slotUID(i), int(s.cfg.GID)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Janitor deletes scratch copies that no run owns every interval, and trims the dependency caches
// every cacheEvery, until ctx is done. Runs clean up after themselves; the sweep catches what a
// crash in the middle of one would leave.
func (s *Server) Janitor(ctx context.Context, interval, cacheEvery time.Duration) {
	s.TrimCaches()
	t := time.NewTicker(interval)
	defer t.Stop()
	lastTrim := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep()
			if time.Since(lastTrim) >= cacheEvery {
				s.TrimCaches()
				lastTrim = time.Now()
			}
		}
	}
}

// TrimCaches brings every idle slot's cache under CacheMaxBytes by deleting whole tool caches
// (go-build, npm, m2, ...), largest first. They are rebuilt on demand, so the cost of trimming is
// a slower next build. A slot in use is skipped until the next pass.
func (s *Server) TrimCaches() {
	if s.cfg.CacheDir == "" || s.cfg.CacheMaxBytes <= 0 {
		return
	}
	for i := range s.busy {
		if !s.busy[i].TryLock() {
			continue
		}
		s.trimCache(i)
		s.busy[i].Unlock()
	}
}

func (s *Server) trimCache(slot int) {
	dir := filepath.Join(s.cfg.CacheDir, "slot-"+strconv.Itoa(slot))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type sized struct {
		path string
		size int64
	}
	var parts []sized
	var total int64
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		n := diskUsage(p)
		parts = append(parts, sized{p, n})
		total += n
	}
	if total <= s.cfg.CacheMaxBytes {
		return
	}
	sort.Slice(parts, func(a, b int) bool { return parts[a].size > parts[b].size })
	for _, p := range parts {
		if total <= s.cfg.CacheMaxBytes {
			break
		}
		if err := s.removeTree(p.path, s.slotUID(slot)); err != nil {
			log.Printf("bordo-sandbox: trimming cache %s: %v", p.path, err)
			continue
		}
		log.Printf("bordo-sandbox: cache over %d MB, removed %s (%d MB)", s.cfg.CacheMaxBytes>>20, p.path, p.size>>20)
		total -= p.size
	}
}

// diskUsage is the space path takes on disk (allocated blocks, like du), not its apparent size.
func diskUsage(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			total += int64(st.Blocks) * 512
		} else {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (s *Server) sweep() {
	entries, err := os.ReadDir(s.cfg.WorkRoot)
	if err != nil {
		return
	}
	for _, e := range entries {
		dir := filepath.Join(s.cfg.WorkRoot, e.Name())
		s.mu.Lock()
		busy := s.active[dir]
		s.mu.Unlock()
		if busy {
			continue
		}
		if err := s.removeTree(dir, ownerOf(dir)); err != nil {
			log.Printf("bordo-sandbox: removing stale %s: %v", dir, err)
		}
	}
}

// Handler serves /healthz and POST /exec.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	mux.HandleFunc("/exec", s.handleExec)
	return mux
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req Request
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !projectRe.MatchString(req.Project) {
		writeErr(w, http.StatusBadRequest, "invalid project")
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		writeErr(w, http.StatusBadRequest, "command is required")
		return
	}
	timeout := defaultTimeout
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}
	res, err := s.Run(r.Context(), req.Project, req.Command, timeout)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Run waits for a free slot, copies the project's workspace, runs command in the copy and returns its
// output. Whatever the command started is killed and the copy deleted before Run returns.
func (s *Server) Run(ctx context.Context, project, command string, timeout time.Duration) (*Result, error) {
	src := filepath.Join(s.cfg.WorkspaceRoot, project)
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("no workspace for project %q: run code_checkout first", project)
	}
	var slot int
	select {
	case slot = <-s.slots:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.busy[slot].Lock() // waits only if the janitor is trimming this slot's cache
	uid := s.slotUID(slot)
	if err := os.MkdirAll(s.cfg.WorkRoot, 0o755); err != nil {
		s.busy[slot].Unlock()
		s.slots <- slot
		return nil, err
	}
	work := filepath.Join(s.cfg.WorkRoot, uuid.NewString())
	s.mu.Lock()
	s.active[work] = true
	s.mu.Unlock()
	defer func() {
		// Teardown must finish before the slot is reused, so the next run starts with no processes
		// of its user alive and nothing of the last run's on the tmpfs.
		s.reap(uid, work)
		s.mu.Lock()
		delete(s.active, work)
		s.mu.Unlock()
		s.busy[slot].Unlock()
		s.slots <- slot
	}()
	if err := copyTree(src, work, s.cfg.DropPrivileges, uid, int(s.cfg.GID)); err != nil {
		return nil, fmt.Errorf("preparing workspace copy: %w", err)
	}
	return s.execute(ctx, work, s.cacheDir(slot, work), uid, command, timeout)
}

func (s *Server) cacheDir(slot int, work string) string {
	if s.cfg.CacheDir == "" {
		return filepath.Join(work, ".cache")
	}
	dir := filepath.Join(s.cfg.CacheDir, "slot-"+strconv.Itoa(slot))
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return filepath.Join(work, ".cache")
	}
	return dir
}

func (s *Server) execute(ctx context.Context, work, cache string, uid int, command string, timeout time.Duration) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// No core dumps: they land in the copy, which lives on a memory-backed tmpfs.
	cmd := exec.CommandContext(ctx, "sh", "-c", "ulimit -c 0 2>/dev/null\n"+command)
	cmd.Dir = work
	cmd.Env = env(work, cache)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if s.cfg.DropPrivileges {
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: s.cfg.GID, Groups: []uint32{}}
	}
	// Kill the whole process group on timeout or cancellation, not just the shell.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

	// Hand the command a real pipe instead of an io.Writer, so Wait returns as soon as the shell
	// exits rather than when every background job that inherited stdout has closed it.
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = pw, pw
	out := &capBuffer{limit: maxOutput}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, err
	}
	pw.Close()
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(out, pr)
		close(copied)
	}()
	err = cmd.Wait()
	elapsed := time.Since(start)
	pgid := cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGKILL) // background jobs still in the group
	if s.cfg.DropPrivileges {
		s.killUser(uid) // and anything that left the group
	}
	select {
	case <-copied:
	case <-time.After(2 * time.Second):
		// Something outside our reach still holds the pipe; stop reading.
	}
	pr.Close()
	<-copied

	res := &Result{Output: out.String(), Truncated: out.truncated, DurationMS: elapsed.Milliseconds()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.TimedOut, res.ExitCode = true, -1
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return nil, err
	}
	return res, nil
}

// reap kills every process of uid, removes its System V IPC objects and its files in the shared
// tmpfs mounts, and deletes work (if set). Without privileges (tests) there is no dedicated user, so
// it only deletes work.
func (s *Server) reap(uid int, work string) {
	if s.cfg.DropPrivileges {
		s.killUser(uid)
		s.removeIPCOf(uid)
		for _, dir := range s.cfg.SharedTmp {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				path := filepath.Join(dir, e.Name())
				if ownerOf(path) == uid {
					_ = s.removeTree(path, uid)
				}
			}
		}
	}
	if work != "" {
		if err := s.removeTree(work, uid); err != nil {
			log.Printf("bordo-sandbox: removing %s: %v", work, err)
		}
	}
}

// removeIPCOf deletes uid's System V IPC objects, as uid, by re-running this binary.
func (s *Server) removeIPCOf(uid int) {
	objs := sysvIPCOwnedBy("/proc/sysvipc", uid)
	if len(objs) == 0 {
		return
	}
	self, err := os.Executable()
	if err != nil {
		log.Printf("bordo-sandbox: removing IPC objects of uid %d: %v", uid, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, append([]string{IPCRMCommand}, objs...)...)
	cmd.Env = []string{}
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: s.cfg.GID, Groups: []uint32{}}}
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("bordo-sandbox: removing IPC objects of uid %d: %v %s", uid, err, out)
	}
}

// killUser stops, then kills, every process running as uid, and rescans until none is left.
// Stopping everything first keeps a fork loop from replacing children faster than they die.
func (s *Server) killUser(uid int) {
	for i := 0; i < 20; i++ {
		pids := processesOf(uid)
		if len(pids) == 0 {
			return
		}
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGSTOP)
		}
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		time.Sleep(20 * time.Millisecond)
	}
	log.Printf("bordo-sandbox: processes of uid %d survived teardown", uid)
}

// removeTree deletes path. Its contents belong to the slot user, and root without CAP_DAC_OVERRIDE
// may not unlink another user's files, so the user empties it first (making read-only directories,
// such as Go's module cache, writable); root then removes what is left, which includes the top
// entry when its parent is root-owned.
func (s *Server) removeTree(path string, uid int) error {
	if s.cfg.DropPrivileges && uid > 0 {
		_ = s.asUser(uid, `chmod -R u+rwx "$1" 2>/dev/null; rm -rf "$1" 2>/dev/null; true`, path)
	}
	return os.RemoveAll(path)
}

// asUser runs a short shell script as uid with no environment.
func (s *Server) asUser(uid int, script string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", append([]string{"-c", script, "sh"}, args...)...)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"}
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: s.cfg.GID, Groups: []uint32{}}}
	return cmd.Run()
}

// processesOf lists live (non-zombie) processes whose real, effective or saved UID is uid.
func processesOf(uid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	want := strconv.Itoa(uid)
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		status, err := os.ReadFile(filepath.Join("/proc", e.Name(), "status"))
		if err != nil {
			continue
		}
		if procOwnedBy(string(status), want) {
			pids = append(pids, pid)
		}
	}
	return pids
}

func procOwnedBy(status, uid string) bool {
	owned, zombie := false, false
	for _, line := range strings.Split(status, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "State:":
			zombie = f[1] == "Z" || f[1] == "X"
		case "Uid:":
			for _, id := range f[1:min(len(f), 4)] {
				owned = owned || id == uid
			}
		}
	}
	return owned && !zombie
}

func ownerOf(path string) int {
	info, err := os.Lstat(path)
	if err != nil {
		return -1
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}

// chownTreeIfNeeded hands dir and its contents to uid:gid when dir is not already theirs.
func chownTreeIfNeeded(dir string, uid, gid int) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) == uid && int(st.Gid) == gid {
		return nil
	}
	return filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, uid, gid)
	})
}

// env is the only environment commands see: no tokens, no service addresses.
func env(home, cache string) []string {
	return []string{
		"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + home,
		"TMPDIR=" + home,
		"CI=1",
		"GOCACHE=" + filepath.Join(cache, "go-build"),
		"GOPATH=" + filepath.Join(cache, "go"),
		"GOFLAGS=-buildvcs=false",
		"npm_config_cache=" + filepath.Join(cache, "npm"),
		"YARN_CACHE_FOLDER=" + filepath.Join(cache, "yarn"),
		"PIP_CACHE_DIR=" + filepath.Join(cache, "pip"),
		// HOME is the throwaway copy, so without these Maven and Gradle download every dependency
		// again on every run, into the memory-backed /work.
		"MAVEN_OPTS=-Dmaven.repo.local=" + filepath.Join(cache, "m2"),
		"GRADLE_USER_HOME=" + filepath.Join(cache, "gradle"),
		// A Gradle daemon would only be killed at teardown; don't start one.
		"GRADLE_OPTS=-Dorg.gradle.daemon=false",
		"XDG_CACHE_HOME=" + filepath.Join(cache, "xdg"),
	}
}

// copyTree copies src to dst, skipping .git, and hands the copy to uid:gid when dropping privileges.
func copyTree(src, dst string, chown bool, uid, gid int) error {
	var total int64
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			if total += info.Size(); total > maxCopyBytes {
				return fmt.Errorf("workspace is larger than %d MB", maxCopyBytes>>20)
			}
			if err := copyFile(path, target, info.Mode().Perm()|0o200); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if chown {
		return filepath.WalkDir(dst, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Lchown(path, uid, gid)
		})
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// capBuffer keeps at most limit bytes and remembers that it dropped the rest.
type capBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); room > 0 {
		if len(p) > room {
			c.buf.Write(p[:room])
			c.truncated = true
		} else {
			c.buf.Write(p)
		}
	} else {
		c.truncated = true
	}
	return len(p), nil
}

func (c *capBuffer) String() string { return strings.ToValidUTF8(c.buf.String(), "�") }
