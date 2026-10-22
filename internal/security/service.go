package security

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Oranjekop/GistManager/internal/storage"
	"golang.org/x/crypto/scrypt"
)

var ErrLocked = errors.New("应用尚未解锁")

type Vault interface {
	Load() (string, error)
	Save(string) error
	Delete() error
}

type Biometric interface {
	Available(context.Context) (bool, string)
	Verify(context.Context) error
}

type Settings struct {
	Version          int    `json:"version"`
	LockEnabled      bool   `json:"lockEnabled"`
	BiometricEnabled bool   `json:"biometricEnabled"`
	PasswordSalt     string `json:"passwordSalt"`
	PasswordHash     string `json:"passwordHash"`
	EncryptedToken   string `json:"encryptedToken,omitempty"`
	LegacyDirectory  string `json:"legacyStorageDirectory,omitempty"`
}

type Options struct {
	CurrentPassword  string
	NewPassword      string
	ClearPassword    bool
	LockEnabled      bool
	BiometricEnabled bool
}

type State struct {
	Locked           bool
	LockEnabled      bool
	BiometricEnabled bool
	HasPassword      bool
}

type Service struct {
	mu        sync.Mutex
	path      string
	settings  Settings
	unlocked  bool
	vault     Vault
	biometric Biometric
	authEpoch uint64
}

func Open(dir string, vault Vault, biometric Biometric) (*Service, error) {
	s := &Service{path: filepath.Join(dir, "security.json"), vault: vault, biometric: biometric}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.settings.Version = 2
	} else if err != nil {
		return nil, err
	} else if err = json.Unmarshal(data, &s.settings); err != nil {
		return nil, fmt.Errorf("安全设置损坏，无法安全启动: %w", err)
	}
	if s.settings.LockEnabled && (s.settings.PasswordHash == "" || s.settings.PasswordSalt == "") {
		return nil, errors.New("安全设置不完整，请恢复 security.json 备份")
	}
	s.unlocked = !s.settings.LockEnabled
	return s, nil
}

func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state()
}

func (s *Service) state() State {
	return State{Locked: s.settings.LockEnabled && !s.unlocked, LockEnabled: s.settings.LockEnabled,
		BiometricEnabled: s.settings.BiometricEnabled, HasPassword: s.settings.PasswordHash != ""}
}

func (s *Service) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unlocked = !s.settings.LockEnabled
	s.authEpoch++
}

func (s *Service) Unlock(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settings.LockEnabled && !verify(password, s.settings) {
		return errors.New("解锁密码不正确")
	}
	s.unlocked = true
	s.authEpoch++
	return nil
}

func (s *Service) BiometricAvailability(ctx context.Context) (bool, string) {
	return s.biometric.Available(ctx)
}

func (s *Service) UnlockBiometric(ctx context.Context) error {
	return s.UnlockBiometricForWindow(ctx, 0)
}

func (s *Service) verifyBiometric(ctx context.Context, owner uintptr) error {
	if biometric, ok := s.biometric.(interface {
		VerifyForWindow(context.Context, uintptr) error
	}); ok {
		return biometric.VerifyForWindow(ctx, owner)
	}
	return s.biometric.Verify(ctx)
}

