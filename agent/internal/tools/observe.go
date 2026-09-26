package tools

import "context"

var notImplementedResponse = map[string]string{
	"status":  "not_implemented",
	"message": "observability stack not yet deployed (BRD-030)",
}

func metricsQueryTool(_ *CPClient) *Tool {
	return &Tool{
		Name:        "metrics_query",
		Description: "Query metrics using PromQL (requires BRD-030 observability stack)",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "PromQL query string"},
				"start": map[string]any{"type": "string", "description": "Start time (RFC3339 or Unix timestamp)"},
				"end":   map[string]any{"type": "string", "description": "End time (RFC3339 or Unix timestamp)"},
			},
			"required": []string{"query"},
		},
		CallFn: func(_ context.Context, _ map[string]any) (any, error) {
			return notImplementedResponse, nil
		},
	}
}

func logsQueryTool(_ *CPClient) *Tool {
	return &Tool{
		Name:        "logs_query",
		Description: "Query logs using LogQL (requires BRD-030 observability stack)",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "LogQL query string"},
				"start": map[string]any{"type": "string", "description": "Start time (RFC3339 or Unix timestamp)"},
				"end":   map[string]any{"type": "string", "description": "End time (RFC3339 or Unix timestamp)"},
			},
			"required": []string{"query"},
		},
		CallFn: func(_ context.Context, _ map[string]any) (any, error) {
			return notImplementedResponse, nil
		},
	}
}
