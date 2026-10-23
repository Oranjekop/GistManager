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

func TestClearPasswordRequiresAuthenticationAndPersists(t *testing.T) {
	for _, biometric := range []bool{false, true} {
		t.Run(map[bool]string{false: "current-password", true: "system-verification"}[biometric], func(t *testing.T) {
			ctx, dir, vault := context.Background(), t.TempDir(), &fakeVault{token: "keep-token"}
			s, err := Open(dir, vault, fakeBiometric{true, true})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Configure(ctx, Options{NewPassword: "original-password", LockEnabled: true, BiometricEnabled: biometric}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "security.json")
			before, _ := os.ReadFile(path)
			for _, password := range []string{"", "wrong"} {
				if err = s.ClearPasswordWithPassword(ctx, password); err == nil {
					t.Fatal("password cleared without valid authentication")
				}
				after, _ := os.ReadFile(path)
				if string(after) != string(before) {
					t.Fatal("failed clearing modified persisted settings")
				}
			}
			if biometric {
				s.Lock()
				// Recovery must work without knowing the password while locked.
				err = s.ClearPasswordWithBiometric(ctx, 0)
			} else {
				err = s.ClearPasswordWithPassword(ctx, "original-password")
			}
			if err != nil {
				t.Fatal(err)
			}
			s, err = Open(dir, vault, fakeBiometric{})
			if err != nil {
				t.Fatal(err)
			}
			if state := s.State(); state.Locked || state.HasPassword || state.LockEnabled || state.BiometricEnabled {
				t.Fatal("clearing left a lock or password", state)
			}
			if token, err := s.Token(); err != nil || token != "keep-token" {
				t.Fatal("clearing removed the GitHub token", token, err)
			}
			if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "original-password") {
				t.Fatal("plaintext password persisted", err)
			}
		})
	}
}

