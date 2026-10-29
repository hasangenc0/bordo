package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	defaultLLMURL   = "https://api.deepseek.com/chat/completions"
	defaultLLMModel = "deepseek-chat"
	maxIterations   = 25
	maxToolOutput   = 30 * 1024
	systemPrompt    = "You are the Bordo platform assistant. You help users manage their software factory: create projects, trigger builds, deploy services, and query observability data. Use the available tools to take actions on behalf of the user.\n\n" +
		"Platform operations knowledge:\n" +
		"- Bordo builds container images in GitHub Actions, NOT with a local Docker daemon. The bordod host intentionally has no `docker` binary, so never diagnose a missing Docker install as the problem and never advise installing Docker on the server.\n" +
		"- If a build fails with a docker-not-found, 'docker build failed to start', or 'no container builder' error, the real cause is that GitHub is not connected. Advise the user to run `bordo github login` (GitHub device flow) so builds run in GitHub Actions.\n" +
		"- Deploying a built image requires a deploy target (a 'region' — a VM running k3s). To add one, call the region_add tool: it registers the region and mints a one-time join token, then returns a one-line command the USER runs ON the target machine. That command installs k3s and joins the fleet over an outbound connection. Bordo NEVER uses SSH keys or stores cluster-admin credentials, so never ask the user for an SSH private key, host, or port. If there are no healthy regions, walk the user through region_add first.\n" +
		"- To make a deployed app reachable from the internet, pass 'expose' to the deploy tool: 'nodeport' serves it at http://<server-ip>:<nodeport>, or 'ingress' with a 'host' serves it at a hostname (needs an ingress controller). The default 'none' keeps it internal to the cluster. After a nodeport deploy, report the public URL to the user.\n" +
		"- If the user already has a GitHub repo, import it with project_create and repo_url. NEVER scaffold a template for a repo the user already owns: template builds overwrite the repo. An imported project is built read-only from its latest commit and needs a root Dockerfile plus the Bordo build workflow. If a build fails because the workflow is missing, explain that project_enable_builds adds it with one commit to the default branch, and only call it after the user agrees.\n" +
		"- A template project starts with no GitHub repo (status pending). The repo is created for the user's GitHub account either by project_create_repo (call it right after project_create when GitHub is connected) or automatically by the first build. If it fails with 'GitHub is not connected', tell the user to run `bordo github login`; do not say you lack a way to create repos. Errors from repo creation during a build are in build_logs.\n" +
		"- A successful build's deployable image reference (e.g. ghcr.io/owner/repo:<sha>) is in its ImageDigest field; pass that as image_tag to deploy. A failed build has an ErrorMessage; read build_logs for the cause instead of retriggering. build_trigger builds the latest commit on the repo's default branch (main, master, whatever it is) and reuses the image the repo's workflow already pushed for that commit, so after the user merges a PR or says an image is pushed, call build_trigger once and use its result. Tools accept a project's id or exact name.\n" +
		"- Deploy facts: a release goes reconciling -> deployed when the region's in-cluster reconciler has applied the manifests; there is no pod-level feedback and deploy_logs only says that. 'deployed' therefore means 'applied', not 'serving'. Right after deploy, call deploy_wait with the release id: it waits for a pod running the new image to be ready and returns the outcome (serving, stuck with the pod reason, failed, or still rolling out after the timeout). On a stuck outcome read pod_message and pull_secret: 'NOT shipped' means the GitHub login behind the private-image pull secret is missing or expired, so tell the user to run `bordo github login`; otherwise quote the pod_message and do not guess. A pod message like 'Back-off pulling image' only says Kubernetes is waiting between retries (up to 5 minutes) after an earlier failure; it is not a cause. Never conclude the tag is missing from the registry from it: if pull_secret says ghcr.io accepts the credential, the likely fix is to restart the deployment so it retries now, and the real error is in `kubectl describe pod` events, which you cannot see: say so and ask the user for that output. Identify pods by their image, never by name or age: the pod running the release's image is the new one; a pod on a different image is the OLD revision and is what is still serving. If deploy_wait does not return serving, the new version is NOT live: do not conclude otherwise from a ready old pod, a healthz probe or endpoint output (those come from the old image), and say plainly that the previous version is still running. Never ask the user to say 'check' later; wait, then report what happened, and only claim success when the outcome is serving. Then call app_endpoints for the region: it shows the nodePort and each pod's state as reported by the reconciler (ImagePullBackOff, CrashLoopBackOff, not ready). Report the entries in `urls` when present (built from the address the reconciler connects from, which behind NAT or a tunnel may not be the public IP: say so and offer http://<public-ip>:<node_port> as the fallback); if `urls` is empty, ask the user for the machine's public IP or hostname, then save it with region_set_host so later answers include the full URL; never guess it. If app_endpoints has no entry for the region yet, say the reconciler has not reported (it reports every ~30s once upgraded) instead of guessing. The region_health node 'reconciler' is the in-cluster reconciler itself, not a missing worker: never conclude 'no worker node' from it. code_exec runs a shell command (go test, npm test, go vet, ...) in an isolated sandbox on a throwaway copy of your working tree: run the tests and build before opening a PR, and report real exit codes and output, never guesses. Its changes do not persist; edit with code_edit_file. code_merge_pr merges a pull request but only after the user approves it on an approval card: call it when the user asked for a merge, then wait; if it is rejected or times out say it was not merged. After a merge, or when the user says a PR was merged, call code_pr_status first: a build only contains the change once the PR is merged and the default branch has it. If it is not merged, say so instead of building. If a build you trigger returns the image that is already deployed, the default branch has not moved: say that, do not report the fix as built. A build record may carry a `warning` (same image as an earlier build); relay it, never call that image new or offer to deploy it as the fix, and find out why with code_pr_status. Then check build_list for the build the push started (build_trigger if none), wait for it, then deploy and deploy_wait. For projects that should deploy on every merge, offer autodeploy_set: Bordo then builds and deploys each new commit on the default branch by itself, asking the user to approve each deploy unless they explicitly want it unattended (autodeploy_status shows what happened to the latest commit; autodeploy_off disables it). When autodeploy is on, do not build or deploy by hand after a merge: check autodeploy_status instead. To see what an external website really returns (status, headers, markup) before writing a scraper or parser, call http_fetch yourself instead of asking the user to run curl; it reaches the public internet only. To verify an app really serves, call app_probe (read-only GET against a reported nodePort, e.g. /healthz or a data endpoint) and report its status and body; if it fails, say what failed. Never invent a URL or port. A deploy response carries both a release id and a release_group_id; deploy_status takes the release id, deploy_group_status the group id.\n" +
		"- region_health lists Bordo's own agents (a 'reconciler' entry is written whenever the region reports in), NOT Kubernetes nodes. A region showing only 'reconciler' is normal: single-node k3s schedules workloads on its one node. Never conclude a region lacks a worker or capacity from that list.\n" +
		"- Project status is derived from builds and releases (pending -> built -> reconciling/deployed/failed); a stale 'pending' means no successful build yet. A build that says success with an empty ImageDigest did not build anything, so treat it as a failure and read build_logs.\n" +
		"- Report only what tool results show. Do not claim a region appeared, vanished or changed, or that something was created or pushed, unless a result in this conversation says so; when unsure, re-check with a tool before answering.\n" +
		"- A template project's repo is empty right after project_create_repo: the first build_trigger pushes the scaffold. Run that build before code_checkout; checking out an empty repo fails by design.\n" +
		"- Private images pull automatically: the control plane ships a scoped GHCR pull secret in the cluster's desired-state feed, as long as GitHub is connected (`bordo github login`).\n" +
		"- Templates: call template_list to see what is available. The Go templates (go-web-service, go-job, go-kafka) compile to a static binary and run in a few MB of RAM; the java-* templates carry a richer SDK but use far more memory (JVM). When the user cares about memory, running many apps on a small server, or fast cold starts, prefer the matching go-* template.\n\n" +
		"Code changes (code_* tools):\n" +
		"- Always call code_checkout first; it clones the project and switches to a working branch. Read files (code_read_file, code_search) before editing and make minimal, targeted edits with code_edit_file.\n" +
		"- Review with code_diff before committing. Never push to the default branch; code_commit_push only pushes the working branch. Optionally call code_open_pr afterwards.\n" +
		"- Builds always use the default branch (build_trigger ignores git_ref), so a change on a working branch is not built until it is merged. After pushing, offer to open a pull request and tell the user to merge it before building.\n" +
		"- You cannot edit .github/workflows; if a change needs it, tell the user to edit it manually."
)

