package reconciler

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// cloneOrPull clones repoURL into localPath if it does not exist, otherwise pulls.
// Returns changed=true when the pull brought in new commits.
func cloneOrPull(ctx context.Context, repoURL, localPath string) (changed bool, err error) {
	if _, err := os.Stat(localPath + "/.git"); os.IsNotExist(err) {
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", repoURL, localPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return false, wrapExecError("git clone", out, err)
		}
		return true, nil
	}

	// Capture HEAD before pull.
	before, _ := gitRevParse(ctx, localPath, "HEAD")

	cmd := exec.CommandContext(ctx, "git", "-C", localPath, "pull", "--ff-only", "--quiet")
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, wrapExecError("git pull", out, err)
	}

	after, _ := gitRevParse(ctx, localPath, "HEAD")
	return before != after, nil
}

func gitRevParse(ctx context.Context, dir, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", ref)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// getChangedDirs returns the unique top-level directory names that changed between HEAD~1 and HEAD.
func getChangedDirs(ctx context.Context, localPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", localPath, "diff", "--name-only", "HEAD~1", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return nil, wrapExecError("git diff", out, err)
	}

	seen := map[string]bool{}
	var dirs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if !seen[parts[0]] {
			seen[parts[0]] = true
			dirs = append(dirs, parts[0])
		}
	}
	return dirs, nil
}

// kubectlApply runs kubectl apply -f manifestDir using the given kubeconfig.
func kubectlApply(ctx context.Context, manifestDir, kubeconfig string) error {
	args := []string{"apply", "-f", manifestDir, "--recursive"}
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	if kubeconfig != "" {
		cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return wrapExecError("kubectl apply", out, err)
	}
	return nil
}

func wrapExecError(op string, out []byte, err error) error {
	if len(bytes.TrimSpace(out)) > 0 {
		return fmt.Errorf("%s: %w: %s", op, err, strings.TrimSpace(string(out)))
	}
	return fmt.Errorf("%s: %w", op, err)
}
