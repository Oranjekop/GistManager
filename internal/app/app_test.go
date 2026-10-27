package app

import (
	"context"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Oranjekop/GistManager/internal/security"
	"github.com/Oranjekop/GistManager/internal/update"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func fixtureApp(t testing.TB) *App {
	t.Helper()
	dir := t.TempDir()
	secure, err := security.Open(dir, &DemoVault{}, DemoBiometric{})
	if err != nil {
		t.Fatal(err)
	}
	updater := update.New(dir, "1.0.0")
	updater.Key = ""
	updater.HTTP.Transport = appRoundTrip(func(r *http.Request) (*http.Response, error) {
		data := `{"tag_name":"v1.0.1","name":"v1.0.1"}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data))}, nil
	})
	a := New(NewDemoAPI(), secure, updater, "1.0.0", dir)
	iconData, err := os.ReadFile("../../resources/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	a.Icon, err = ui.DecodeBitmap(iconData)
	if err != nil {
		t.Fatal(err)
	}
	a.async, a.post = func(fn func()) { fn() }, func(fn func()) { fn() }
	a.openURL = func(string) error { return nil }
	a.copyText = func(string) {}
	a.LoadDemo()
	return a
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}
func fill(t *testing.T, tt *ui.Tester, label, value string) {
	t.Helper()
	click(t, tt, label)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(value)
}

func TestNativeEditRenameAddDeleteWorkflow(t *testing.T) {
	a := fixtureApp(t)
	tt := ui.NewTester(a.View, 1500, 940)
	if !tt.HasText("只读") || !a.code.ReadOnly {
		t.Fatal("editor must start read-only")
	}
	before := a.editorText()
	click(t, tt, "代码编辑器")
	tt.Type("not allowed")
	if a.editorText() != before {
		t.Fatal("read-only editor changed")
	}
	click(t, tt, "编辑")
	fill(t, tt, "代码编辑器", "package main\n\n// 中文输入验证\nfunc main() {}\n")
	if !a.dirty() || a.code.ReadOnly {
		t.Fatal("edit did not modify native buffer")
	}
	click(t, tt, "settings.json")
	if !a.confirmOpen {
		t.Fatal("switching files lost unsaved changes without confirmation")
	}
	click(t, tt, "取消")
	if a.selected != "main.go" || !a.dirty() {
		t.Fatal("cancel did not retain edits")
	}
	click(t, tt, "保存修改")
	if a.issue != "" || a.dirty() || a.editing || !strings.Contains(a.current.Files["main.go"].Content, "中文输入验证") {
		t.Fatal("save failed", a.issue)
	}
	click(t, tt, "重命名")
	fill(t, tt, "新文件名", "tool.go")
	click(t, tt, "确认重命名")
	if a.selected != "tool.go" || a.renameOpen {
		t.Fatal("rename failed", a.issue)
	}
	if _, exists := a.current.Files["main.go"]; exists {
		t.Fatal("old file remains")
	}
	click(t, tt, "新增文件")
	fill(t, tt, "文件名", "extra.txt")
	fill(t, tt, "代码编辑器", "A new file\n")
	click(t, tt, "创建")
	if a.selected != "extra.txt" || len(a.current.Files) != 4 {
		t.Fatal("add file failed", a.issue)
	}
	click(t, tt, "删除文件")
	if !a.confirmOpen {
		t.Fatal("deletion must be confirmed")
	}
	click(t, tt, "永久删除")
	if len(a.current.Files) != 3 {
		t.Fatal("delete file failed", a.issue)
	}
	click(t, tt, "新建 Gist")
	fill(t, tt, "Gist 描述", "New native Gist")
	fill(t, tt, "文件名", "hello.go")
	fill(t, tt, "代码编辑器", "package hello\n")
	click(t, tt, "创建")
	if len(a.gists) != 3 || a.current.Description != "New native Gist" {
		t.Fatal("create Gist failed", a.issue)
	}
	click(t, tt, "删除 Gist")
	click(t, tt, "永久删除")
	if len(a.gists) != 2 || a.current == nil {
		t.Fatal("delete Gist failed", a.issue)
	}
}

func TestSearchLinksDirtyExitAndCRLFRoundTrip(t *testing.T) {
	a := fixtureApp(t)
	tt := ui.NewTester(a.View, 1500, 940)
	fill(t, tt, "搜索 Gist", "request.py")
	if got := a.filtered(); len(got) != 1 || a.gists[got[0]].ID != "demo-2" {
		t.Fatal("filename filtering failed")
	}
	fill(t, tt, "搜索 Gist", "")
	var opened, copied string
	a.openURL = func(s string) error { opened = s; return nil }
	a.copyText = func(s string) { copied = s }
	click(t, tt, "复制 Raw")
	click(t, tt, "打开 Raw")
	if copied == "" || copied != opened {
		t.Fatal("Raw actions failed")
	}
	click(t, tt, "在 GitHub 中打开")
	if opened != a.current.HTMLURL {
		t.Fatal("Gist web link failed")
	}
	file := a.current.Files[a.selected]
	file.Content = "first\r\n第二行\r\n"
	a.current.Files[a.selected] = file
	a.resetEditor(file.Content)
	a.editing = true
	a.code.Buffer.Edit(editor.Edit{Start: 0, End: 5, Text: "changed"})
	a.save()
	if a.current.Files[a.selected].Content != "changed\r\n第二行\r\n" {
		t.Fatal("save changed original CRLF line endings")
	}
	a.editing = true
	a.code.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "unsaved"})
	if a.RequestClose() || !a.confirmOpen {
		t.Fatal("exit discarded edits without confirmation")
	}
	quit := false
	a.quit = func() { quit = true }
	a.confirmed()
	if !quit || !a.allowClose {
		t.Fatal("confirmed exit failed")
	}
}

func TestLockSettingsAndBiometricFallback(t *testing.T) {
	a := fixtureApp(t)
	tt := ui.NewTester(a.View, 1500, 940)
	click(t, tt, "系统设置")
	click(t, tt, "设置密码")
	fill(t, tt, "新密码", "native-password")
	fill(t, tt, "确认新密码", "native-password")
	click(t, tt, "设置并开启锁定")
	if a.passwordOpen || !a.settingsOpen || !a.Security.State().LockEnabled {
		t.Fatal("password setup failed", a.passwordIssue)
	}
	click(t, tt, "关闭")
	click(t, tt, "立即锁定")
	if a.current != nil || a.user != nil || len(a.gists) != 0 || !a.Security.State().Locked {
		t.Fatal("lock did not clear workspace")
	}
	if tt.HasText("main.go") || !tt.HasText("工作区已锁定") {
		t.Fatal("locked workspace is visible")
	}
	fill(t, tt, "输入程序密码", "wrong")
	click(t, tt, "解锁")
	if !a.Security.State().Locked {
		t.Fatal("wrong password unlocked")
	}
	fill(t, tt, "输入程序密码", "native-password")
	click(t, tt, "解锁")
	if a.Security.State().Locked || a.user == nil || a.current == nil {
		t.Fatal("password fallback did not resume", a.issue)
	}
	if a.unlockPassword != "" || a.tokenDraft != "" {
		t.Fatal("credential input retained")
	}
}

func TestNativeLayoutAndSnapshots(t *testing.T) {
	a := fixtureApp(t)
	for _, size := range [][2]float32{{1500, 940}, {1000, 680}} {
		tt := ui.NewTester(a.View, int(size[0]), int(size[1]))
		for _, label := range []string{"新建 Gist", "系统设置", "启用锁定", "代码编辑器", "自动换行"} {
			box, found := tt.Find(label)
			if !found || box.X < 0 || box.Y < 0 || box.X+box.W > size[0]+1 || box.Y+box.H > size[1]+1 {
				t.Fatalf("%s outside window %v: %+v", label, size, box)
			}
		}
		if tt.FramePending() {
			t.Fatal("idle view never settles")
		}
		if size[0] == 1500 {
			snapshot(t, tt, "native-light.png")
			tt.SetDark(true)
			snapshot(t, tt, "native-dark.png")
		}
	}
}

func snapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("GIST_SNAPSHOTS")
	if dir == "" {
		return
	}
	tt.Advance(250 * time.Millisecond)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestEmptySearchCanReturnToTheList(t *testing.T) {
	a := fixtureApp(t)
	tt := ui.NewTester(a.View, 1000, 680)
	current := a.current.ID
	fill(t, tt, "搜索 Gist", "no-such-gist-fixture")
	if !tt.HasText("没有匹配的 Gist") || a.current.ID != current {
		t.Fatal("empty search lost the current workspace")
	}
	snapshot(t, tt, "search-empty-light.png")
	click(t, tt, "清空搜索")
	if a.search != "" || !tt.HasText("API 请求示例") || a.current.ID != current {
		t.Fatal("clearing search did not restore the list")
	}
	click(t, tt, "API 请求示例")
	if a.current.ID != "demo-2" {
		t.Fatal("list could not select a Gist after clearing search")
	}
}

func TestUpdateDialogUsesOnlyFormalReleases(t *testing.T) {
	a := fixtureApp(t)
	opened := ""
	a.openURL = func(location string) error { opened = location; return nil }
	tt := ui.NewTester(a.View, 1500, 940)
	click(t, tt, "检查更新")
	if !a.updateOpen || a.updateState.Status != "manual" {
		t.Fatal("installer-only release should offer manual download")
	}
	if tt.HasText("测试版") || tt.HasText("正式版") {
		t.Fatal("update dialog still exposes a release-channel selector")
	}
	snapshot(t, tt, "update-formal-only.png")
	click(t, tt, "前往 GitHub 下载")
	if opened != "https://github.com/Oranjekop/GistManager/releases/latest" {
		t.Fatal("download page does not use the latest formal release", opened)
	}
	click(t, tt, "关闭")
	if a.updateOpen {
		t.Fatal("update dialog did not close")
	}
}

func TestLegacyColorsAndLanguageCoverage(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := legacyTheme(ui.Theme{Dark: dark})
		accent, text := "#2563eb", "#0f172a"
		if dark {
			accent, text = "#38bdf8", "#d7deea"
		}
		if theme.Accent != ui.Hex(accent) || theme.Text != ui.Hex(text) {
			t.Fatal("legacy UI palette changed")
		}
	}
	if codeTheme.Background != ui.Hex("#0f172a") || codeTheme.Tokens[editor.TokenString].Color != ui.Hex("#f9a8d4") {
		t.Fatal("legacy editor palette changed")
	}
	for _, name := range []string{"main.js", "main.mjs", "main.cjs", "main.ts", "App.vue", "a.json", "a.md", "a.yml", "a.yaml", "a.html", "a.css", "a.scss", "a.less", "a.xml", "a.py", "a.java", "a.go", "a.rs", "a.sh", "a.ps1", "a.sql"} {
		if languageFor(name) == nil {
			t.Errorf("lost legacy language: %s", name)
		}
	}
	line := "function Test { $value = 42 # comment"
	tokens, state := tokenizePowerShell(line, 0, nil)
	if state != 0 || len(tokens) == 0 || tokens[0].Kind != editor.TokenKeyword || tokens[len(tokens)-1].Kind != editor.TokenComment {
		t.Fatal("PowerShell highlighting failed")
	}
	_, state = tokenizePowerShell("@'", 0, nil)
	tokens, state = tokenizePowerShell("<# is string content", state, nil)
	if state != 4 || tokens[0].Kind != editor.TokenString {
		t.Fatal("here-string lost across lines")
	}
}

func TestStaleRequestCannotRestoreLockedWorkspace(t *testing.T) {
	a := fixtureApp(t)
	if err := a.Security.Configure(context.Background(), security.Options{LockEnabled: true, NewPassword: "fixture-password"}); err != nil {
		t.Fatal(err)
	}
	var queued func()
	a.async = func(fn func()) { queued = fn }
	a.perform("pending", func(context.Context) (func(), error) { return func() { a.user = &a.current.Owner }, nil })
	a.lock()
	queued()
	if a.user != nil || a.current != nil || !a.Security.State().Locked {
		t.Fatal("stale request repopulated locked workspace")
	}
}
