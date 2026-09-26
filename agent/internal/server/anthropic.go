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
	defaultLLMURL   = "https://api.cloudflare.com/client/v4/accounts/5ef26bd0b28e4de5cd395bf98f4a843d/ai/v1/chat/completions"
	defaultLLMModel = "deepseek-ai/DeepSeek-V4.1-Flash"
	maxIterations   = 10
	systemPrompt    = "You are the Bordo platform assistant. You help users manage their software factory: create projects, trigger builds, deploy services, and query observability data. Use the available tools to take actions on behalf of the user."
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

// AnthropicClient wraps the Cloudflare AI Gateway (OpenAI-compatible) endpoint.
// The name is kept for backward compatibility with server.go.
type AnthropicClient struct {
	Token  string
	Model  string
	APIURL string
	http   *http.Client
}

// NewAnthropicClient creates a client. Reads CF_API_TOKEN from env first;
// falls back to the apiKey argument (which may come from BORDO_AGENT_TOKEN or config).
func NewAnthropicClient(apiKey string) *AnthropicClient {
	token := os.Getenv("CF_API_TOKEN")
	if token == "" {
		token = apiKey
	}
	model := os.Getenv("BORDO_LLM_MODEL")
	if model == "" {
		model = defaultLLMModel
	}
	url := os.Getenv("BORDO_LLM_URL")
	if url == "" {
		url = defaultLLMURL
	}
	return &AnthropicClient{
		Token:  token,
		Model:  model,
		APIURL: url,
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
