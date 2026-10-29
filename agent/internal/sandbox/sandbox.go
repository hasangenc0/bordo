// Package sandbox runs shell commands against a throwaway copy of a project's workspace.
//
// It is meant to run in its own container, on a network that cannot reach the control plane,
// with the agent's workspace mounted read-only. Each request copies the working tree (without
// .git) to a scratch directory, runs the command there as an unprivileged user with a scrubbed
// environment and a hard timeout, and deletes the copy. Nothing the command does persists or
// reaches the real repository: the agent edits files through code_edit_file, and uses this only
// to build, test and inspect.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
	CacheDir      string // persistent dependency caches (module/npm downloads), owned by the unprivileged user
	// UID/GID of the unprivileged user commands run as. DropPrivileges is off only in tests,
	// which do not run as root.
	DropPrivileges bool
	UID, GID       uint32
	MaxParallel    int
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
	cfg Config
	sem chan struct{}
}

// New returns a sandbox Server.
func New(cfg Config) *Server {
	n := cfg.MaxParallel
	if n <= 0 {
		n = 2
	}
	return &Server{cfg: cfg, sem: make(chan struct{}, n)}
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
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-r.Context().Done():
		return
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

// Run copies the project's workspace, runs command in the copy and returns its output.
func (s *Server) Run(ctx context.Context, project, command string, timeout time.Duration) (*Result, error) {
	src := filepath.Join(s.cfg.WorkspaceRoot, project)
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("no workspace for project %q: run code_checkout first", project)
	}
	if err := os.MkdirAll(s.cfg.WorkRoot, 0o755); err != nil {
		return nil, err
	}
	work := filepath.Join(s.cfg.WorkRoot, uuid.NewString())
	defer os.RemoveAll(work)
	if err := copyTree(src, work, s.cfg.DropPrivileges, int(s.cfg.UID), int(s.cfg.GID)); err != nil {
		return nil, fmt.Errorf("preparing workspace copy: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = work
	cmd.Env = s.env(work)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if s.cfg.DropPrivileges {
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: s.cfg.UID, Gid: s.cfg.GID}
	}
	// Kill the whole process group on timeout, not just the shell.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	out := &capBuffer{limit: maxOutput}
	cmd.Stdout, cmd.Stderr = out, out

	start := time.Now()
	err := cmd.Run()
	res := &Result{Output: out.String(), Truncated: out.truncated, DurationMS: time.Since(start).Milliseconds()}
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

// env is the only environment commands see: no tokens, no service addresses.
func (s *Server) env(home string) []string {
	cache := s.cfg.CacheDir
	if cache == "" {
		cache = filepath.Join(home, ".cache")
	}
	return []string{
		"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + home,
		"TMPDIR=" + home,
		"CI=1",
		"GOCACHE=" + filepath.Join(cache, "go-build"),
		"GOPATH=" + filepath.Join(cache, "go"),
		"GOFLAGS=-buildvcs=false",
		"npm_config_cache=" + filepath.Join(cache, "npm"),
		"PIP_CACHE_DIR=" + filepath.Join(cache, "pip"),
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
