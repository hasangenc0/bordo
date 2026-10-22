package tools

import (
	"context"
	"errors"
	"net/url"
	"strconv"
)

func observeParams(params map[string]any, keys ...string) url.Values {
	q := url.Values{}
	for _, k := range keys {
		switch v := params[k].(type) {
		case string:
			if v != "" {
				q.Set(k, v)
			}
		case float64:
			q.Set(k, strconv.FormatInt(int64(v), 10))
		}
	}
	return q
}

func metricsQueryTool(cp *CPClient) *Tool {
	return &Tool{
		Name: "metrics_query",
		Description: "Query metrics with PromQL across regions. Without step it is an instant query; " +
			"with step (e.g. \"60s\") it is a range query over start..end (unix seconds, default last hour).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":  map[string]any{"type": "string", "description": "PromQL query string"},
				"region": map[string]any{"type": "string", "description": "Limit to one region (optional)"},
				"start":  map[string]any{"type": "string", "description": "Range start, unix seconds (optional)"},
				"end":    map[string]any{"type": "string", "description": "Range end, unix seconds (optional)"},
				"step":   map[string]any{"type": "string", "description": "Range step, e.g. 60s (optional; makes this a range query)"},
			},
			"required": []string{"query"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			if s, _ := params["query"].(string); s == "" {
				return nil, errors.New("query is required")
			}
			q := observeParams(params, "query", "region", "start", "end", "step")
			return cp.Get(ctx, "/v1/observe/metrics?"+q.Encode())
		},
	}
}

func logsQueryTool(cp *CPClient) *Tool {
	return &Tool{
		Name:        "logs_query",
		Description: "Query logs with LogQL across regions (default last hour, max 100 lines per region).",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":  map[string]any{"type": "string", "description": "LogQL query string"},
				"region": map[string]any{"type": "string", "description": "Limit to one region (optional)"},
				"start":  map[string]any{"type": "string", "description": "Start as unix nanoseconds (optional)"},
				"end":    map[string]any{"type": "string", "description": "End as unix nanoseconds (optional)"},
			},
			"required": []string{"query"},
		},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			if s, _ := params["query"].(string); s == "" {
				return nil, errors.New("query is required")
			}
			q := observeParams(params, "query", "region", "start", "end")
			return cp.Get(ctx, "/v1/observe/logs?"+q.Encode())
		},
	}
}
