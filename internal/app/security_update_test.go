package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"testing"

	"github.com/Oranjekop/GistManager/internal/security"
	"github.com/egoist/mygo/ui"
)

type appRoundTrip func(*http.Request) (*http.Response, error)

func (f appRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type authFixture struct {
	available bool
	verified  bool
	attempts  int
}

func (b *authFixture) Available(context.Context) (bool, string) {
	return b.available, "Windows Hello / PIN"
}
func (b *authFixture) Verify(context.Context) error {
	b.attempts++
	if !b.verified {
		return errors.New("fixture cancelled")
	}
	return nil
}

func lockedFixture(t *testing.T, biometric *authFixture, automatic bool) *App {
	t.Helper()
	a := fixtureApp(t)
	secure, err := security.Open(a.DataDir, &DemoVault{Value: "keep-token"}, biometric)
	if err != nil {
		t.Fatal(err)
	}
	available := biometric.available
	biometric.available = true
	defer func() { biometric.available = available }()
	if err = secure.Configure(context.Background(), security.Options{NewPassword: "fixture-password", LockEnabled: true, BiometricEnabled: automatic}); err != nil {
		t.Fatal(err)
	}
	a.Security, a.Updates = secure, nil
	a.Security.Lock()
	a.clearWorkspace()
	return a
}

func TestAutomaticAuthenticationOncePerLockAndManualRetry(t *testing.T) {
	biometric := &authFixture{available: true}
	a := lockedFixture(t, biometric, true)
	a.Start()
	if biometric.attempts != 1 || !a.Security.State().Locked || a.unlocking {
		t.Fatal("startup did not request exactly one verification", biometric.attempts, a.issue)
	}
	a.Start()
	for range 3 {
		a.maybeAutoUnlock()
	}
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Frame()
	if biometric.attempts != 1 {
		t.Fatal("cancelled automatic verification was retried")
	}
	click(t, tt, "使用 Windows Hello / PIN")
	if biometric.attempts != 2 || !a.Security.State().Locked {
		t.Fatal("manual retry failed")
	}
	fill(t, tt, "软件解锁密码", "fixture-password")
	click(t, tt, "解锁")
	if a.Security.State().Locked || a.user == nil {
		t.Fatal("password fallback failed", a.issue)
	}
	biometric.verified = true
	click(t, tt, "立即锁定")
	if biometric.attempts != 3 || a.Security.State().Locked || a.user == nil {
		t.Fatal("next lock cycle did not request authentication", biometric.attempts, a.issue)
	}
}

func TestAutomaticAuthenticationAvailabilityAndDuplicateRequests(t *testing.T) {
	for _, available := range []bool{false, true} {
		for _, automatic := range []bool{false, true} {
			biometric := &authFixture{available: available, verified: true}
			a := lockedFixture(t, biometric, automatic)
			var queue []func()
			a.async = func(fn func()) { queue = append(queue, fn) }
			a.Start()
			queue[0]()
			a.maybeAutoUnlock()
			a.maybeAutoUnlock()
			expected := 1
			if available && automatic {
				expected = 2
				a.unlock(true)
			}
			if len(queue) != expected {
				t.Fatalf("duplicate or unexpected authentication: available=%t auto=%t queued=%d", available, automatic, len(queue))
			}
			if expected == 2 {
				queue[1]()
				if biometric.attempts != 1 || a.Security.State().Locked {
					t.Fatal("automatic verification failed")
				}
			}
		}
	}
}

func TestLateAvailabilityDoesNotPromptAfterPasswordUnlock(t *testing.T) {
	biometric := &authFixture{available: true, verified: true}
	a := lockedFixture(t, biometric, true)
	var queue []func()
	a.async = func(fn func()) { queue = append(queue, fn) }
	a.Start()
	a.unlockPassword = "fixture-password"
	a.unlock(false)
	queue[1]()
	queue[0]()
	if biometric.attempts != 0 || len(queue) != 3 || a.Security.State().Locked {
		t.Fatal("late availability requested a redundant verification")
	}
}

func TestForgottenPasswordRecoveryFromLockedScreen(t *testing.T) {
	biometric := &authFixture{available: true}
	a := lockedFixture(t, biometric, false)
	a.Start()
	tt := ui.NewTester(a.View, 1000, 680)
	click(t, tt, "忘记密码")
	assertCenteredDialog(t, tt, "密码恢复对话框", 1000, 680)
	snapshot(t, tt, "password-recovery-light.png")
	click(t, tt, "验证身份并清除密码")
	if !a.recoveryOpen || a.recoveryIssue == "" || !a.Security.State().Locked || !a.Security.State().HasPassword {
		t.Fatal("cancelled recovery changed the password")
	}
	biometric.verified = true
	click(t, tt, "验证身份并清除密码")
	if a.recoveryOpen || a.Security.State().HasPassword || a.Security.State().Locked || a.user == nil || a.current == nil {
		t.Fatal("verified recovery did not restore the workspace", a.recoveryIssue)
	}
	token, err := a.Security.Token()
	if err != nil || token != "keep-token" {
		t.Fatal("recovery removed the token", token, err)
	}
	if biometric.attempts != 2 {
		t.Fatal("recovery reused a previous OS verification")
	}
}

func TestClearPasswordSettingsFlow(t *testing.T) {
	biometric := &authFixture{available: true, verified: true}
	a := lockedFixture(t, biometric, false)
	if err := a.Security.Unlock("fixture-password"); err != nil {
		t.Fatal(err)
	}
	a.Start()
	tt := ui.NewTester(a.View, 1000, 680)
	click(t, tt, "系统设置")
	click(t, tt, "清除程序密码")
	if tt.HasText("确认新密码") || a.settingsForm.LockEnabled || a.settingsForm.BiometricEnabled {
		t.Fatal("clear-password form still asks for a new password or enables locking")
	}
	assertCenteredDialog(t, tt, "系统设置对话框", 1000, 680)
	snapshot(t, tt, "clear-password-settings-light.png")
	click(t, tt, "保存设置")
	if !a.settingsOpen || a.settingsIssue == "" || !a.Security.State().HasPassword {
		t.Fatal("cleared password without authentication")
	}
	fill(t, tt, "当前密码", "fixture-password")
	click(t, tt, "保存设置")
	if a.settingsOpen || a.Security.State().HasPassword || a.Security.State().LockEnabled || a.settingsForm.CurrentPassword != "" {
		t.Fatal("clear-password settings were not saved", a.settingsIssue)
	}
}

func TestForgottenPasswordRecoveryFromSettings(t *testing.T) {
	biometric := &authFixture{available: true, verified: true}
	a := lockedFixture(t, biometric, false)
	if err := a.Security.Unlock("fixture-password"); err != nil {
		t.Fatal(err)
	}
	a.Start()
	tt := ui.NewTester(a.View, 1000, 680)
	click(t, tt, "系统设置")
	click(t, tt, "清除程序密码")
	click(t, tt, "忘记密码？使用系统身份验证")
	click(t, tt, "验证身份并清除密码")
	if a.recoveryOpen || a.settingsOpen || a.Security.State().HasPassword {
		t.Fatal("settings biometric recovery failed", a.recoveryIssue)
	}
}

func TestKeylessDownloadAndReopeningUpdateDialog(t *testing.T) {
	a := fixtureApp(t)
	data := []byte("download fixture")
	hash := sha256.Sum256(data)
	name := "GistManager-1.0.1-" + a.Updates.Platform + ".zip"
	location := "https://github.com/Oranjekop/GistManager/releases/download/v1.0.1/" + name
	a.Updates.HTTP.Transport = appRoundTrip(func(r *http.Request) (*http.Response, error) {
		body := data
		if r.URL.String() != location {
			body, _ = json.Marshal(map[string]any{"tag_name": "v1.0.1", "assets": []any{
				map[string]any{"name": name, "browser_download_url": location, "size": len(data), "digest": fmt.Sprintf("sha256:%x", hash)},
			}})
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	tt := ui.NewTester(a.View, 1000, 680)
	click(t, tt, "检查更新")
	if a.updateState.Status != "available" || !tt.HasText("下载更新") {
		t.Fatal("keyless update not offered", a.updateState)
	}
	assertCenteredDialog(t, tt, "软件更新对话框", 1000, 680)
	snapshot(t, tt, "update-available-light.png")
	var pending func()
	a.async = func(fn func()) { pending = fn }
	click(t, tt, "下载更新")
	click(t, tt, "关闭")
	click(t, tt, "检查更新")
	if !a.updateOpen || !a.updateBusy || a.updateState.Status != "downloading" {
		t.Fatal("could not reopen a running download")
	}
	pending()
	tt.Frame()
	if a.updateBusy || a.updateState.Status != "downloaded" || !tt.HasText("重启并安装") {
		t.Fatal("download did not finish", a.updateState)
	}
	click(t, tt, "关闭")
	click(t, tt, "检查更新")
	if a.updateState.Status != "downloaded" {
		t.Fatal("reopening discarded a downloaded update")
	}
	snapshot(t, tt, "update-downloaded-light.png")
}

func TestInstallerStartFailureOffersManualDownload(t *testing.T) {
	a := fixtureApp(t)
	a.updateOpen, a.updateState.Status = true, "downloaded"
	quit := false
	a.quit = func() { quit = true }
	tt := ui.NewTester(a.View, 1000, 680)
	click(t, tt, "重启并安装")
	if quit || a.updateState.Status != "error" || !tt.HasText("在 GitHub 查看") {
		t.Fatal("failed installation closed the app or hid manual updating", a.updateState)
	}
}

func TestAuthenticationPanelsCenteredAcrossSizesAndThemes(t *testing.T) {
	for _, size := range [][2]int{{1000, 680}, {1500, 940}} {
		for _, dark := range []bool{false, true} {
			biometric := &authFixture{available: true}
			a := lockedFixture(t, biometric, false)
			a.Start()
			tt := ui.NewTester(a.View, size[0], size[1])
			tt.SetDark(dark)
			panel := findBox(t, tt, "解锁面板")
			contentCenterY := float32(68 + (size[1]-68-32)/2)
			if math.Abs(float64(panel.X+panel.W/2-float32(size[0])/2)) > 1 ||
				math.Abs(float64(panel.Y+panel.H/2-contentCenterY)) > 1 {
				t.Fatal("lock panel is not centered within the workspace", panel)
			}
			click(t, tt, "忘记密码")
			assertCenteredDialog(t, tt, "密码恢复对话框", float32(size[0]), float32(size[1]))
			snapshot(t, tt, fmt.Sprintf("recovery-%d-dark-%t.png", size[0], dark))
		}
	}
}

func assertCenteredDialog(t *testing.T, tt *ui.Tester, label string, width, height float32) {
	t.Helper()
	box := findBox(t, tt, label)
	if math.Abs(float64(box.X+box.W/2-width/2)) > 1 || math.Abs(float64(box.Y+box.H/2-height/2)) > 1 {
		t.Fatalf("%s is not centered: %+v", label, box)
	}
	if box.X < 0 || box.Y < 0 || box.X+box.W > width || box.Y+box.H > height {
		t.Fatalf("%s exceeds the window: %+v", label, box)
	}
}
