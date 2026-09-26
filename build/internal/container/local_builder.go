package container

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
)

// LocalBuilder builds images using the local Docker daemon.
// It requires Docker (or nerdctl for Rancher Desktop) to be installed and running.
type LocalBuilder struct{}

// NewLocalBuilder returns a Builder that uses the local Docker daemon.
func NewLocalBuilder() Builder {
	return &LocalBuilder{}
}

// dockerBin returns "docker" if available, falling back to "nerdctl" (Rancher Desktop).
func dockerBin() string {
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	if _, err := exec.LookPath("nerdctl"); err == nil {
		return "nerdctl"
	}
	return "docker" // will fail with a clear "not found" error
}

func (b *LocalBuilder) Build(ctx context.Context, opts BuildOptions) (<-chan LogLine, error) {
	if opts.Dockerfile == "" {
		opts.Dockerfile = "Dockerfile"
	}
	if opts.Tag == "" {
		opts.Tag = "latest"
	}

	imageRef := opts.RegistryURL + "/" + opts.ImageName + ":" + opts.Tag
	if opts.RegistryURL == "" {
		imageRef = opts.ImageName + ":" + opts.Tag
	}

	// Use --load for local builds (no registry); --push when a registry is configured.
	// --load loads the built image into the local daemon (works on Rancher Desktop).
	var pushFlag string
	if opts.RegistryURL == "" {
		pushFlag = "--load"
	} else {
		pushFlag = "--push"
	}

	args := []string{
		"buildx", "build",
		"--file", opts.Dockerfile,
		"--tag", imageRef,
		pushFlag,
		"--progress=plain",
		opts.ContextPath,
	}

	bin := dockerBin()
	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s buildx: %w", bin, err)
	}

	ch := make(chan LogLine, 128)

	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			ch <- LogLine{Text: scanner.Text()}
		}
		scanner2 := bufio.NewScanner(stderr)
		for scanner2.Scan() {
			ch <- LogLine{Text: scanner2.Text()}
		}
		if err := cmd.Wait(); err != nil {
			ch <- LogLine{Text: fmt.Sprintf("[error] docker build failed: %v", err)}
		} else {
			action := "loaded locally"
			if opts.RegistryURL != "" {
				action = "pushed"
			}
			ch <- LogLine{Text: fmt.Sprintf("[done] image %s: %s", action, imageRef)}
		}
	}()

	return ch, nil
}
