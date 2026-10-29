package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// execTool runs a shell command in the sandbox service against a throwaway copy of the project's
// workspace. The sandbox lives in its own container with no tokens and no route to Bordo's services,
// so this is a real shell without handing the agent's secrets to repo code.
func (c *CodeTools) execTool(sandboxURL string) *Tool {
	return c.tool("code_exec",
		"Run a shell command (build, test, lint, inspect) in an isolated sandbox against a COPY of the project's current working tree, including your uncommitted edits. "+
			"Examples: `go test ./...`, `go vet ./...`, `npm ci && npm test`, `python3 -m pytest`. Pipes and && work. "+
			"The copy is thrown away afterwards: files the command creates or changes do NOT persist and are not committed, so make real changes with code_edit_file / code_write_file. "+
			"No access to Bordo, its tokens or git history; it has internet access for dependencies. Returns exit_code, combined output (capped) and timed_out. Run code_checkout first.",
		map[string]any{
			"command":         prop("string", "Shell command, run with sh -c in the repo root"),
			"timeout_seconds": prop("integer", "Default 120, max 600"),
		}, []string{"command"},
		func(ctx context.Context, w *ws, params map[string]any) (any, error) {
			if err := w.requireCloned(); err != nil {
				return nil, err
			}
			cmd := strings.TrimSpace(str(params, "command"))
			if cmd == "" {
				return nil, errors.New("command is required")
			}
			timeout := intParam(params, "timeout_seconds")
			if timeout <= 0 {
				timeout = 120
			}
			if timeout > 600 {
				timeout = 600
			}
			return runInSandbox(ctx, sandboxURL, filepath.Base(w.dir), cmd, timeout)
		})
}

func runInSandbox(ctx context.Context, baseURL, project, command string, timeoutSeconds int) (any, error) {
	body, _ := json.Marshal(map[string]any{"project": project, "command": command, "timeout_seconds": timeoutSeconds})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/exec", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: time.Duration(timeoutSeconds+60) * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("the sandbox is not reachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return nil, errors.New("sandbox: " + e.Error)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
