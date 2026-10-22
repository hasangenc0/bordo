package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDeriveKeyPersistsOnStoreVolume verifies the key is written next to the DB
// (the persistent volume) and is stable across calls — so a container recreate
// does not orphan encrypted data.
func TestDeriveKeyPersistsOnStoreVolume(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BORDO_STORE_PATH", filepath.Join(dir, "bordod.db"))
	t.Setenv("BORDO_SECRET_KEY", "") // ensure env override is off
	t.Setenv("HOME", t.TempDir())    // make sure we are NOT using $HOME

	k1, err := DeriveKey()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	// The key file must land next to the DB, not in $HOME.
	if _, err := os.Stat(filepath.Join(dir, "secret.key")); err != nil {
		t.Fatalf("expected secret.key next to the DB: %v", err)
	}
	// A second derive (simulating a restart) must return the same key.
	k2, err := DeriveKey()
	if err != nil {
		t.Fatalf("derive again: %v", err)
	}
	if k1 != k2 {
		t.Fatal("key changed across calls — would orphan encrypted data")
	}
}

func TestDeriveKeyEnvOverride(t *testing.T) {
	t.Setenv("BORDO_SECRET_KEY", "a-fixed-passphrase")
	a, err := DeriveKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := DeriveKey()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("env-derived key must be deterministic")
	}
}
