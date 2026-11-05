package cmd

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
)

// A full disk is how a Bordo machine most often goes wrong: images, the sandbox dependency cache
// and logs fill it, kubelet taints the node disk-pressure, and the region's reconciler can no
// longer start. These helpers let the CLI say so before that happens.
const (
	diskWarnPercent     = 80.0
	diskCriticalPercent = 90.0
	// minFreeToPull is the least free space `bordo upgrade` wants before pulling images; the
	// sandbox image alone is more than a GB.
	minFreeToPull = 3 << 30
)

type diskReading struct {
	Total, Free uint64
	Path        string
}

func (d diskReading) usedPercent() float64 {
	if d.Total == 0 {
		return 0
	}
	return float64(d.Total-d.Free) / float64(d.Total) * 100
}

func (d diskReading) level() string {
	switch p := d.usedPercent(); {
	case p >= diskCriticalPercent:
		return "critical"
	case p >= diskWarnPercent:
		return "warn"
	}
	return "ok"
}

func (d diskReading) String() string {
	return fmt.Sprintf("%.0f%% used, %s free of %s", d.usedPercent(), humanBytes(d.Free), humanBytes(d.Total))
}

func humanBytes(n uint64) string {
	const gb, mb = 1 << 30, 1 << 20
	if n >= gb {
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	}
	return fmt.Sprintf("%.0f MB", float64(n)/mb)
}

// statDisk measures the filesystem holding path.
func statDisk(path string) (diskReading, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return diskReading{}, false
	}
	bs := uint64(st.Bsize)
	return diskReading{Total: uint64(st.Blocks) * bs, Free: uint64(st.Bavail) * bs, Path: path}, true
}

// hostDisk measures the disk Docker keeps images and volumes on, falling back to "/" (Docker
// Desktop keeps them in a VM, whose root is not a path on this machine).
func hostDisk() (diskReading, bool) {
	if out, err := exec.Command("docker", "info", "--format", "{{.DockerRootDir}}").Output(); err == nil {
		if root := strings.TrimSpace(string(out)); root != "" {
			if d, ok := statDisk(root); ok {
				return d, true
			}
		}
	}
	return statDisk("/")
}

// diskAdvice is what to do about a disk that is filling, in the order that frees the most first.
const diskAdvice = `  docker system df -v           # what is using the space (look at the volumes)
  docker builder prune -f       # drop build cache
  docker volume rm <project>_sandbox-cache   # the sandbox dependency cache; safe, only costs a slower next build
                                              # (stop the sandbox first: docker compose rm -sf bordo-sandbox)`

// reportDisk prints the disk line and, when it is filling up, what to do. It never fails the
// command it is part of.
func reportDisk(w io.Writer, d diskReading) {
	fmt.Fprintf(w, "\nDisk (%s): %s\n", d.Path, d)
	switch d.level() {
	case "critical":
		fmt.Fprintln(w, "  The disk is nearly full. Past this point Kubernetes taints the node disk-pressure and the region's\n"+
			"  reconciler stops, which makes the region read unreachable. Free space now:")
		fmt.Fprintln(w, diskAdvice)
	case "warn":
		fmt.Fprintln(w, "  The disk is filling up. Free space before it gets worse:")
		fmt.Fprintln(w, diskAdvice)
	}
}

// preflightDisk warns before a pull or start when the disk is already tight. It only warns: the
// pull may still fit, and refusing could strand someone who has no better option.
func preflightDisk(w io.Writer) {
	d, ok := hostDisk()
	if !ok {
		return
	}
	switch {
	case d.Free < minFreeToPull:
		fmt.Fprintf(w, "Warning: only %s free on %s (%s). Pulling images needs a few GB and may fail.\n", humanBytes(d.Free), d.Path, d)
		fmt.Fprintln(w, diskAdvice)
		fmt.Fprintln(w)
	case d.level() != "ok":
		fmt.Fprintf(w, "Note: disk on %s is %s.\n\n", d.Path, d)
	}
}

// pruneBuildCache drops Docker's build cache after an upgrade. It only holds layers from image
// builds (never volumes or running containers), and nothing Bordo runs needs it.
func pruneBuildCache() {
	out, err := exec.Command("docker", "builder", "prune", "-f").CombinedOutput()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Total reclaimed space:") && !strings.HasSuffix(strings.TrimSpace(line), " 0B") {
			fmt.Println("Cleared Docker build cache. " + strings.TrimSpace(line))
		}
	}
}
