package security

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActualElectronCredentialMigration(t *testing.T) {
	fixture := os.Getenv("GIST_LEGACY_FIXTURE")
	if fixture == "" {
		t.Skip("set GIST_LEGACY_FIXTURE to an Electron safeStorage fixture")
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "security.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	stateData, err := os.ReadFile(filepath.Join(filepath.Dir(fixture), "legacy-electron-data", "Local State"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "Local State"), stateData, 0o600); err != nil {
		t.Fatal(err)
	}
	vault := &fakeVault{}
	s, err := Open(dir, vault, fakeBiometric{})
	if err != nil {
		t.Fatal(err)
	}
	if !s.State().Locked {
		t.Fatal("legacy lock was lost")
	}
	if err = s.Unlock("legacy-pass"); err != nil {
		t.Fatal("Node password could not unlock", err)
	}
	token, err := s.Token()
	if err != nil {
		t.Fatal("Electron safeStorage migration failed", err)
	}
	if token != "fixture-legacy-token" || vault.token != token {
		t.Fatal("legacy token not transferred")
	}
	s, err = Open(dir, vault, fakeBiometric{})
	if err != nil || !s.State().Locked || s.settings.EncryptedToken != "" {
		t.Fatal("migration state not persisted", err)
	}
}

func TestSystemCredentialStoreAndAvailability(t *testing.T) {
	if os.Getenv("GIST_TEST_SYSTEM_CREDENTIALS") != "1" {
		t.Skip("opt-in Windows credential-store integration")
	}
	vault := SystemVault{Service: "GistManager-integration-" + filepath.Base(t.TempDir())}
	t.Cleanup(func() { _ = vault.Delete() })
	if err := vault.Save("test-only-native-token"); err != nil {
		t.Fatal(err)
	}
	if value, err := vault.Load(); err != nil || value != "test-only-native-token" {
		t.Fatal("system vault round trip", err)
	}
	if err := vault.Delete(); err != nil {
		t.Fatal(err)
	}
	if value, err := vault.Load(); err != nil || value != "" {
		t.Fatal("system vault delete", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	available, label := (SystemBiometric{}).Available(ctx)
	t.Logf("Windows Hello availability: %v (%s)", available, label)
}
