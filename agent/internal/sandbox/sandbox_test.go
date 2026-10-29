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
