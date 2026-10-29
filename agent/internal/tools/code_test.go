package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeRel(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"", ".", false},
		{"src/main.go", "src/main.go", false},
		{"./a//b/../c.txt", "a/c.txt", false},
		{"/etc/passwd", "", true},
		{"../x", "", true},
		{"a/../../x", "", true},
		{"..", "", true},
		{".git/config", "", true},
		{"a/.GIT/hooks", "", true},
		{"a\x00b", "", true},
	}
	for _, c := range cases {
		got, err := SafeRel(c.in)
		if (err != nil) != c.wantErr || (!c.wantErr && got != c.want) {
			t.Errorf("SafeRel(%q) = %q, %v; want %q err=%v", c.in, got, err, c.want, c.wantErr)
		}
	}
}

func TestIsWorkflowPath(t *testing.T) {
	for p, want := range map[string]bool{
		".github/workflows/ci.yml": true,
		".GitHub/Workflows/ci.yml": true,
		".github/workflows":        true,
		".github/CODEOWNERS":       false,
		"docs/.github/workflows/x": false,
	} {
		if IsWorkflowPath(p) != want {
			t.Errorf("IsWorkflowPath(%q) != %v", p, want)
		}
	}
}

func TestReplaceUnique(t *testing.T) {
	cases := []struct {
		name, s, old, new string
		all               bool
		want              string
		n                 int
		wantErr           bool
	}{
		{"unique", "a b c", "b", "X", false, "a X c", 1, false},
		{"ambiguous", "a b b", "b", "X", false, "", 0, true},
		{"all", "a b b", "b", "X", true, "a X X", 2, false},
		{"missing", "abc", "z", "X", false, "", 0, true},
		{"empty old", "abc", "", "X", false, "", 0, true},
		{"identical", "abc", "a", "a", false, "", 0, true},
	}
	for _, c := range cases {
		got, n, err := ReplaceUnique(c.s, c.old, c.new, c.all)
		if (err != nil) != c.wantErr || got != c.want || n != c.n {
			t.Errorf("%s: got %q,%d,%v", c.name, got, n, err)
		}
	}
}

func TestParseRepoURL(t *testing.T) {
	cases := []struct {
		in, host, owner, repo string
		wantErr               bool
	}{
		{"https://github.com/acme/app", "github.com", "acme", "app", false},
		{"https://github.com/acme/app.git", "github.com", "acme", "app", false},
		{"https://x-access-token:abc@github.com/acme/app.git/", "github.com", "acme", "app", false},
		{"git@github.com:acme/app.git", "github.com", "acme", "app", false},
		{"ssh://git@github.com/acme/app.git", "github.com", "acme", "app", false},
		{"github.com/acme/app", "github.com", "acme", "app", false},
		{"https://github.com/acme", "", "", "", true},
		{"nonsense", "", "", "", true},
	}
	for _, c := range cases {
		h, o, r, err := ParseRepoURL(c.in)
		if (err != nil) != c.wantErr || h != c.host || o != c.owner || r != c.repo {
			t.Errorf("ParseRepoURL(%q) = %q %q %q %v", c.in, h, o, r, err)
		}
	}
}

func TestScrubToken(t *testing.T) {
	tok := "ghp_secret123"
	enc := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + tok))
	in := "fatal: auth failed for " + tok + " header Basic " + enc
	out := ScrubToken(in, tok)
	if strings.Contains(out, tok) || strings.Contains(out, enc) {
		t.Errorf("token leaked: %s", out)
	}
	if ScrubToken("hello", "") != "hello" {
		t.Error("empty token must be a no-op")
	}
}

func TestParseDiffFiles(t *testing.T) {
	got := ParseDiffFiles("M\ta.go\nA\tb.go\nD\tc.go\n", "3\t1\ta.go\n10\t0\tb.go\n0\t7\tc.go\n")
	want := []DiffFile{{"a.go", "modified", 3, 1}, {"b.go", "added", 10, 0}, {"c.go", "deleted", 0, 7}}
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] %v != %v", i, got[i], want[i])
		}
	}
}

