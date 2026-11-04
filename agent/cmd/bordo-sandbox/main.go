// Command bordo-sandbox serves the sandboxed command runner the agent uses for code_exec.
// It runs in its own container; see internal/sandbox.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/hasangenc0/bordo/agent/internal/sandbox"
)

func main() {
	// Teardown re-runs this binary as a slot user to delete that user's IPC objects.
	if len(os.Args) > 1 && os.Args[1] == sandbox.IPCRMCommand {
		os.Exit(sandbox.RunIPCRM(os.Args[2:]))
	}
	env := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	// Slot i runs commands as BORDO_SANDBOX_UID+i, users nothing else in the container has.
	uid, _ := strconv.Atoi(env("BORDO_SANDBOX_UID", "10000"))
	gid, _ := strconv.Atoi(env("BORDO_SANDBOX_GID", "10000"))
	parallel, _ := strconv.Atoi(env("BORDO_SANDBOX_PARALLEL", "2"))
	cacheMB, _ := strconv.Atoi(env("BORDO_SANDBOX_CACHE_MAX_MB", "3072"))
	cfg := sandbox.Config{
		WorkspaceRoot:  env("BORDO_WORKSPACE", "/data/workspaces"),
		WorkRoot:       env("BORDO_SANDBOX_WORK", "/work"),
		CacheDir:       env("BORDO_SANDBOX_CACHE", "/cache"),
		DropPrivileges: os.Getuid() == 0,
		UID:            uint32(uid),
		GID:            uint32(gid),
		MaxParallel:    parallel,
		CacheMaxBytes:  int64(cacheMB) << 20,
		SharedTmp:      []string{"/tmp", "/dev/shm", "/dev/mqueue"},
	}
	if !cfg.DropPrivileges {
		fmt.Fprintln(os.Stderr, "bordo-sandbox: not running as root, commands will run as this user")
	}
	sb := sandbox.New(cfg)
	if err := sb.Prepare(); err != nil {
		fmt.Fprintln(os.Stderr, "bordo-sandbox:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go sb.Janitor(ctx, 5*time.Minute, time.Hour)

	addr := ":" + env("BORDO_SANDBOX_PORT", "7403")
	srv := &http.Server{Addr: addr, Handler: sb.Handler(), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		// Let in-flight runs finish their teardown; requests that outlast this are cut off, which
		// cancels their commands.
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	fmt.Println("bordo-sandbox listening on", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	<-done
}
