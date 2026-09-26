package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// ObserveCmd returns the top-level observe command group.
func ObserveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observe",
		Short: "Query metrics, logs, and traces",
	}
	cmd.AddCommand(observeMetricsCmd(), observeLogsCmd(), observeTracesCmd())
	return cmd
}

func observeMetricsCmd() *cobra.Command {
	var project, metric, region string
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Query metrics (PromQL) for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			q := metric
			if project != "" {
				q = fmt.Sprintf(`{project="%s",%s}`, project, metric)
			}
			path := "/v1/observe/metrics?query=" + url.QueryEscape(q)
			if region != "" {
				path += "&region=" + url.QueryEscape(region)
			}
			body, err := c.GetRaw(cmd.Context(), path)
			if err != nil {
				return err
			}
			return printObserveResult(body)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project ID to scope the query")
	cmd.Flags().StringVar(&metric, "metric", "", "PromQL expression (required)")
	cmd.Flags().StringVar(&region, "region", "", "Region name to query (default: all regions)")
	_ = cmd.MarkFlagRequired("metric")
	return cmd
}

func observeLogsCmd() *cobra.Command {
	var project, query, last, region string
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Query logs (LogQL) for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			logql := query
			if logql == "" && project != "" {
				logql = fmt.Sprintf(`{project="%s"}`, project)
			}
			dur, err := time.ParseDuration(last)
			if err != nil {
				return fmt.Errorf("invalid --last duration %q: %w", last, err)
			}
			now := time.Now()
			start := now.Add(-dur)
			path := fmt.Sprintf("/v1/observe/logs?query=%s&start=%d&end=%d",
				url.QueryEscape(logql), start.UnixNano(), now.UnixNano())
			if region != "" {
				path += "&region=" + url.QueryEscape(region)
			}
			body, err := c.GetRaw(cmd.Context(), path)
			if err != nil {
				return err
			}
			return printObserveResult(body)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project ID to scope the query")
	cmd.Flags().StringVar(&query, "query", "", "LogQL expression (overrides --project default)")
	cmd.Flags().StringVar(&last, "last", "1h", "Look back duration (e.g. 30m, 1h, 24h)")
	cmd.Flags().StringVar(&region, "region", "", "Region name to query (default: all regions)")
	return cmd
}

func observeTracesCmd() *cobra.Command {
	var traceID, region string
	cmd := &cobra.Command{
		Use:   "traces",
		Short: "Fetch a trace by ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			path := "/v1/observe/traces/" + traceID
			if region != "" {
				path += "?region=" + url.QueryEscape(region)
			}
			body, err := c.GetRaw(cmd.Context(), path)
			if err != nil {
				return err
			}
			return printObserveResult(body)
		},
	}
	cmd.Flags().StringVar(&traceID, "trace-id", "", "Trace ID to fetch (required)")
	cmd.Flags().StringVar(&region, "region", "", "Region name to query (default: all regions, first match)")
	_ = cmd.MarkFlagRequired("trace-id")
	return cmd
}

func printObserveResult(body []byte) error {
	var check map[string]string
	if err := json.Unmarshal(body, &check); err == nil {
		if check["status"] == "backends_not_configured" {
			fmt.Println("Observability backends not configured.")
			fmt.Println("Deploy the stack and set environment variables:")
			fmt.Println("  BORDO_VM_URL    — VictoriaMetrics URL (e.g. http://node:8428)")
			fmt.Println("  BORDO_LOKI_URL  — Loki URL (e.g. http://node:3100)")
			fmt.Println("  BORDO_TEMPO_URL — Tempo URL (e.g. http://node:3200)")
			fmt.Println("\nSee deploy/observability/README.md for setup instructions.")
			return nil
		}
	}
	var pretty any
	if err := json.Unmarshal(body, &pretty); err != nil {
		fmt.Println(string(body))
		return nil
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	fmt.Println(string(out))
	return nil
}
