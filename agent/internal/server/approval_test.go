package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hasangenc0/bordo/agent/internal/tools"
)

func fakeApprovalCP(t *testing.T, decision string, afterPolls int32) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/approvals":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ap-1","status":"pending"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/approvals/ap-1":
			status := "pending"
			if polls.Add(1) >= afterPolls {
				status = decision
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ap-1", "status": status})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &polls
}

func withFastApproval(t *testing.T) {
	op, ot := approvalPoll, approvalTimeout
	approvalPoll, approvalTimeout = 5*time.Millisecond, 2*time.Second
	t.Cleanup(func() { approvalPoll, approvalTimeout = op, ot })
}

func TestRequestApprovalWaitsThenProceeds(t *testing.T) {
	withFastApproval(t)
	cp, polls := fakeApprovalCP(t, "approved", 3)
	s := &Server{cpClient: tools.NewCPClient(cp.URL, "")}
	var events []ActionEvent
	err := s.requestApproval(context.Background(), "code_merge_pr", "merge", "Merge PR #7", map[string]any{"number": 7}, func(e ActionEvent) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if polls.Load() < 3 {
		t.Errorf("did not wait for the decision: %d polls", polls.Load())
	}
	if len(events) != 2 || events[0].ActionStatus != "pending" || events[0].ActionID != "ap-1" || events[1].ActionStatus != "approved" {
		t.Fatalf("events = %+v", events)
	}
}

func TestRequestApprovalRejected(t *testing.T) {
	withFastApproval(t)
	cp, _ := fakeApprovalCP(t, "rejected", 1)
	s := &Server{cpClient: tools.NewCPClient(cp.URL, "")}
	var last ActionEvent
	err := s.requestApproval(context.Background(), "code_merge_pr", "merge", "Merge PR #7", nil, func(e ActionEvent) { last = e })
	if err != errRejected || last.ActionStatus != "failed" {
		t.Fatalf("err = %v, last = %+v", err, last)
	}
}

func TestRequestApprovalTimesOut(t *testing.T) {
	withFastApproval(t)
	cp, _ := fakeApprovalCP(t, "approved", 1<<30)
	s := &Server{cpClient: tools.NewCPClient(cp.URL, "")}
	approvalTimeout = 50 * time.Millisecond
	err := s.requestApproval(context.Background(), "code_merge_pr", "merge", "x", nil, func(ActionEvent) {})
	if err == nil || !strings.Contains(err.Error(), "no one approved") {
		t.Fatalf("err = %v", err)
	}
}
