package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicAPIURL  = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	defaultModel     = "claude-haiku-4-5-20251001"
	maxIterations    = 10
	systemPrompt     = "You are the Bordo platform assistant. You help users manage their software factory: create projects, trigger builds, deploy services, and query observability data. Use the available tools to take actions on behalf of the user."
)

// AnthropicTool mirrors the Anthropic API tool definition.
type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ContentBlock is one element in a message's content array.
type ContentBlock struct {
	Type string `json:"type"`
	// type=text
	Text string `json:"text,omitempty"`
	// type=tool_use
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
	// type=tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

// AnthropicMessage is one turn in the API conversation.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string OR []ContentBlock
}

type apiRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []AnthropicMessage `json:"messages"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
}

type apiResponse struct {
	StopReason string         `json:"stop_reason"`
	Content    []ContentBlock `json:"content"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// AnthropicClient calls the Anthropic Messages API.
type AnthropicClient struct {
	APIKey string
	Model  string
	http   *http.Client
}

func NewAnthropicClient(apiKey string) *AnthropicClient {
	return &AnthropicClient{
		APIKey: apiKey,
		Model:  defaultModel,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

// RunAgentLoop runs the full agentic tool-calling loop.
//
// It sends messages to Claude, handles tool_use blocks by calling tools and
// feeding results back, and repeats until stop_reason=end_turn or maxIterations.
//
// onText is called with each text block as it arrives.
// onToolCall is called just before each tool is invoked.
func (c *AnthropicClient) RunAgentLoop(
	ctx context.Context,
	messages []AnthropicMessage,
	tools []AnthropicTool,
	toolCaller func(ctx context.Context, name string, input map[string]any) (any, error),
	onText func(text string),
	onToolCall func(name string, input map[string]any),
) (string, error) {
	var fullText string
	msgs := make([]AnthropicMessage, len(messages))
	copy(msgs, messages)

	for i := 0; i < maxIterations; i++ {
		resp, err := c.callAPI(ctx, msgs, tools)
		if err != nil {
			return fullText, err
		}

		for _, block := range resp.Content {
			if block.Type == "text" && block.Text != "" {
				fullText += block.Text
				if onText != nil {
					onText(block.Text)
				}
			}
		}

		if resp.StopReason == "end_turn" || resp.StopReason == "" {
			break
		}
		if resp.StopReason != "tool_use" {
			break
		}

		// Append Claude's full assistant turn (must include tool_use blocks for ID matching).
		msgs = append(msgs, AnthropicMessage{Role: "assistant", Content: resp.Content})

		// Call each tool and collect results.
		var resultBlocks []ContentBlock
		for _, block := range resp.Content {
			if block.Type != "tool_use" {
				continue
			}
			if onToolCall != nil {
				onToolCall(block.Name, block.Input)
			}
			result, err := toolCaller(ctx, block.Name, block.Input)
			var resultStr string
			if err != nil {
				resultStr = fmt.Sprintf(`{"error":%q}`, err.Error())
			} else {
				b, _ := json.Marshal(result)
				resultStr = string(b)
			}
			resultBlocks = append(resultBlocks, ContentBlock{
				Type:      "tool_result",
				ToolUseID: block.ID,
				Content:   resultStr,
			})
		}

		msgs = append(msgs, AnthropicMessage{Role: "user", Content: resultBlocks})
	}

	return fullText, nil
}

func (c *AnthropicClient) callAPI(ctx context.Context, msgs []AnthropicMessage, tools []AnthropicTool) (*apiResponse, error) {
	body := apiRequest{
		Model:     c.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  msgs,
		Tools:     tools,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic API: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading anthropic response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API %d: %s", resp.StatusCode, string(raw))
	}

	var result apiResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decoding anthropic response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("anthropic %s: %s", result.Error.Type, result.Error.Message)
	}
	return &result, nil
}
