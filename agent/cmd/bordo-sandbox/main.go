// Command bordo-sandbox serves the sandboxed command runner the agent uses for code_exec.
// It runs in its own container; see internal/sandbox.
package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hasangenc0/bordo/agent/internal/sandbox"
)

func main() {
	env := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	uid, _ := strconv.Atoi(env("BORDO_SANDBOX_UID", "65534"))
	gid, _ := strconv.Atoi(env("BORDO_SANDBOX_GID", "65534"))
	cfg := sandbox.Config{
		WorkspaceRoot:  env("BORDO_WORKSPACE", "/data/workspaces"),
		WorkRoot:       env("BORDO_SANDBOX_WORK", "/work"),
		CacheDir:       env("BORDO_SANDBOX_CACHE", "/cache"),
		DropPrivileges: os.Getuid() == 0,
		UID:            uint32(uid),
		GID:            uint32(gid),
	}
	if !cfg.DropPrivileges {
		fmt.Fprintln(os.Stderr, "bordo-sandbox: not running as root, commands will run as this user")
	}
	if cfg.DropPrivileges {
		// The cache volume is created root-owned; commands run as the unprivileged user.
		_ = os.MkdirAll(cfg.CacheDir, 0o755)
		_ = os.Chown(cfg.CacheDir, uid, gid)
	}
	addr := ":" + env("BORDO_SANDBOX_PORT", "7403")
	srv := &http.Server{Addr: addr, Handler: sandbox.New(cfg).Handler(), ReadHeaderTimeout: 10 * time.Second}
	fmt.Println("bordo-sandbox listening on", addr)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