func (s *Service) UnlockBiometricForWindow(ctx context.Context, owner uintptr) error {
	s.mu.Lock()
	allowed := s.settings.LockEnabled && s.settings.BiometricEnabled
	epoch := s.authEpoch
	s.mu.Unlock()
	if !allowed {
		return errors.New("系统身份验证未启用")
	}
	if err := s.verifyBiometric(ctx, owner); err != nil {
		return fmt.Errorf("系统身份验证未通过，请使用软件密码: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if epoch != s.authEpoch {
		return errors.New("锁定状态已变化，请重新验证")
	}
	s.unlocked = true
	s.authEpoch++
	return nil
}

// Password recovery always requests fresh OS authentication. It is also
// available from the locked screen, even if automatic biometrics were disabled.
func (s *Service) ClearPasswordWithBiometric(ctx context.Context, owner uintptr) error {
	s.mu.Lock()
	epoch, hasPassword := s.authEpoch, s.settings.PasswordHash != ""
	s.mu.Unlock()
	if !hasPassword {
		return errors.New("尚未设置程序密码")
	}
	if available, reason := s.biometric.Available(ctx); !available {
		return errors.New(reason)
	}
	if err := s.verifyBiometric(ctx, owner); err != nil {
		return fmt.Errorf("未清除密码，系统身份验证未通过: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if epoch != s.authEpoch {
		return errors.New("安全设置已变化，请重新验证")
	}
	return s.clearPassword()
}

func (s *Service) clearPassword() error {
	settings := s.settings
	settings.PasswordHash, settings.PasswordSalt = "", ""
	settings.LockEnabled, settings.BiometricEnabled = false, false
	return s.saveSettings(settings)
}

func (s *Service) saveSettings(settings Settings) error {
	settings.Version = 2
	if err := storage.WriteJSON(s.path, settings); err != nil {
		return err
	}
	s.settings, s.unlocked = settings, true
	s.authEpoch++
	return nil
}

func (s *Service) Configure(ctx context.Context, options Options) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state().Locked {
		return ErrLocked
	}
	settings := s.settings
	if settings.PasswordHash != "" && !verify(options.CurrentPassword, settings) {
		return errors.New("当前解锁密码不正确")
	}
	if options.ClearPassword {
		if options.NewPassword != "" {
			return errors.New("清除密码时不能同时设置新密码")
		}
		return s.clearPassword()
	}
	if options.NewPassword != "" && utf8.RuneCountInString(options.NewPassword) < 6 {
		return errors.New("新的解锁密码至少需要 6 位")
	}
	if options.LockEnabled && settings.PasswordHash == "" && options.NewPassword == "" {
		return errors.New("请先设置至少 6 位的软件解锁密码")
	}
	if options.BiometricEnabled {
		available, reason := s.biometric.Available(ctx)
		if !available {
			return errors.New(reason)
		}
	}
	if options.NewPassword != "" {
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return err
		}
		settings.PasswordSalt = hex.EncodeToString(salt)
		hash, err := passwordHash(options.NewPassword, settings.PasswordSalt)
		if err != nil {
			return err
		}
		settings.PasswordHash = hex.EncodeToString(hash)
	}
	settings.LockEnabled = options.LockEnabled
	settings.BiometricEnabled = options.LockEnabled && options.BiometricEnabled
	return s.saveSettings(settings)
}

func passwordHash(password, salt string) ([]byte, error) {
	// Node's scryptSync uses the hex salt as a string, with these parameters.
	return scrypt.Key([]byte(password), []byte(salt), 16384, 8, 1, 64)
}

func verify(password string, settings Settings) bool {
	if password == "" || settings.PasswordHash == "" {
		return false
	}
	expected, err := hex.DecodeString(settings.PasswordHash)
	if err != nil || len(expected) != 64 {
		return false
	}
	actual, err := passwordHash(password, settings.PasswordSalt)
	return err == nil && subtle.ConstantTimeCompare(actual, expected) == 1
}

func (s *Service) Token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state().Locked {
		return "", ErrLocked
	}
	token, err := s.vault.Load()
	if err != nil || token != "" || s.settings.EncryptedToken == "" {
		return token, err
	}
	// Migration happens only after unlocking, and never writes a plaintext token.
	legacyDir := s.settings.LegacyDirectory
	if legacyDir == "" {
		legacyDir = filepath.Dir(s.path)
	}
	token, err = decryptLegacyToken(s.settings.EncryptedToken, legacyDir)
	if err != nil {
		return "", errors.New("旧版 Token 无法迁移，请重新输入 GitHub Token；原安全设置仍保留")
	}
	if err = s.vault.Save(token); err != nil {
		return "", err
	}
	settings := s.settings
	settings.EncryptedToken, settings.Version = "", 2
	settings.LegacyDirectory = ""
	if err = storage.WriteJSON(s.path, settings); err != nil {
		return "", err
	}
	s.settings = settings
	return token, nil
}

func (s *Service) SaveToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state().Locked {
		return ErrLocked
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("Token 不能为空")
	}
	if err := s.vault.Save(strings.TrimSpace(token)); err != nil {
		return err
	}
	if s.settings.EncryptedToken != "" {
		settings := s.settings
		settings.EncryptedToken, settings.Version = "", 2
		settings.LegacyDirectory = ""
		if err := storage.WriteJSON(s.path, settings); err != nil {
			return err
		}
		s.settings = settings
	}
	return nil
}

func (s *Service) ClearToken() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state().Locked {
		return ErrLocked
	}
	settings := s.settings
	settings.EncryptedToken = ""
	settings.LegacyDirectory = ""
	if err := storage.WriteJSON(s.path, settings); err != nil {
		return err
	}
	if err := s.vault.Delete(); err != nil {
		return err
	}
	s.settings = settings
	return nil
}
