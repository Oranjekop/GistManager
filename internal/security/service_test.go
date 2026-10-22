package security

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Oranjekop/GistManager/internal/storage"
)

type fakeVault struct {
	token string
	fail  bool
}

func (v *fakeVault) Load() (string, error) { return v.token, nil }
func (v *fakeVault) Save(s string) error {
	if v.fail {
		return errors.New("vault unavailable")
	}
	v.token = s
	return nil
}
func (v *fakeVault) Delete() error { v.token = ""; return nil }

type fakeBiometric struct{ available, verified bool }

func (b fakeBiometric) Available(context.Context) (bool, string) {
	return b.available, "fixture biometric"
}
func (b fakeBiometric) Verify(context.Context) error {
	if b.verified {
		return nil
	}
	return errors.New("cancelled")
}

func TestSecurityLifecycleAndPersistence(t *testing.T) {
	ctx, dir, vault := context.Background(), t.TempDir(), &fakeVault{}
	s, err := Open(dir, vault, fakeBiometric{true, true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveToken("github-secret"); err != nil {
		t.Fatal(err)
	}
	if err = s.Configure(ctx, Options{LockEnabled: true, BiometricEnabled: true, NewPassword: "test-password"}); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if _, err = s.Token(); !errors.Is(err, ErrLocked) {
		t.Fatal("locked token read", err)
	}
	if err = s.SaveToken("replace"); !errors.Is(err, ErrLocked) {
		t.Fatal("locked token write", err)
	}
	if err = s.ClearToken(); !errors.Is(err, ErrLocked) {
		t.Fatal("locked token clear", err)
	}
	if err = s.Configure(ctx, Options{}); !errors.Is(err, ErrLocked) {
		t.Fatal("locked settings change", err)
	}
	if err = s.Unlock("wrong"); err == nil || !s.State().Locked {
		t.Fatal("wrong password unlocked")
	}
	if err = s.UnlockBiometric(ctx); err != nil {
		t.Fatal(err)
	}
	if token, err := s.Token(); err != nil || token != "github-secret" {
		t.Fatal(token, err)
	}
	s, err = Open(dir, vault, fakeBiometric{true, false})
	if err != nil {
		t.Fatal(err)
	}
	if !s.State().Locked {
		t.Fatal("startup did not lock")
	}
	if err = s.UnlockBiometric(ctx); err == nil || !s.State().Locked {
		t.Fatal("cancelled biometric unlocked")
	}
	if err = s.Unlock("test-password"); err != nil {
		t.Fatal(err)
	}
	if err = s.Configure(ctx, Options{CurrentPassword: "wrong", NewPassword: "new-password"}); err == nil {
		t.Fatal("changed password without old password")
	}
	if err = s.Configure(ctx, Options{CurrentPassword: "test-password", NewPassword: "new-password", LockEnabled: true}); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if s.Unlock("test-password") == nil {
		t.Fatal("old password still accepted")
	}
	if err = s.Unlock("new-password"); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearToken(); err != nil || vault.token != "" {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "security.json"))
	for _, secret := range []string{"github-secret", "test-password", "new-password"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("plaintext credential persisted")
		}
	}
}

func TestLegacyPasswordAndCorruptSettings(t *testing.T) {
	// Generated with Node crypto.scryptSync('legacy-pass', '001122aabb', 64).
	expected := "9f95c81af476269feb922fe4102e8c27bab75d79a4c4cd284ddcfc83d17db7aa73f73c169e5c2fadf56dea9213a2fc4552977343ee7943e9a6d98e98f5c7793d"
	hash, err := passwordHash("legacy-pass", "001122aabb")
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(hash) != expected {
		t.Fatal("password hash is incompatible with the Electron version")
	}
	dir := t.TempDir()
	settings := Settings{Version: 1, LockEnabled: true, PasswordSalt: "001122aabb", PasswordHash: hex.EncodeToString(hash), EncryptedToken: "legacy-cipher"}
	if err = storage.WriteJSON(filepath.Join(dir, "security.json"), settings); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, &fakeVault{}, fakeBiometric{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Unlock("legacy-pass"); err != nil {
		t.Fatal("legacy password rejected", err)
	}
	if _, err = s.Token(); err == nil {
		t.Fatal("invalid legacy token should request re-entry")
	}
	if err = s.SaveToken("new-token"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "security.json"))
	if strings.Contains(string(data), "legacy-cipher") {
		t.Fatal("legacy ciphertext not removed after migration")
	}
	if err = os.WriteFile(filepath.Join(dir, "security.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(dir, &fakeVault{}, fakeBiometric{}); err == nil {
		t.Fatal("corrupt security settings silently unlocked")
	}
}

func TestConfigurationValidationAndVaultFailure(t *testing.T) {
	s, _ := Open(t.TempDir(), &fakeVault{fail: true}, fakeBiometric{})
	for _, options := range []Options{{LockEnabled: true}, {LockEnabled: true, NewPassword: "123"}, {NewPassword: "123456", BiometricEnabled: true}} {
		if s.Configure(context.Background(), options) == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	if s.SaveToken("new-token") == nil {
		t.Fatal("vault failure ignored")
	}
	if s.State().HasPassword {
		t.Fatal("failed configuration mutated settings")
	}
}

func TestLegacyDirectoryImportDoesNotOverrideOrDelete(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "gist-editor-workspace")
	native := filepath.Join(root, "Gist管理器")
	settings := Settings{Version: 1, LockEnabled: true, PasswordSalt: "legacy-salt", PasswordHash: "legacy-hash", EncryptedToken: "encrypted"}
	if err := storage.WriteJSON(filepath.Join(old, "security.json"), settings); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteJSON(filepath.Join(old, "updater.json"), map[string]string{"channel": "beta"}); err != nil {
		t.Fatal(err)
	}
	if err := ImportLegacy(native, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(native, "security.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "legacyStorageDirectory") || !strings.Contains(string(data), "legacy-hash") {
		t.Fatal("legacy state not imported")
	}
	if _, err = os.Stat(filepath.Join(old, "security.json")); err != nil {
		t.Fatal("old settings removed")
	}
	if err = storage.WriteJSON(filepath.Join(native, "security.json"), Settings{Version: 2}); err != nil {
		t.Fatal(err)
	}
	if err = ImportLegacy(native, root); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(native, "security.json"))
	if strings.Contains(string(data), "legacy-hash") {
		t.Fatal("existing native state overwritten")
	}
}