// ── integration with a local bare repo ───────────────────────────────────────

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func setup(t *testing.T) (*CodeTools, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	bare := filepath.Join(tmp, "origin.git")
	run(t, tmp, "init", "--bare", "-b", "main", bare)
	seed := filepath.Join(tmp, "seed")
	run(t, tmp, "clone", bare, seed)
	run(t, seed, "checkout", "-b", "main")
	_ = os.MkdirAll(filepath.Join(seed, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(seed, "src", "app.txt"), []byte("hello\nworld\nhello\n"), 0o644)
	_ = os.WriteFile(filepath.Join(seed, "README.md"), []byte("# app\n"), 0o644)
	run(t, seed, "add", "-A")
	run(t, seed, "commit", "-m", "init")
	run(t, seed, "push", "origin", "main")

	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			_, _ = w.Write([]byte(`{"projects":[{"id":"p-1","name":"my-app","git_repo_url":"https://github.com/acme/app.git"}]}`))
		case "/v1/github/token":
			_, _ = w.Write([]byte(`{"token":"tok","type":"user"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cp.Close)

	c := NewCodeTools(NewCPClient(cp.URL, ""), filepath.Join(tmp, "ws"))
	c.remoteURL = func(host, owner, repo string) string {
		if host != "github.com" || owner != "acme" || repo != "app" {
			t.Errorf("remoteURL(%s,%s,%s)", host, owner, repo)
		}
		return bare
	}
	return c, bare
}

func call(t *testing.T, c *CodeTools, name string, params map[string]any) (any, error) {
	t.Helper()
	for _, tl := range c.Tools() {
		if tl.Name == name {
			return tl.Call(context.Background(), params)
		}
	}
	t.Fatalf("no tool %s", name)
	return nil, nil
}

func TestCodeWorkflow(t *testing.T) {
	c, bare := setup(t)
	p := func(kv ...any) map[string]any {
		m := map[string]any{"project": "my-app"}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}

	// Before checkout nothing works.
	if _, err := call(t, c, "code_read_file", p("path", "README.md")); err == nil {
		t.Fatal("expected error before checkout")
	}

	res, err := call(t, c, "code_checkout", p("branch", "bordo/test"))
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["branch"] != "bordo/test" || m["base"] != "main" || m["path_count"] != 2 {
		t.Fatalf("checkout: %v", m)
	}
	// Resume without branch keeps the working branch.
	res, err = call(t, c, "code_checkout", p())
	if err != nil || res.(map[string]any)["branch"] != "bordo/test" {
		t.Fatalf("resume: %v %v", res, err)
	}

	// read / list / search
	r, err := call(t, c, "code_read_file", p("path", "src/app.txt", "numbered", true, "offset", 2, "limit", 1))
	if err != nil || r.(map[string]any)["content"] != "2\tworld" {
		t.Fatalf("read: %v %v", r, err)
	}
	l, err := call(t, c, "code_list_files", p())
	if err != nil || len(l.(map[string]any)["files"].([]string)) != 3 { // src/, src/app.txt, README.md
		t.Fatalf("list: %v %v", l, err)
	}
	s, err := call(t, c, "code_search", p("pattern", "wor.d"))
	if err != nil || len(s.(map[string]any)["matches"].([]string)) != 1 {
		t.Fatalf("search: %v %v", s, err)
	}
	s, err = call(t, c, "code_search", p("pattern", "nomatchhere"))
	if err != nil || len(s.(map[string]any)["matches"].([]string)) != 0 {
		t.Fatalf("empty search: %v %v", s, err)
	}

	// edit uniqueness, then success
	if _, err := call(t, c, "code_edit_file", p("path", "src/app.txt", "old_string", "hello", "new_string", "bye")); err == nil {
		t.Fatal("ambiguous edit should fail")
	}
	if _, err := call(t, c, "code_edit_file", p("path", "src/app.txt", "old_string", "world", "new_string", "there")); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, c, "code_write_file", p("path", "docs/new.md", "content", "new\n")); err != nil {
		t.Fatal(err)
	}

	// guarded paths
	for _, path := range []string{"../evil", "/etc/passwd", ".git/config", ".github/workflows/ci.yml"} {
		if _, err := call(t, c, "code_write_file", p("path", path, "content", "x")); err == nil {
			t.Errorf("write %q should be rejected", path)
		}
	}

	// symlink escape
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644)
	if err := os.Symlink(outside, filepath.Join(c.root, "p-1", "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, c, "code_read_file", p("path", "link/secret")); err == nil {
		t.Error("symlink escape on read should fail")
	}
	if _, err := call(t, c, "code_write_file", p("path", "link/new", "content", "x")); err == nil {
		t.Error("symlink escape on write should fail")
	}
	_ = os.Remove(filepath.Join(c.root, "p-1", "link"))

	// diff includes untracked file
	d, err := call(t, c, "code_diff", p())
	if err != nil {
		t.Fatal(err)
	}
	dm := d.(map[string]any)
	files := dm["files"].([]DiffFile)
	if len(files) != 2 || !strings.Contains(dm["diff"].(string), "+there") || !strings.Contains(dm["diff"].(string), "docs/new.md") {
		t.Fatalf("diff: %v", dm)
	}

	// dirty workspace blocks switching branches
	if _, err := call(t, c, "code_checkout", p("branch", "bordo/other")); err == nil {
		t.Error("switch with dirty workspace should fail")
	}

	// commit + push
	cr, err := call(t, c, "code_commit_push", p("message", "edit things"))
	if err != nil {
		t.Fatal(err)
	}
	cm := cr.(map[string]any)
	if cm["pushed"] != true || cm["compare_url"] != "https://github.com/acme/app/compare/main...bordo/test" {
		t.Fatalf("push: %v", cm)
	}
	if out := run(t, bare, "log", "--format=%s", "bordo/test", "-1"); strings.TrimSpace(out) != "edit things" {
		t.Errorf("origin log = %q", out)
	}
	if _, err := call(t, c, "code_commit_push", p("message", "again")); err == nil {
		t.Error("nothing-to-commit should error")
	}

	// the token must not be persisted in the clone's git config
	cfg, _ := os.ReadFile(filepath.Join(c.root, "p-1", ".git", "config"))
	if strings.Contains(string(cfg), "tok") || strings.Contains(string(cfg), "extraheader") {
		t.Errorf("token persisted in .git/config:\n%s", cfg)
	}
}

func TestCommitPushRefusesDefaultBranch(t *testing.T) {
	c, _ := setup(t)
	if _, err := call(t, c, "code_checkout", map[string]any{"project": "p-1"}); err != nil {
		t.Fatal(err)
	}
	run(t, filepath.Join(c.root, "p-1"), "checkout", "main")
	_ = os.WriteFile(filepath.Join(c.root, "p-1", "x.txt"), []byte("x"), 0o644)
	if _, err := call(t, c, "code_commit_push", map[string]any{"project": "p-1", "message": "m"}); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("want refusal on default branch, got %v", err)
	}
	if _, err := call(t, c, "code_checkout", map[string]any{"project": "p-1", "branch": "main"}); err == nil {
		t.Error("checkout of default branch should be refused")
	}
}

func TestCommitPushBlocksWorkflowChanges(t *testing.T) {
	c, _ := setup(t)
	if _, err := call(t, c, "code_checkout", map[string]any{"project": "p-1"}); err != nil {
		t.Fatal(err)
	}
	wf := filepath.Join(c.root, "p-1", ".github", "workflows")
	_ = os.MkdirAll(wf, 0o755)
	_ = os.WriteFile(filepath.Join(wf, "ci.yml"), []byte("on: push"), 0o644)
	if _, err := call(t, c, "code_commit_push", map[string]any{"project": "p-1", "message": "m"}); err == nil || !strings.Contains(err.Error(), "workflows") {
		t.Errorf("want workflow block, got %v", err)
	}
}

func TestOpenPR(t *testing.T) {
	c, _ := setup(t)
	var gotAuth, gotPath string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_, _ = w.Write([]byte(`{"html_url":"https://github.com/acme/app/pull/7","number":7}`))
	}))
	defer gh.Close()
	c.apiBase = gh.URL
	if _, err := call(t, c, "code_checkout", map[string]any{"project": "p-1"}); err != nil {
		t.Fatal(err)
	}
	res, err := call(t, c, "code_open_pr", map[string]any{"project": "p-1", "title": "T"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["number"] != 7 || gotPath != "/repos/acme/app/pulls" || gotAuth != "Bearer tok" {
		t.Errorf("pr: %v %s %s", m, gotPath, gotAuth)
	}
}

func TestGitErrorsAreScrubbed(t *testing.T) {
	c, _ := setup(t)
	c.remoteURL = func(_, _, _ string) string { return "/nonexistent/tok/path" }
	_, err := call(t, c, "code_checkout", map[string]any{"project": "p-1"})
	if err == nil {
		t.Fatal("expected clone failure")
	}
	if strings.Contains(err.Error(), "tok") && !strings.Contains(err.Error(), "***") {
		t.Errorf("token not scrubbed: %v", err)
	}
}

func TestMergePR(t *testing.T) {
	c, _ := setup(t)
	var merged bool
	var mergeBody map[string]string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/pulls/7":
			_, _ = w.Write([]byte(`{"state":"open","merged":false,"draft":false,"title":"T","base":{"ref":"main"},"head":{"sha":"abc123"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/app/pulls/7/merge":
			_ = json.NewDecoder(r.Body).Decode(&mergeBody)
			merged = true
			_, _ = w.Write([]byte(`{"sha":"def456","merged":true,"message":"Pull Request successfully merged"}`))
		case r.URL.Path == "/repos/acme/app/pulls/8":
			_, _ = w.Write([]byte(`{"state":"open","merged":false,"draft":true,"base":{"ref":"main"},"head":{"sha":"x"}}`))
		case r.URL.Path == "/repos/acme/app/pulls/9":
			_, _ = w.Write([]byte(`{"state":"closed","merged":true,"base":{"ref":"main"},"head":{"sha":"x"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()
	c.apiBase = gh.URL

	res, err := call(t, c, "code_merge_pr", map[string]any{"project": "p-1", "number": float64(7)})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if !merged || m["merged"] != true || m["merge_commit"] != "def456" || mergeBody["merge_method"] != "squash" || mergeBody["sha"] != "abc123" {
		t.Fatalf("merge result %v body %v", m, mergeBody)
	}
	for _, n := range []float64{8, 9} {
		if _, err := call(t, c, "code_merge_pr", map[string]any{"project": "p-1", "number": n}); err == nil {
			t.Errorf("PR %v must not be mergeable (draft / already merged)", n)
		}
	}
}

func TestMergeToolIsApprovalGated(t *testing.T) {
	c, _ := setup(t)
	for _, tl := range c.Tools() {
		if tl.Name == "code_merge_pr" {
			if tl.ApprovalSummary == nil {
				t.Fatal("code_merge_pr must require approval")
			}
			if got := tl.ApprovalSummary(map[string]any{"project": "my-app", "number": float64(7)}); !strings.Contains(got, "PR #7") || !strings.Contains(got, "my-app") {
				t.Fatalf("summary = %q", got)
			}
			return
		}
		if tl.ApprovalSummary != nil {
			t.Errorf("%s must not be gated", tl.Name)
		}
	}
	t.Fatal("code_merge_pr not registered")
}

func TestExecToolCallsSandbox(t *testing.T) {
	c, _ := setup(t)
	var got map[string]any
	sb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exec" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"exit_code":1,"output":"FAIL\n","duration_ms":12}`))
	}))
	defer sb.Close()
	t.Setenv("BORDO_SANDBOX_URL", sb.URL)

	if _, err := call(t, c, "code_checkout", map[string]any{"project": "p-1"}); err != nil {
		t.Fatal(err)
	}
	res, err := call(t, c, "code_exec", map[string]any{"project": "p-1", "command": "go test ./...", "timeout_seconds": float64(30)})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["exit_code"] != float64(1) || got["project"] != "p-1" || got["command"] != "go test ./..." || got["timeout_seconds"] != float64(30) {
		t.Fatalf("result %v, sandbox saw %v", m, got)
	}
}

func TestExecToolAbsentWithoutSandbox(t *testing.T) {
	c, _ := setup(t)
	t.Setenv("BORDO_SANDBOX_URL", "")
	for _, tl := range c.Tools() {
		if tl.Name == "code_exec" {
			t.Fatal("code_exec must not be registered without a sandbox")
		}
	}
}

func TestPRStatus(t *testing.T) {
	c, _ := setup(t)
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/app/pulls/1":
			_, _ = w.Write([]byte(`{"state":"open","merged":false,"title":"open one","base":{"ref":"main"}}`))
		case "/repos/acme/app/pulls/2":
			_, _ = w.Write([]byte(`{"state":"closed","merged":true,"title":"done","merge_commit_sha":"m1","base":{"ref":"main"}}`))
		case "/repos/acme/app/commits/main":
			_, _ = w.Write([]byte(`{"sha":"m1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()
	c.apiBase = gh.URL

	res, err := call(t, c, "code_pr_status", map[string]any{"project": "p-1", "number": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.(map[string]any); m["state"] != "open" || !strings.Contains(m["next"].(string), "not merged yet") {
		t.Fatalf("open PR: %v", m)
	}
	res, err = call(t, c, "code_pr_status", map[string]any{"project": "p-1", "number": float64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.(map[string]any); m["state"] != "merged" || m["head_is_the_merge_commit"] != true {
		t.Fatalf("merged PR: %v", m)
	}
}
