package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	approvalPoll    = 1500 * time.Millisecond
	approvalTimeout = 10 * time.Minute
)

// errRejected is returned to the model when the user declines an action, so it reports that
// plainly instead of retrying.
var errRejected = errors.New("the user rejected this action; do not retry it, and tell them it was not done")

// requestApproval creates a pending approval in the control plane, shows it to the user (the
// console and the iOS app render a pending "action" frame with approve/reject buttons whose id
// is the approval id) and blocks until it is decided, rejected, or times out. emit sends an
// action frame to the client. It never runs the tool itself.
func (s *Server) requestApproval(ctx context.Context, tool, kind, summary string, input map[string]any, emit func(ActionEvent)) error {
	payload, _ := json.Marshal(map[string]any{"tool": tool, "input": input})
	raw, err := s.cpClient.Post(ctx, "/v1/approvals", map[string]string{"kind": kind, "target": summary, "payload": string(payload)})
	if err != nil {
		return fmt.Errorf("could not request approval: %w", err)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.ID == "" {
		return errors.New("could not request approval: bad response from the control plane")
	}
	ev := ActionEvent{Type: "action", ActionID: created.ID, ActionKind: kind, ActionTarget: summary, ActionStatus: "pending", ActionTs: time.Now().UnixMilli()}
	emit(ev)

	deadline := time.Now().Add(approvalTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(approvalPoll):
		}
		raw, err := s.cpClient.Get(ctx, "/v1/approvals/"+created.ID)
		if err == nil {
			var a struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(raw, &a) == nil {
				switch a.Status {
				case "approved":
					ev.ActionStatus, ev.ActionTs = "approved", time.Now().UnixMilli()
					emit(ev)
					return nil
				case "rejected":
					ev.ActionStatus, ev.ActionTs = "failed", time.Now().UnixMilli()
					emit(ev)
					return errRejected
				}
			}
		}
		if time.Now().After(deadline) {
			ev.ActionStatus, ev.ActionTs = "failed", time.Now().UnixMilli()
			emit(ev)
			return errors.New("no one approved this action within 10 minutes, so it was not done; ask again if it is still wanted")
		}
	}
}
