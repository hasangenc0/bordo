package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureConsolePassword(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")

	// Existing install without a password and without a trailing newline:
	// the new line must not be glued onto the previous one.
	if err := os.WriteFile(env, []byte("BORDO_INTERNAL_TOKEN=abc\nCONSOLE_PORT=3000"), 0o600); err != nil {
		t.Fatal(err)
	}
	pw, created, err := ensureConsolePassword(env)
	if err != nil || !created || len(pw) < 32 {
		t.Fatalf("first call: pw=%q created=%v err=%v", pw, created, err)
	}
	got, _ := os.ReadFile(env)
	if !strings.Contains(string(got), "CONSOLE_PORT=3000\nBORDO_CONSOLE_PASSWORD="+pw+"\n") {
		t.Fatalf(".env not appended cleanly:\n%s", got)
	}
	if info, _ := os.Stat(env); info.Mode().Perm() != 0o600 {
		t.Errorf(".env perms = %v, want 0600", info.Mode().Perm())
	}

	// Second call is stable: same password, nothing rewritten.
	pw2, created2, err := ensureConsolePassword(env)
	if err != nil || created2 || pw2 != pw {
		t.Fatalf("second call: pw=%q created=%v err=%v", pw2, created2, err)
	}
	if again, _ := os.ReadFile(env); string(again) != string(got) {
		t.Error(".env changed on second call")
	}

	// Missing .env is created owner-only.
	fresh := filepath.Join(dir, "new", ".env")
	_ = os.MkdirAll(filepath.Dir(fresh), 0o700)
	if _, created, err := ensureConsolePassword(fresh); err != nil || !created {
		t.Fatalf("fresh: created=%v err=%v", created, err)
	}
	if info, _ := os.Stat(fresh); info.Mode().Perm() != 0o600 {
		t.Errorf("fresh .env perms = %v, want 0600", info.Mode().Perm())
	}
}
