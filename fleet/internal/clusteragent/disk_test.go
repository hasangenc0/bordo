package clusteragent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestDiskUsageOfRoot(t *testing.T) {
	total, free, ok := diskUsage("/")
	if !ok || total == 0 || free > total {
		t.Fatalf("diskUsage(/) = %d, %d, %v", total, free, ok)
	}
	if _, _, ok := diskUsage("/definitely/not/a/path"); ok {
		t.Fatal("a missing path must not report usage")
	}
}

func TestPullCarriesDiskHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	a := New(Config{CPURL: srv.URL, ClusterToken: "tok", Namespace: "ns"})
	if _, err := a.pullDesiredState(context.Background()); err != nil {
		t.Fatal(err)
	}
	total, e1 := strconv.ParseUint(got.Get(diskTotalHeader), 10, 64)
	free, e2 := strconv.ParseUint(got.Get(diskFreeHeader), 10, 64)
	if e1 != nil || e2 != nil || total == 0 || free > total {
		t.Fatalf("disk headers = %q / %q", got.Get(diskTotalHeader), got.Get(diskFreeHeader))
	}
	if got.Get(revisionHeader) != strconv.Itoa(Revision) {
		t.Fatalf("revision header = %q", got.Get(revisionHeader))
	}
}
