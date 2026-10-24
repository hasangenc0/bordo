package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeRepoURL(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"https://github.com/hasangenc0/hello-api", "https://github.com/hasangenc0/hello-api", true},
		{"https://github.com/hasangenc0/hello-api.git", "https://github.com/hasangenc0/hello-api", true},
		{"  https://github.com/o/r/  ", "https://github.com/o/r", true},
		{"git@github.com:o/r.git", "https://github.com/o/r", true},
		{"https://gitlab.com/o/r", "", false},
		{"http://github.com/o/r", "", false},
		{"https://github.com/o", "", false},
		{"https://github.com/o/r/tree/main", "", false},
		{"https://github.com/o/../r", "", false},
		{"https://github.com/o/r;rm -rf", "", false},
		{"https://evil.com/https://github.com/o/r", "", false},
	} {
		got, _, _, err := NormalizeRepoURL(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("%q: got (%q, %v), want (%q, ok=%v)", tc.in, got, err, tc.want, tc.ok)
		}
	}
}

func fakeGitHub(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	old := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() { githubAPIBase = old; srv.Close() })
}

func TestRepoHeadAndHasFile(t *testing.T) {
	fakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r":
			_, _ = w.Write([]byte(`{"default_branch":"master"}`))
		case "/repos/o/r/commits/master":
			_, _ = w.Write([]byte(`{"sha":"abc123"}`))
		case "/repos/o/r/contents/Dockerfile":
			if r.URL.Query().Get("ref") != "abc123" {
				t.Errorf("ref = %q", r.URL.Query().Get("ref"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ctx := context.Background()
	b, sha, err := RepoHead(ctx, "tok", "o", "r")
	if err != nil || b != "master" || sha != "abc123" {
		t.Fatalf("RepoHead = %q %q %v", b, sha, err)
	}
	if ok, err := RepoHasFile(ctx, "tok", "o", "r", "Dockerfile", "abc123"); err != nil || !ok {
		t.Errorf("Dockerfile: %v %v", ok, err)
	}
	if ok, err := RepoHasFile(ctx, "tok", "o", "r", ".github/workflows/bordo-build.yml", "abc123"); err != nil || ok {
		t.Errorf("missing file reported present: %v %v", ok, err)
	}
	if _, _, err := RepoHead(ctx, "tok", "o", "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown repo should say not found, got %v", err)
	}
}

func TestAwaitRunDispatchesWhenNoRunAppears(t *testing.T) {
	var dispatched, polls atomic.Int32
	fakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workflows/bordo-build.yml/dispatches"):
			dispatched.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/runs"):
			// no run until the workflow has been dispatched, then in-progress, then done
			switch {
			case dispatched.Load() == 0:
				_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
			case polls.Add(1) < 2:
				_, _ = w.Write([]byte(`{"workflow_runs":[{"status":"in_progress","head_sha":"abc"}]}`))
			default:
				_, _ = w.Write([]byte(`{"workflow_runs":[{"status":"completed","conclusion":"success","head_sha":"abc"}]}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run, err := AwaitRunForCommit(ctx, "tok", "o", "r", "abcdef123456", "master", 20*time.Millisecond, 10*time.Millisecond, nil)
	if err != nil || run.Conclusion != "success" {
		t.Fatalf("run = %+v err = %v", run, err)
	}
	if dispatched.Load() != 1 {
		t.Errorf("dispatched %d times, want exactly 1", dispatched.Load())
	}
}

func TestAwaitRunDoesNotDispatchWhenPushRunExists(t *testing.T) {
	var dispatched atomic.Int32
	fakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			dispatched.Add(1)
		}
		_, _ = w.Write([]byte(`{"workflow_runs":[{"status":"completed","conclusion":"success","head_sha":"abc"}]}`))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := AwaitRunForCommit(ctx, "tok", "o", "r", "abcdef123456", "main", 10*time.Millisecond, 10*time.Millisecond, nil); err != nil {
		t.Fatal(err)
	}
	if dispatched.Load() != 0 {
		t.Error("must not dispatch when a run already exists")
	}
}

func TestWorkflowTriggersOnTheRepoDefaultBranch(t *testing.T) {
	y := workflowYAML("master")
	if !strings.Contains(y, "branches: [master]") || strings.Contains(y, "[main]") {
		t.Errorf("workflow must listen on master:\n%s", y)
	}
	if !strings.Contains(y, "${{ github.sha }}") || !strings.Contains(y, "workflow_dispatch") {
		t.Errorf("workflow lost its expressions or dispatch trigger:\n%s", y)
	}
}
