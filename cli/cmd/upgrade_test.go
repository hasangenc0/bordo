package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDocker puts a `docker` on PATH that reports one running bordod container
// from another stack for `ps`, and records every other invocation.
func fakeDocker(t *testing.T, psOutput string) (calls func() string) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls.log")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = ps ]; then printf '%s' '" + psOutput + "'; exit 0; fi\n" +
		"echo \"$@\" >> '" + log + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() string { b, _ := os.ReadFile(log); return string(b) }
}

func managedDir(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := platformDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const otherStack = "bordo-bordod-1\t0.0.0.0:7401->7401/tcp\n"

func TestUpgradeRefusesWhenAnotherStackOwnsThePorts(t *testing.T) {
	dir := managedDir(t)
	calls := fakeDocker(t, otherStack)

	err := upgradeStack(dir, false)
	if err == nil || !strings.Contains(err.Error(), "Another Bordo stack is running") {
		t.Fatalf("want conflict error, got %v", err)
	}
	if got := calls(); got != "" {
		t.Errorf("must not pull or start anything, but ran:\n%s", got)
	}
}

func TestUpgradeForceProceedsDespiteConflict(t *testing.T) {
	dir := managedDir(t)
	calls := fakeDocker(t, otherStack)

	if err := upgradeStack(dir, true); err != nil {
		t.Fatal(err)
	}
	got := calls()
	if !strings.Contains(got, "pull") || !strings.Contains(got, "up -d") {
		t.Errorf("expected pull and up, got:\n%s", got)
	}
}

func TestUpgradeOfAnExplicitDirIsNotBlockedByItsOwnStack(t *testing.T) {
	managedDir(t)
	other := t.TempDir() // e.g. /opt/bordo/deploy/bordo: the running stack itself
	if err := os.WriteFile(filepath.Join(other, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := fakeDocker(t, otherStack)

	if err := upgradeStack(other, false); err != nil {
		t.Fatalf("upgrading the running stack's own dir must work: %v", err)
	}
	if !strings.Contains(calls(), "up -d") {
		t.Errorf("expected up, got:\n%s", calls())
	}
}

func TestUpgradeBackfillsConsolePasswordOnlyInExistingEnv(t *testing.T) {
	managedDir(t)
	withEnv := t.TempDir()
	withoutEnv := t.TempDir()
	for _, d := range []string{withEnv, withoutEnv} {
		_ = os.WriteFile(filepath.Join(d, "docker-compose.yml"), []byte("services: {}\n"), 0o644)
	}
	_ = os.WriteFile(filepath.Join(withEnv, ".env"), []byte("BORDO_INTERNAL_TOKEN=abc\n"), 0o600)
	fakeDocker(t, "")

	if err := upgradeStack(withEnv, false); err != nil {
		t.Fatal(err)
	}
	if pw := readEnvValue(filepath.Join(withEnv, ".env"), "BORDO_CONSOLE_PASSWORD"); len(pw) < 32 {
		t.Errorf("password not added to existing .env (got %q)", pw)
	}

	if err := upgradeStack(withoutEnv, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(withoutEnv, ".env")); !os.IsNotExist(err) {
		t.Error("must not create a .env where there was none")
	}
}
