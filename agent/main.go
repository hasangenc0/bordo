// Package main is the entry point for bordo-agent, the Bordo AI agent server.
//
// bordo-agent hosts a WebSocket chat interface and an MCP tool server that
// lets Claude operate the Bordo platform. See ADR-0005 for rationale.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/bordo-io/bordo/agent/internal/server"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var (
		port      int
		apiKey    string
		logLevel  string
	)

	cmd := &cobra.Command{
		Use:          "bordo-agent",
		Short:        "Bordo AI agent — chat interface and MCP tool server",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if apiKey == "" {
				apiKey = os.Getenv("ANTHROPIC_API_KEY")
			}
			if apiKey == "" {
				apiKey = os.Getenv("BORDO_AGENT_API_KEY")
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			return server.Run(ctx, server.Config{
				Port:     port,
				APIKey:   apiKey,
				LogLevel: logLevel,
			})
		},
	}

	cmd.Flags().IntVar(&port, "port", 7402, "HTTP port")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Anthropic API key (or set ANTHROPIC_API_KEY)")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "log level: debug|info|warn|error")
	return cmd
}
