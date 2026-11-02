package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "ws", "p1")
	for path, content := range map[string]string{
		"main.txt":         "hello\n",
		"sub/inner.txt":    "inner\n",
		".git/config":      "[core]\nsecret = yes\n",
		".git/hooks/pre-x": "#!/bin/sh\n",
	} {
		full := filepath.Join(proj, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return New(Config{WorkspaceRoot: filepath.Join(root, "ws"), WorkRoot: filepath.Join(root, "work"), CacheDir: filepath.Join(root, "cache")}), proj
}

func TestRunSeesCopyWithoutGitAndRealWorkspaceIsUntouched(t *testing.T) {
	s, proj := newTestServer(t)
	res, err := s.Run(context.Background(), "p1", "cat main.txt sub/inner.txt; ls -a | tr '\\n' ' '; echo changed > main.txt; touch new.txt", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Output, "hello") || !strings.Contains(res.Output, "inner") {
		t.Fatalf("result = %+v", res)
	}
	if strings.Contains(res.Output, ".git") {
		t.Errorf(".git must not be copied into the sandbox: %q", res.Output)
	}
	got, _ := os.ReadFile(filepath.Join(proj, "main.txt"))
	if string(got) != "hello\n" {
		t.Errorf("command changed the real workspace: %q", got)
	}
	if _, err := os.Stat(filepath.Join(proj, "new.txt")); err == nil {
		t.Error("command created a file in the real workspace")
	}
	if entries, _ := os.ReadDir(s.cfg.WorkRoot); len(entries) != 0 {
		t.Errorf("scratch copy was not removed: %v", entries)
	}
}

func TestRunEnvironmentIsScrubbed(t *testing.T) {
	t.Setenv("BORDO_AGENT_TOKEN", "super-secret")
	t.Setenv("DEEPSEEK_API_KEY", "key")
	s, _ := newTestServer(t)
	res, err := s.Run(context.Background(), "p1", "env", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"super-secret", "DEEPSEEK", "BORDO_"} {
		if strings.Contains(res.Output, leak) {
			t.Errorf("environment leaked %q: %s", leak, res.Output)
		}
	}
}

func TestRunExitCodeTimeoutAndOutputCap(t *testing.T) {
	s, _ := newTestServer(t)
	res, _ := s.Run(context.Background(), "p1", "echo oops >&2; exit 3", 10*time.Second)
	if res.ExitCode != 3 || !strings.Contains(res.Output, "oops") {
		t.Errorf("exit/stderr: %+v", res)
	}
	start := time.Now()
	res, err := s.Run(context.Background(), "p1", "sleep 30 & sleep 30", 300*time.Millisecond)
	if err != nil || !res.TimedOut || time.Since(start) > 10*time.Second {
		t.Errorf("timeout: %+v err=%v after %v", res, err, time.Since(start))
	}
	res, _ = s.Run(context.Background(), "p1", "head -c 200000 /dev/zero | tr '\\0' x", 10*time.Second)
	if !res.Truncated || len(res.Output) != maxOutput {
		t.Errorf("output cap: truncated=%v len=%d", res.Truncated, len(res.Output))
	}
}

func TestRunRejectsUnknownProject(t *testing.T) {
	s, _ := newTestServer(t)
	if _, err := s.Run(context.Background(), "missing", "true", time.Second); err == nil {
		t.Fatal("unknown project must error")
	}
	for _, bad := range []string{"../etc", "a/b", "", ".hidden", "x y"} {
		if projectRe.MatchString(bad) {
			t.Errorf("project name %q must be rejected", bad)
		}
	}
}

func TestRunKillsBackgroundJobsAndReturnsWhenShellExits(t *testing.T) {
	s, _ := newTestServer(t)
	marker := filepath.Join(t.TempDir(), "still-running")
	start := time.Now()
	// The background job inherits stdout; the run must not wait for it, and it must not outlive the run.
	res, err := s.Run(context.Background(), "p1", "(sleep 2; touch "+marker+") & nohup sleep 30 >/dev/null 2>&1 & echo started", 10*time.Second)
	if err != nil {
		t.Fatalf("run with background jobs failed: %v", err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Output, "started") {
		t.Fatalf("result = %+v", res)
	}
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("run waited %v for background jobs", took)
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("background job kept running after the run returned")
	}
}

func TestRunsAreLimitedToSlots(t *testing.T) {
	s, _ := newTestServer(t)
	s = New(Config{WorkspaceRoot: s.cfg.WorkspaceRoot, WorkRoot: s.cfg.WorkRoot, MaxParallel: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go func() { _, _ = s.Run(context.Background(), "p1", "sleep 1", 5*time.Second) }()
	time.Sleep(100 * time.Millisecond)
	if _, err := s.Run(ctx, "p1", "true", time.Second); err == nil {
		t.Fatal("second run must wait for the only slot and give up when its context ends")
	}
}

func TestPrepareAndJanitorRemoveStaleCopiesOnly(t *testing.T) {
	s, _ := newTestServer(t)
	stale := filepath.Join(s.cfg.WorkRoot, "stale", "nested")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Prepare(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(stale)); !os.IsNotExist(err) {
		t.Errorf("stale copy survived Prepare: %v", err)
	}
	for _, slot := range []string{"slot-0", "slot-1"} {
		if st, err := os.Stat(filepath.Join(s.cfg.CacheDir, slot)); err != nil || !st.IsDir() {
			t.Errorf("cache dir %s not created: %v", slot, err)
		}
	}

	busy := filepath.Join(s.cfg.WorkRoot, "busy")
	_ = os.MkdirAll(busy, 0o755)
	s.active[busy] = true
	_ = os.MkdirAll(stale, 0o755)
	s.sweep()
	if _, err := os.Stat(busy); err != nil {
		t.Error("sweep removed the copy of a run in progress")
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("sweep left a stale copy")
	}
}

func TestProcOwnedBy(t *testing.T) {
	status := func(state, uids string) string {
		return "Name:\tsleep\nState:\t" + state + " (sleeping)\nTgid:\t42\nUid:\t" + uids + "\nGid:\t10000\t10000\t10000\t10000\n"
	}
	cases := []struct {
		status string
		want   bool
	}{
		{status("S", "10000\t10000\t10000\t10000"), true},
		{status("R", "0\t10001\t0\t0"), true},              // effective uid
		{status("S", "0\t0\t10001\t0"), true},              // saved uid
		{status("Z", "10001\t10001\t10001\t10001"), false}, // already dead
		{status("S", "10002\t10002\t10002\t10002"), false},
	}
	for i, c := range cases {
		uid := "10000"
		if i > 0 {
			uid = "10001"
		}
		if got := procOwnedBy(c.status, uid); got != c.want {
			t.Errorf("case %d: procOwnedBy = %v, want %v", i, got, c.want)
		}
	}
}