func TestPasswordRemovalRejectsLockedCancelledAndFailedPersistence(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir(), &fakeVault{token: "keep-token"}, fakeBiometric{})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Configure(ctx, Options{NewPassword: "original-password", LockEnabled: true}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if err = s.ClearPasswordWithPassword(ctx, "original-password"); !errors.Is(err, ErrLocked) {
		t.Fatal("password removal bypassed the lock screen", err)
	}
	if err = s.Unlock("original-password"); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = s.ClearPasswordWithPassword(cancelled, "original-password"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled password removal succeeded", err)
	}
	after, _ := os.ReadFile(s.path)
	if string(before) != string(after) {
		t.Fatal("rejected removal modified persisted settings")
	}
	blocked := filepath.Join(filepath.Dir(s.path), "not-a-directory")
	if err = os.WriteFile(blocked, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(blocked, "security.json")
	if err = s.ClearPasswordWithPassword(ctx, "original-password"); err == nil || !s.State().HasPassword || !s.State().LockEnabled {
		t.Fatal("failed persistence changed password state", err)
	}
	if token, err := s.Token(); err != nil || token != "keep-token" {
		t.Fatal("failed removal lost token", token, err)
	}
}

func TestChangingPasswordPreservesTemporarilyUnavailableBiometrics(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir(), &fakeVault{}, fakeBiometric{true, true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Configure(ctx, Options{NewPassword: "original-password", LockEnabled: true, BiometricEnabled: true}); err != nil {
		t.Fatal(err)
	}
	s.biometric = fakeBiometric{}
	if err = s.Configure(ctx, Options{CurrentPassword: "original-password", NewPassword: "new-password", LockEnabled: true, BiometricEnabled: true}); err != nil {
		t.Fatal("unavailable system verification prevented a password change", err)
	}
	s.Lock()
	if err = s.Unlock("original-password"); err == nil {
		t.Fatal("old password survived the change")
	}
	if err = s.Unlock("new-password"); err != nil || !s.State().BiometricEnabled {
		t.Fatal("password fallback or saved biometric preference was lost", err)
	}
}

func TestRecoveryCancellationAndUnavailableBiometricsKeepPassword(t *testing.T) {
	for _, biometric := range []fakeBiometric{{true, false}, {false, true}} {
		ctx, vault := context.Background(), &fakeVault{token: "keep-token"}
		s, _ := Open(t.TempDir(), vault, biometric)
		if err := s.Configure(ctx, Options{NewPassword: "original-password", LockEnabled: true}); err != nil {
			t.Fatal(err)
		}
		s.Lock()
		before, _ := os.ReadFile(s.path)
		if err := s.ClearPasswordWithBiometric(ctx, 0); err == nil {
			t.Fatal("unauthenticated recovery succeeded")
		}
		after, _ := os.ReadFile(s.path)
		if string(after) != string(before) || !s.State().Locked || !s.State().HasPassword {
			t.Fatal("failed recovery modified security state")
		}
		if err := s.Unlock("original-password"); err != nil || vault.token != "keep-token" {
			t.Fatal("failed recovery lost the original password or token", err)
		}
	}
}

type windowBiometric struct {
	owner  uintptr
	verify func()
}

func (*windowBiometric) Available(context.Context) (bool, string) { return true, "fixture biometric" }
func (*windowBiometric) Verify(context.Context) error {
	return errors.New("window owner was omitted")
}
func (b *windowBiometric) VerifyForWindow(_ context.Context, owner uintptr) error {
	b.owner = owner
	if b.verify != nil {
		b.verify()
	}
	return nil
}

func TestAuthenticationUsesWindowOwnerAndRejectsStaleProof(t *testing.T) {
	ctx := context.Background()
	biometric := &windowBiometric{}
	s, _ := Open(t.TempDir(), &fakeVault{}, biometric)
	if err := s.Configure(ctx, Options{NewPassword: "original-password", LockEnabled: true, BiometricEnabled: true}); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if err := s.UnlockBiometricForWindow(ctx, 123); err != nil || biometric.owner != 123 {
		t.Fatal("biometric verification omitted the parent window", err, biometric.owner)
	}
	s.Lock()
	biometric.verify = func() { s.Lock() }
	if err := s.UnlockBiometricForWindow(ctx, 123); err == nil || !s.State().Locked {
		t.Fatal("a previous lock cycle's verification unlocked the app")
	}
	if err := s.ClearPasswordWithBiometric(ctx, 456); err == nil || !s.State().HasPassword || biometric.owner != 456 {
		t.Fatal("stale verification cleared the password", err)
	}
	biometric.verify = nil
	// Recovery also works if automatic OS authentication was not enabled.
	if err := s.Unlock("original-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.Configure(ctx, Options{CurrentPassword: "original-password", LockEnabled: true}); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if err := s.ClearPasswordWithBiometric(ctx, 456); err != nil || s.State().HasPassword {
		t.Fatal("system-verified recovery depended on automatic authentication", err)
	}
}

func TestCancelledRecoveryCannotUseSuccessfulBiometricResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	biometric := &windowBiometric{verify: cancel}
	s, _ := Open(t.TempDir(), &fakeVault{}, biometric)
	if err := s.Configure(context.Background(), Options{NewPassword: "original-password", LockEnabled: true}); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if err := s.ClearPasswordWithBiometric(ctx, 1); !errors.Is(err, context.Canceled) || !s.State().Locked || !s.State().HasPassword {
		t.Fatal("cancelled verification cleared security state", err)
	}
}

func TestFailedRecoveryPersistenceKeepsSecurityState(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	s, _ := Open(dir, &fakeVault{token: "keep-token"}, fakeBiometric{true, true})
	if err := s.Configure(ctx, Options{NewPassword: "original-password", LockEnabled: true}); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(blocked, "security.json")
	if err := s.ClearPasswordWithBiometric(ctx, 0); err == nil || !s.State().Locked || !s.State().HasPassword {
		t.Fatal("failed persistence cleared the in-memory password", err)
	}
}
