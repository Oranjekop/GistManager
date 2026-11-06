package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Oranjekop/GistManager/internal/l10n"
	"github.com/Oranjekop/GistManager/internal/security"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func useTestLocale(t *testing.T, locale string) {
	t.Helper()
	i18n.SetLocaleFunc(func() string { return locale })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return "zh" }) })
}

func TestLanguagesKeepEditingAndSecurityFlows(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark-%t", lang, dark), func(t *testing.T) {
				useTestLocale(t, lang)
				a := fixtureApp(t)
				tt := ui.NewTester(a.View, 1000, 680)
				tt.SetDark(dark)
				labels := []string{"编辑", "保存修改", "系统设置", "设置密码", "新密码", "确认新密码", "设置并开启锁定", "返回设置", "清除密码", "验证并清除密码", "当前程序密码", "程序密码对话框", "密码恢复对话框", "系统设置对话框"}
				if lang == "en" {
					labels = []string{"Edit", "Save changes", "Settings", "Set password", "New password", "Confirm new password", "Set password and lock", "Back to settings", "Clear password", "Verify and clear password", "Current app password", "App password dialog", "Password recovery dialog", "Settings dialog"}
				}
				click(t, tt, labels[0])
				fill(t, tt, map[string]string{"zh": "代码编辑器", "en": "Code editor"}[lang], "// 中文用户内容 must remain unchanged\n")
				click(t, tt, labels[1])
				if a.dirty() || a.editorText() != "// 中文用户内容 must remain unchanged\n" {
					t.Fatal("localized save changed user content")
				}
				click(t, tt, labels[2])
				assertCenteredDialog(t, tt, labels[13], 1000, 680)
				click(t, tt, labels[3])
				assertCenteredDialog(t, tt, labels[11], 1000, 680)
				fill(t, tt, labels[4], "test-password")
				fill(t, tt, labels[5], "test-password")
				click(t, tt, labels[6])
				if !a.Security.State().HasPassword {
					t.Fatal("password setup failed", a.passwordIssue)
				}
				click(t, tt, labels[8])
				assertCenteredDialog(t, tt, labels[12], 1000, 680)
				fill(t, tt, labels[10], "incorrect")
				click(t, tt, labels[9])
				want := "当前程序密码不正确"
				if lang == "en" {
					want = "Incorrect current app password"
				}
				if !tt.HasText(want) || !a.Security.State().HasPassword {
					t.Fatal("localized failure did not preserve password", a.recoveryIssue)
				}
				fill(t, tt, labels[10], "test-password")
				click(t, tt, labels[9])
				if a.Security.State().HasPassword {
					t.Fatal("localized password removal failed", a.recoveryIssue)
				}
				snapshot(t, tt, fmt.Sprintf("language-%s-dark-%t-settings.png", lang, dark))
			})
		}
	}
}

func TestLanguageLayouts(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, dark := range []bool{false, true} {
			for _, size := range [][2]int{{1000, 680}, {1500, 940}} {
				t.Run(fmt.Sprintf("%s/dark-%t/%d", lang, dark, size[0]), func(t *testing.T) {
					useTestLocale(t, lang)
					a := fixtureApp(t)
					a.language = lang
					tt := ui.NewTester(a.View, size[0], size[1])
					tt.SetLocale(lang)
					tt.SetDark(dark)
					for _, source := range []string{"检查更新", "系统设置", "编辑", "复制 Raw", "打开 Raw", "重命名", "删除文件", "自动换行"} {
						box := findBox(t, tt, l10n.Text(source))
						if box.X < 0 || box.Y < 0 || box.X+box.W > float32(size[0])+1 || box.Y+box.H > float32(size[1])+1 {
							t.Fatalf("%s exceeds window: %+v", source, box)
						}
					}
					capture := func(scene string) { snapshot(t, tt, fmt.Sprintf("%s-%s-dark-%t-%d.png", scene, lang, dark, size[0])) }
					capture("workspace")
					a.clearWorkspace()
					tt.Frame()
					capture("welcome")
					if err := a.Security.Configure(context.Background(), security.Options{NewPassword: "test-password", LockEnabled: true}); err != nil {
						t.Fatal(err)
					}
					a.openSettings()
					tt.Frame()
					assertCenteredDialog(t, tt, l10n.Text("系统设置对话框"), float32(size[0]), float32(size[1]))
					capture("settings")
					a.settingsOpen = false
					a.Security.Lock()
					tt.Frame()
					capture("locked")
				})
			}
		}
	}
}

func TestLanguagePreferencePersistsIndependently(t *testing.T) {
	a := fixtureApp(t)
	if a.language != "" {
		t.Fatal("new installs must follow system")
	}
	var applied string
	a.applyLocale = func(tag string) { applied = tag }
	a.resetEditor("unsaved user code")
	a.settingsForm.LockEnabled = true
	a.setLanguage("en")
	if applied != "en" || !a.settingsForm.LockEnabled || a.editorText() != "unsaved user code" {
		t.Fatal("language change affected workspace or security draft")
	}
	b := fixtureApp(t)
	b.DataDir, b.applyLocale = a.DataDir, a.applyLocale
	b.LoadLayout()
	if b.language != "en" || applied != "en" {
		t.Fatal("language was not restored")
	}
	b.setLanguage("")
	if applied != "" {
		t.Fatal("system language was not restored")
	}
	data, err := os.ReadFile(filepath.Join(a.DataDir, "layout.json"))
	var preferences map[string]any
	if err != nil || json.Unmarshal(data, &preferences) != nil || preferences["language"] != "" {
		t.Fatal("system default was not saved")
	}
	b.setLanguage("invalid")
	if b.language != "" {
		t.Fatal("invalid language accepted")
	}
}

func TestLanguageSelectorAppliesWithoutClosingSettings(t *testing.T) {
	useTestLocale(t, "zh")
	a := fixtureApp(t)
	a.openSettings()
	a.status = l10n.Msgf("已打开 %s", "系统设置.go")
	a.settingsIssue = l10n.FromError(l10n.Error("当前程序密码不正确"))
	a.updateState.SetMessage(l10n.Msgf("发现新版本 %s", "9.9.9"))
	tt := ui.NewTester(a.View, 1000, 680)
	a.applyLocale = func(tag string) {
		locale := tag
		if tag == "" {
			locale = "zh"
		}
		i18n.SetLocaleFunc(func() string { return locale })
		tt.SetLocale(locale)
	}
	click(t, tt, "界面语言")
	click(t, tt, "English")
	if a.language != "en" || !a.settingsOpen || !tt.HasText("Save settings") {
		t.Fatal("English was not applied immediately")
	}
	if a.status.String() != "Opened 系统设置.go" || !tt.HasText("Incorrect current app password") || a.updateState.MessageText() != "Version 9.9.9 is available" {
		t.Fatal("existing messages did not follow the new language")
	}
	click(t, tt, "Interface language")
	click(t, tt, "System default")
	if a.language != "" || !tt.HasText("保存设置") {
		t.Fatal("system language was not restored")
	}
	if a.status.String() != "已打开 系统设置.go" || !tt.HasText("当前程序密码不正确") {
		t.Fatal("existing messages did not follow system language")
	}
}
