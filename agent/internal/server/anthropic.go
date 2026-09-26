package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// callAnthropic sends the conversation to the Anthropic Messages API and returns
// the assistant's reply text.
func callAnthropic(ctx context.Context, apiKey string, history []Message, userMsg string) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	msgs := make([]msg, 0, len(history)+1)
	for _, m := range history {
		if m.Role == "user" || m.Role == "assistant" {
			msgs = append(msgs, msg{Role: m.Role, Content: m.Content})
		}
	}
	msgs = append(msgs, msg{Role: "user", Content: userMsg})

	body := map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 1024,
		"system":     "You are Bordo, an AI assistant for the Bordo software factory platform. Help users manage projects, builds, releases, and infrastructure.",
		"messages":   msgs,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding anthropic response: %w", err)
	}

	for _, c := range result.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in anthropic response")
}
