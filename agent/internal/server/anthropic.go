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
		"- Private images pull automatically: the control plane ships a scoped GHCR pull secret in the cluster's desired-state feed, as long as GitHub is connected (`bordo github login`).\n" +
		"- Templates: call template_list to see what is available. The Go templates (go-web-service, go-job, go-kafka) compile to a static binary and run in a few MB of RAM; the java-* templates carry a richer SDK but use far more memory (JVM). When the user cares about memory, running many apps on a small server, or fast cold starts, prefer the matching go-* template.\n\n" +
		"Code changes (code_* tools):\n" +
		"- Always call code_checkout first; it clones the project and switches to a working branch. Read files (code_read_file, code_search) before editing and make minimal, targeted edits with code_edit_file.\n" +
		"- Review with code_diff before committing. Never push to the default branch; code_commit_push only pushes the working branch. Optionally call code_open_pr afterwards.\n" +
		"- After pushing, offer to run build_trigger with git_ref set to the working branch so the user can test the change.\n" +
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
