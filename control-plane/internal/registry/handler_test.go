package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeStore struct{ created *Project }

func (f *fakeStore) CreateProject(_ context.Context, p *Project) error {
	p.ID = "id-1"
	p.CreatedAt, p.UpdatedAt = time.Now(), time.Now()
	f.created = p
	return nil
}
func (f *fakeStore) GetProject(context.Context, string) (*Project, error) { return nil, ErrNotFound }
func (f *fakeStore) GetProjectByName(context.Context, string) (*Project, error) {
	return nil, ErrNotFound
}
func (f *fakeStore) ListProjects(context.Context) ([]*Project, error) { return nil, nil }
func (f *fakeStore) DeleteProject(context.Context, string) error      { return nil }
func (f *fakeStore) GetLatestReleaseInfo(context.Context, string) (string, string, bool, error) {
	return "", "", false, nil
}
func (f *fakeStore) GetEnvVars(context.Context, string) ([]*EnvVar, error) { return nil, nil }
func (f *fakeStore) SetEnvVars(context.Context, string, []*EnvVar) error   { return nil }

func post(t *testing.T, body string) (*httptest.ResponseRecorder, *fakeStore) {
	t.Helper()
	fs := &fakeStore{}
	rec := httptest.NewRecorder()
	NewHandler(fs).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	return rec, fs
}

func TestCreateImportedProject(t *testing.T) {
	rec, fs := post(t, `{"git_repo_url":"git@github.com:hasangenc0/hello-api.git"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if fs.created.Name != "hello-api" {
		t.Errorf("name defaults to the repo name, got %q", fs.created.Name)
	}
	if fs.created.GitRepoURL != "https://github.com/hasangenc0/hello-api" || fs.created.Template != "" {
		t.Errorf("stored %+v, want canonical https URL and no template", fs.created)
	}
}

func TestCreateRejectsTemplatePlusRepo(t *testing.T) {
	// A template build scaffolds into the repo and force-pushes: never allow it on imported code.
	rec, fs := post(t, `{"name":"x","template":"go-web-service","git_repo_url":"https://github.com/o/r"}`)
	if rec.Code != http.StatusBadRequest || fs.created != nil {
		t.Errorf("status %d created=%v, want 400 and nothing stored", rec.Code, fs.created)
	}
}

func TestCreateRejectsNonGitHubRepo(t *testing.T) {
	for _, u := range []string{"https://gitlab.com/o/r", "https://github.com/o", "file:///etc/passwd"} {
		rec, fs := post(t, `{"name":"x","git_repo_url":"`+u+`"}`)
		if rec.Code != http.StatusBadRequest || fs.created != nil {
			t.Errorf("%s: status %d created=%v, want 400", u, rec.Code, fs.created)
		}
	}
}

func TestCreateFromTemplateStillWorks(t *testing.T) {
	rec, fs := post(t, `{"name":"svc","template":"go-web-service"}`)
	if rec.Code != http.StatusCreated || fs.created.GitRepoURL != "" {
		t.Errorf("status %d repo=%q", rec.Code, fs.created.GitRepoURL)
	}
}
