package observe

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryMetricsInstantAndRange(t *testing.T) {
	var paths []string
	var rawQueries []string
	vm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		rawQueries = append(rawQueries, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer vm.Close()
	h := newHandlerFromConfig(BackendConfig{VictoriaMetricsURL: vm.URL})

	do := func(qs string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.QueryMetrics(rec, httptest.NewRequest(http.MethodGet, "/v1/observe/metrics?"+qs, nil))
		return rec
	}

	if rec := do("query=up"); rec.Code != 200 {
		t.Fatalf("instant: %d %s", rec.Code, rec.Body)
	}
	if paths[0] != "/api/v1/query" {
		t.Errorf("instant path = %s", paths[0])
	}

	if rec := do("query=up&start=1000&end=2000&step=60s"); rec.Code != 200 {
		t.Fatalf("range: %d %s", rec.Code, rec.Body)
	}
	if paths[1] != "/api/v1/query_range" {
		t.Errorf("range path = %s", paths[1])
	}
	for _, want := range []string{"start=1000", "end=2000", "step=60s"} {
		if !strings.Contains(rawQueries[1], want) {
			t.Errorf("range query %q missing %s", rawQueries[1], want)
		}
	}

	// step only: defaults to the last hour.
	do("query=up&step=30")
	if paths[2] != "/api/v1/query_range" || !strings.Contains(rawQueries[2], "step=30") {
		t.Errorf("default range: %s?%s", paths[2], rawQueries[2])
	}

	if rec := do("query=up&step=bogus"); rec.Code != 400 {
		t.Errorf("bad step code = %d", rec.Code)
	}
}
