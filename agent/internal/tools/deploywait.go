package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

var deployWaitPoll = 5 * time.Second

// deployWaitTool waits for a release to actually be serving, so the agent reports the real
// outcome of a deploy instead of asking the user to "check again".
func deployWaitTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "deploy_wait",
		Description: "Wait for a release to finish rolling out and return the real outcome: the release status plus whether a pod running the release's image is ready, with the URLs. " +
			"Call it right after deploy instead of asking the user to check later. It polls for up to timeout_seconds (default 120, max 300) and returns early on success, failure or a pod stuck in " +
			"ImagePullBackOff/CrashLoopBackOff. If it times out it returns the latest state; say that it is still rolling out rather than claiming success.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":              map[string]any{"type": "string", "description": "Release id from deploy"},
				"timeout_seconds": map[string]any{"type": "integer", "description": "How long to wait (default 120, max 300)"},
			},
			"required": []string{"id"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			id, _ := params["id"].(string)
			if id == "" {
				return nil, fmt.Errorf("id is required")
			}
			timeout := 120
			if f, ok := params["timeout_seconds"].(float64); ok && f > 0 {
				timeout = int(f)
			}
			if timeout > 300 {
				timeout = 300
			}
			deadline := time.Now().Add(time.Duration(timeout) * time.Second)
			var last map[string]any
			for {
				relRaw, err := cp.Get(ctx, "/v1/releases/"+url.PathEscape(id))
				if err != nil {
					return nil, err
				}
				var rel struct {
					Status   string `json:"status"`
					ImageTag string `json:"image_tag"`
					Region   string `json:"region"`
				}
				if err := json.Unmarshal(relRaw, &rel); err != nil {
					return nil, err
				}
				var wl json.RawMessage
				if rel.Region != "" {
					wl, _ = cp.Get(ctx, "/v1/fleet/workloads?region="+url.QueryEscape(rel.Region))
				}
				done, state := rolloutState(rel.Status, rel.ImageTag, rel.Region, wl)
				state["release_id"] = id
				last = state
				if done {
					return state, nil
				}
				if time.Now().Add(deployWaitPoll).After(deadline) {
					state["timed_out"] = true
					state["note"] = "still rolling out when the wait ended; the reconciler reports about every 30s"
					return state, nil
				}
				select {
				case <-ctx.Done():
					return last, ctx.Err()
				case <-time.After(deployWaitPoll):
				}
			}
		},
	}
}

// rolloutState decides whether a release is finished. It is done when the release failed, or
// when it is deployed and a ready pod runs the release's image, or when a pod running that
// image is stuck (ImagePullBackOff, CrashLoopBackOff, ...).
func rolloutState(status, image, region string, workloads []byte) (bool, map[string]any) {
	state := map[string]any{"release_status": status, "image": image, "region": region}
	if status == "failed" || status == "rolled_back" {
		state["outcome"] = "failed"
		return true, state
	}
	var resp struct {
		Regions []struct {
			Region     string   `json:"region"`
			ReportedAt string   `json:"reported_at"`
			URLs       []string `json:"urls"`
			PullSecret string   `json:"pull_secret"`
			Pods       []struct {
				Name    string `json:"name"`
				Ready   bool   `json:"ready"`
				Reason  string `json:"reason"`
				Message string `json:"message"`
				Image   string `json:"image"`
			} `json:"pods"`
		} `json:"regions"`
	}
	if len(workloads) == 0 || json.Unmarshal(workloads, &resp) != nil {
		state["outcome"] = "waiting"
		return false, state
	}
	for _, r := range resp.Regions {
		if r.Region != region {
			continue
		}
		state["reported_at"] = r.ReportedAt
		for _, p := range r.Pods {
			if p.Image == "" {
				// An outdated reconciler cannot tell the new pod from the old one, so a ready pod
				// proves nothing about this release. Say so instead of waiting forever.
				state["outcome"] = "unknown"
				state["note"] = "the region's reconciler is outdated and does not report pod images, so this rollout cannot be verified; restart it (kubectl -n bordo-system rollout restart deployment/bordo-reconciler) after bordo upgrade"
				return true, state
			}
		}
		if r.PullSecret != "" {
			state["pull_secret"] = r.PullSecret
		}
		for _, p := range r.Pods {
			if p.Image != image {
				continue
			}
			state["pod"] = p.Name
			if p.Ready && status == "deployed" {
				state["outcome"] = "serving"
				state["urls"] = r.URLs
				return true, state
			}
			if p.Reason == "ImagePullBackOff" || p.Reason == "ErrImagePull" || p.Reason == "CrashLoopBackOff" || p.Reason == "CreateContainerConfigError" {
				state["outcome"] = "stuck"
				state["pod_reason"] = p.Reason
				if p.Message != "" {
					state["pod_message"] = p.Message
				}
				return true, state
			}
		}
	}
	state["outcome"] = "waiting"
	return false, state
}
