// Package signing provides image signing and SBOM generation via cosign and syft.
package signing

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

var (
	ErrCosignNotFound = errors.New("cosign binary not found on PATH; install from https://github.com/sigstore/cosign")
	ErrSyftNotFound   = errors.New("syft binary not found on PATH; install from https://github.com/anchore/syft")
)

// SignImage signs an OCI image reference with cosign (keyless by default).
// Returns ErrCosignNotFound if cosign is not on PATH.
func SignImage(ctx context.Context, imageRef string) error {
	cosign, err := exec.LookPath("cosign")
	if err != nil {
		return ErrCosignNotFound
	}
	out, err := exec.CommandContext(ctx, cosign, "sign", "--yes", imageRef).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cosign sign %s: %w\n%s", imageRef, err, out)
	}
	return nil
}

// GenerateSBOM generates a CycloneDX SBOM for imageRef using syft and attaches
// it as an OCI attestation. Returns ErrSyftNotFound if syft is not on PATH.
func GenerateSBOM(ctx context.Context, imageRef string) error {
	syft, err := exec.LookPath("syft")
	if err != nil {
		return ErrSyftNotFound
	}
	out, err := exec.CommandContext(ctx, syft, "attest", "--output", "cyclonedx-json", imageRef).CombinedOutput()
	if err != nil {
		return fmt.Errorf("syft attest %s: %w\n%s", imageRef, err, out)
	}
	return nil
}