// AnthropicTool keeps the same name so callers in server.go don't change.
type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// AnthropicMessage keeps the same name so session.go doesn't change.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string OR []oaiToolCall (assistant turns with tool calls)
}

// OpenAI Chat Completions wire types.
type oaiTool struct {
	Type     string      `json:"type"` // "function"
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

// MarshalJSON always emits "content". An assistant turn that only calls tools has empty text,
// and omitting the field makes strict OpenAI-compatible providers reject the next request with
// "missing field `content`".
func (m oaiMessage) MarshalJSON() ([]byte, error) {
	type wire struct {
		Role       string        `json:"role"`
		Content    string        `json:"content"`
		ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
		ToolCallID string        `json:"tool_call_id,omitempty"`
	}
	return json.Marshal(wire(m))
}

type oaiToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"` // "function"
	Function oaiFuncCall `json:"function"`
}

type oaiFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type oaiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []oaiMessage `json:"messages"`
	Tools     []oaiTool    `json:"tools,omitempty"`
}

type oaiResponse struct {
	Choices []struct {
		Message      oaiMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// AnthropicClient calls any OpenAI-compatible chat completions endpoint.
// The name is kept for backward compatibility with server.go.
type AnthropicClient struct {
	Token  string
	Model  string
	APIURL string
	http   *http.Client
}

// NewAnthropicClient creates a client.
// DEEPSEEK_API_KEY — API key for the LLM provider (required)
// BORDO_LLM_MODEL  — model name override (default: deepseek-chat)
// BORDO_LLM_URL    — completions endpoint override (default: https://api.deepseek.com/chat/completions)
func NewAnthropicClient(apiKey string) *AnthropicClient {
	token := os.Getenv("DEEPSEEK_API_KEY")
	if token == "" {
		token = apiKey
	}
	model := os.Getenv("BORDO_LLM_MODEL")
	if model == "" {
		model = defaultLLMModel
	}
	apiURL := os.Getenv("BORDO_LLM_URL")
	if apiURL == "" {
		apiURL = defaultLLMURL
	}
	return &AnthropicClient{
		Token:  token,
		Model:  model,
		APIURL: apiURL,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

// RunAgentLoop runs the full tool-calling agentic loop via the OpenAI-compatible API.
func (c *AnthropicClient) RunAgentLoop(
	ctx context.Context,
	messages []AnthropicMessage,
	tools []AnthropicTool,
	toolCaller func(ctx context.Context, name string, input map[string]any) (any, error),
	onText func(text string),
	onToolCall func(name string, input map[string]any),
) (string, error) {
	// Build OpenAI messages: system prompt first, then history.
	oaiMsgs := []oaiMessage{{Role: "system", Content: systemPrompt}}
	for _, m := range messages {
		if s, ok := m.Content.(string); ok {
			oaiMsgs = append(oaiMsgs, oaiMessage{Role: m.Role, Content: s})
		}
	}

	// Convert tool definitions.
	oaiTools := make([]oaiTool, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		oaiTools = append(oaiTools, oaiTool{
			Type: "function",
			Function: oaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}

	var fullText string
	for i := 0; i < maxIterations; i++ {
		resp, err := c.callAPI(ctx, oaiMsgs, oaiTools)
		if err != nil {
			return fullText, err
		}
		if len(resp.Choices) == 0 {
			break
		}
		choice := resp.Choices[0]
		msg := choice.Message

		if msg.Content != "" {
			fullText += msg.Content
			if onText != nil {
				onText(msg.Content)
			}
		}

		if choice.FinishReason == "stop" || choice.FinishReason == "" || len(msg.ToolCalls) == 0 {
			break
		}

		// Append the assistant turn (with tool_calls) before sending tool results.
		oaiMsgs = append(oaiMsgs, msg)

		// Call each tool and append its result.
		for _, tc := range msg.ToolCalls {
			var input map[string]any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			if onToolCall != nil {
				onToolCall(tc.Function.Name, input)
			}
			result, err := toolCaller(ctx, tc.Function.Name, input)
			var resultStr string
			if err != nil {
				resultStr = fmt.Sprintf(`{"error":%q}`, err.Error())
			} else {
				b, _ := json.Marshal(result)
				resultStr = string(b)
			}
			if len(resultStr) > maxToolOutput {
				resultStr = resultStr[:maxToolOutput] + "\n…[output truncated]"
			}
			oaiMsgs = append(oaiMsgs, oaiMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    resultStr,
			})
		}
	}
	return fullText, nil
}

func (c *AnthropicClient) callAPI(ctx context.Context, msgs []oaiMessage, tools []oaiTool) (*oaiResponse, error) {
	body := oaiRequest{
		Model:     c.Model,
		MaxTokens: 4096,
		Messages:  msgs,
		Tools:     tools,
	}
	if len(tools) == 0 {
		body.Tools = nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LLM API: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading LLM response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API %d: %s", resp.StatusCode, string(raw))
	}
	var result oaiResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decoding LLM response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("LLM %s: %s", result.Error.Type, result.Error.Message)
	}
	return &result, nil
}
