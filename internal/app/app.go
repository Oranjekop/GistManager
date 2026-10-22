package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Oranjekop/GistManager/internal/gist"
	"github.com/Oranjekop/GistManager/internal/security"
	"github.com/Oranjekop/GistManager/internal/storage"
	"github.com/Oranjekop/GistManager/internal/update"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type API interface {
	Session(context.Context, string) (gist.User, error)
	List(context.Context, string) ([]gist.Gist, error)
	Get(context.Context, string, string) (gist.Gist, error)
	Create(context.Context, string, gist.Patch) (gist.Gist, error)
	Update(context.Context, string, string, gist.Patch) (gist.Gist, error)
	Delete(context.Context, string, string) error
}

type App struct {
	Icon                                        *ui.Bitmap
	Window                                      *mygo.Window
	API                                         API
	Security                                    *security.Service
	Updates                                     *update.Service
	Version                                     string
	DataDir                                     string
	Demo                                        bool
	post                                        func(func())
	async                                       func(func())
	openURL                                     func(string) error
	copyText                                    func(string)
	quit                                        func()
	user                                        *gist.User
	gists                                       []gist.Gist
	current                                     *gist.Gist
	selected                                    string
	description                                 string
	code                                        *editor.State
	search                                      string
	editing                                     bool
	mode                                        string
	filename                                    string
	public                                      bool
	busy                                        bool
	status                                      string
	issue                                       string
	busyText                                    string
	cancel                                      context.CancelFunc
	generation                                  uint64
	sidebarWidth, filesWidth                    float32
	wrap                                        bool
	tokenDraft, unlockPassword                  string
	connecting                                  bool
	unlocking                                   bool
	biometricAvailable                          bool
	biometricLabel                              string
	settingsOpen                                bool
	settingsForm                                security.Options
	confirmPassword                             string
	settingsIssue                               string
	settingsBusy                                bool
	tokenOpen                                   bool
	renameOpen                                  bool
	renameDraft                                 string
	updateOpen                                  bool
	updateState                                 update.State
	updateBusy                                  bool
	confirmOpen                                 bool
	confirmTitle, confirmMessage, confirmAction string
	confirmed                                   func()
	allowClose                                  bool
	gistList                                    ui.ListState
	gistIndex                                   int
}

func New(api API, secure *security.Service, updater *update.Service, version, dir string) *App {
	a := &App{API: api, Security: secure, Updates: updater, Version: version, DataDir: dir,
		sidebarWidth: 300, filesWidth: 212, wrap: true, mode: "browse", status: "准备就绪"}
	a.async = func(fn func()) { go fn() }
	a.post = func(fn func()) {
		if a.Window != nil && !a.Window.IsDestroyed() {
			a.Window.Update(fn)
		}
	}
	a.openURL = func(location string) error {
		u, err := url.Parse(location)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return errors.New("只能打开有效的 HTTPS 链接")
		}
		return mygo.Shell.OpenExternal(location)
	}
	a.copyText = func(text string) { mygo.Clipboard.WriteText(text) }
	a.quit = func() { mygo.App.Quit() }
	a.resetEditor("")
	if updater != nil {
		a.updateState = updater.State()
	}
	return a
}

func (a *App) Start() {
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		available, label := a.Security.BiometricAvailability(ctx)
		a.post(func() { a.biometricAvailable, a.biometricLabel = available, label })
	})
	if !a.Security.State().Locked {
		a.resume()
	}
	if a.Updates != nil {
		time.AfterFunc(8*time.Second, func() { a.post(func() { a.checkUpdates(false) }) })
	}
}

func (a *App) resetEditor(text string) {
	a.code = &editor.State{Buffer: editor.NewBuffer(text), ReadOnly: !a.editing, SoftWrap: a.wrap}
	name := a.selected
	if a.mode != "browse" {
		name = a.filename
	}
	a.code.Buffer.SetLanguage(languageFor(name))
	a.code.Buffer.MarkSaved()
}

func languageFor(name string) *editor.Language {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".vue") {
		return editor.LanguageFor("file.html")
	}
	if strings.HasSuffix(lower, ".scss") || strings.HasSuffix(lower, ".less") {
		return editor.LanguageFor("style.css")
	}
	if strings.HasSuffix(lower, ".ps1") || strings.HasSuffix(lower, ".psm1") {
		return powerShell
	}
	return editor.LanguageFor(name)
}

func (a *App) editorText() string {
	var text bytes.Buffer
	_, _ = a.code.Buffer.WriteTo(&text)
	return text.String()
}

func (a *App) dirty() bool {
	if a.mode != "browse" {
		return a.code.Buffer.Modified() || a.description != "" || a.filename != "notes.txt" || a.public
	}
	return a.current != nil && (a.code.Buffer.Modified() || a.description != a.current.Description)
}

func (a *App) ask(title, message, action string, fn func()) {
	a.confirmTitle, a.confirmMessage, a.confirmAction = title, message, action
	a.confirmed, a.confirmOpen = fn, true
}

func (a *App) whenClean(fn func()) {
	if a.dirty() {
		a.ask("有未保存的改动", "继续操作会放弃当前编辑内容。", "放弃并继续", func() {
			a.mode, a.editing, a.public = "browse", false, false
			if a.current != nil {
				a.description = a.current.Description
				a.resetEditor(a.current.Files[a.selected].Content)
			} else {
				a.description = ""
				a.resetEditor("")
			}
			fn()
		})
		return
	}
	fn()
}

func (a *App) perform(message string, work func(context.Context) (func(), error)) {
	if a.busy {
		return
	}
	a.busy, a.busyText, a.issue = true, message, ""
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	generation := a.generation
	a.async(func() {
		done, err := work(ctx)
		cancel()
		a.post(func() {
			if generation != a.generation {
				return
			}
			a.busy, a.cancel = false, nil
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.issue = err.Error()
				}
				return
			}
			if done != nil {
				done()
			}
		})
	})
}

func (a *App) resume() {
	a.perform("正在读取账号…", func(ctx context.Context) (func(), error) {
		token, err := a.Security.Token()
		if err != nil {
			return nil, err
		}
		if token == "" {
			return nil, nil
		}
		return a.connect(ctx, token, false)
	})
}

func (a *App) connect(ctx context.Context, token string, save bool) (func(), error) {
	user, err := a.API.Session(ctx, token)
	if err != nil {
		return nil, err
	}
	items, err := a.API.List(ctx, token)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if save {
		if err = a.Security.SaveToken(token); err != nil {
			return nil, err
		}
	}
	return func() {
		a.clearWorkspace()
		a.user, a.gists = &user, items
		a.tokenDraft, a.tokenOpen = "", false
		a.status = fmt.Sprintf("已连接 @%s · %d 个 Gist", user.Login, len(items))
		if len(items) > 0 {
			a.selectGist(items[0].ID)
		}
	}, nil
}

func (a *App) connectDraft() {
	token := strings.TrimSpace(a.tokenDraft)
	if token == "" {
		a.issue = "请输入 GitHub Token"
		return
	}
	a.perform("正在连接 GitHub…", func(ctx context.Context) (func(), error) { return a.connect(ctx, token, true) })
}

func (a *App) refresh() {
	if a.user == nil {
		return
	}
	a.whenClean(func() {
		a.perform("正在刷新 Gist…", func(ctx context.Context) (func(), error) {
			token, err := a.Security.Token()
			if err != nil {
				return nil, err
			}
			items, err := a.API.List(ctx, token)
			if err != nil {
				return nil, err
			}
			return func() {
				a.gists, a.status = items, fmt.Sprintf("已加载 %d 个 Gist", len(items))
				if a.current != nil {
					for _, g := range items {
						if g.ID == a.current.ID {
							a.selectGist(g.ID)
							return
						}
					}
				}
				a.current, a.selected, a.description, a.mode, a.editing = nil, "", "", "browse", false
				a.resetEditor("")
				if len(items) > 0 {
					a.selectGist(items[0].ID)
				}
			}, nil
		})
	})
}

func (a *App) selectGist(id string) {
	if a.busy {
		return
	}
	a.whenClean(func() {
		a.perform("正在加载文件…", func(ctx context.Context) (func(), error) {
			token, err := a.Security.Token()
			if err != nil {
				return nil, err
			}
			g, err := a.API.Get(ctx, token, id)
			if err != nil {
				return nil, err
			}
			return func() { a.showGist(g, ""); a.status = "已打开 " + g.Title() }, nil
		})
	})
}

func (a *App) showGist(g gist.Gist, filename string) {
	a.current, a.description = &g, g.Description
	a.editing, a.mode, a.public = false, "browse", false
	names := g.Filenames()
	if _, ok := g.Files[filename]; !ok {
		filename = ""
		if len(names) > 0 {
			filename = names[0]
		}
	}
	a.selected = filename
	a.resetEditor(g.Files[filename].Content)
	found := false
	for i := range a.gists {
		if a.gists[i].ID == g.ID {
			a.gists[i] = g
			found = true
			break
		}
	}
	if !found {
		a.gists = append([]gist.Gist{g}, a.gists...)
	}
	sort.SliceStable(a.gists, func(i, j int) bool { return a.gists[i].UpdatedAt.After(a.gists[j].UpdatedAt) })
}

func (a *App) selectFile(name string) {
	if a.busy || (a.selected == name && a.mode == "browse") {
		return
	}
	a.whenClean(func() {
		a.selected, a.editing, a.mode = name, false, "browse"
		a.description = a.current.Description
		a.resetEditor(a.current.Files[name].Content)
	})
}

func (a *App) compose(mode string) {
	if a.user == nil || a.busy || (mode == "add" && a.current == nil) {
		return
	}
	a.whenClean(func() {
		a.mode, a.editing, a.filename, a.public = mode, true, "notes.txt", false
		a.description = ""
		a.resetEditor("")
	})
}

func validFilename(name string) bool {
	return strings.TrimSpace(name) != "" && !strings.ContainsAny(name, "\x00\r\n/\\")
}

func (a *App) save() {
	if a.busy || !a.editing {
		return
	}
	mode, filename, description, text := a.mode, a.selected, a.description, a.editorText()
	if mode != "browse" {
		filename = strings.TrimSpace(a.filename)
	}
	if !validFilename(filename) {
		a.issue = "文件名不能为空，也不能包含路径分隔符"
		return
	}
	if mode == "add" {
		if _, exists := a.current.Files[filename]; exists {
			a.issue = "该文件名已存在"
			return
		}
	}
	if mode != "browse" && text == "" {
		a.issue = "GitHub Gist 的新文件内容不能为空"
		return
	}
	patch := gist.Patch{Files: map[string]*gist.FileChange{filename: {Content: &text}}}
	if mode != "add" {
		patch.Description = &description
	}
	patch.Public = mode == "create" && a.public
	id := ""
	if a.current != nil {
		id = a.current.ID
	}
	a.perform("正在保存到 GitHub…", func(ctx context.Context) (func(), error) {
		token, err := a.Security.Token()
		if err != nil {
			return nil, err
		}
		var g gist.Gist
		if mode == "create" {
			g, err = a.API.Create(ctx, token, patch)
		} else {
			g, err = a.API.Update(ctx, token, id, patch)
		}
		if err != nil {
			return nil, err
		}
		return func() { a.showGist(g, filename); a.status = "已保存到 GitHub" }, nil
	})
}

func (a *App) cancelEditing() {
	a.whenClean(func() {
		a.mode, a.editing, a.public = "browse", false, false
		if a.current != nil {
			a.description = a.current.Description
			a.resetEditor(a.current.Files[a.selected].Content)
		} else {
			a.resetEditor("")
		}
	})
}

func (a *App) rename() {
	if a.current == nil || a.busy {
		return
	}
	name := strings.TrimSpace(a.renameDraft)
	if !validFilename(name) {
		a.issue = "请输入有效的文件名"
		return
	}
	if name == a.selected {
		a.renameOpen = false
		return
	}
	if _, exists := a.current.Files[name]; exists {
		a.issue = "该文件名已存在"
		return
	}
	id, old := a.current.ID, a.selected
	patch := gist.Patch{Files: map[string]*gist.FileChange{old: {Filename: name}}}
	a.perform("正在重命名…", func(ctx context.Context) (func(), error) {
		token, err := a.Security.Token()
		if err != nil {
			return nil, err
		}
		g, err := a.API.Update(ctx, token, id, patch)
		if err != nil {
			return nil, err
		}
		return func() { a.renameOpen = false; a.showGist(g, name); a.status = "文件已重命名" }, nil
	})
}

func (a *App) deleteFile() {
	if a.current == nil || a.busy {
		return
	}
	if len(a.current.Files) < 2 {
		a.issue = "最后一个文件不能单独删除，请使用删除 Gist"
		return
	}
	id, name := a.current.ID, a.selected
	a.ask("删除文件", "确认从 GitHub 永久删除 "+name+"？", "永久删除", func() {
		a.perform("正在删除文件…", func(ctx context.Context) (func(), error) {
			token, err := a.Security.Token()
			if err != nil {
				return nil, err
			}
			g, err := a.API.Update(ctx, token, id, gist.Patch{Files: map[string]*gist.FileChange{name: nil}})
			if err != nil {
				return nil, err
			}
			return func() { a.showGist(g, ""); a.status = "文件已删除" }, nil
		})
	})
}

func (a *App) deleteGist() {
	if a.current == nil || a.busy {
		return
	}
	id := a.current.ID
	a.ask("删除 Gist", "确认永久删除这个 Gist 和它的全部文件？此操作无法撤销。", "永久删除", func() {
		a.perform("正在删除 Gist…", func(ctx context.Context) (func(), error) {
			token, err := a.Security.Token()
			if err != nil {
				return nil, err
			}
			if err = a.API.Delete(ctx, token, id); err != nil {
				return nil, err
			}
			return func() {
				items := a.gists[:0]
				for _, g := range a.gists {
					if g.ID != id {
						items = append(items, g)
					}
				}
				a.gists, a.current, a.selected, a.description = items, nil, "", ""
				a.editing, a.mode = false, "browse"
				a.resetEditor("")
				a.status = "Gist 已删除"
				if len(items) > 0 {
					a.selectGist(items[0].ID)
				}
			}, nil
		})
	})
}

func (a *App) clearWorkspace() {
	a.current, a.user, a.gists = nil, nil, nil
	a.description, a.selected, a.mode, a.editing, a.public = "", "", "browse", false, false
	a.tokenDraft, a.unlockPassword, a.search, a.renameDraft = "", "", "", ""
	a.settingsForm, a.confirmPassword = security.Options{}, ""
	a.settingsOpen, a.tokenOpen, a.renameOpen, a.confirmOpen = false, false, false, false
	a.confirmed = nil
	a.resetEditor("")
}

func (a *App) lock() {
	if !a.Security.State().LockEnabled {
		a.openSettings()
		return
	}
	a.whenClean(func() {
		a.generation++
		if a.cancel != nil {
			a.cancel()
			a.cancel = nil
		}
		a.busy = false
		a.Security.Lock()
		a.clearWorkspace()
		a.issue, a.status = "", "应用已锁定"
	})
}

func (a *App) unlock(biometric bool) {
	if a.unlocking {
		return
	}
	password := a.unlockPassword
	a.unlockPassword, a.unlocking, a.issue = "", true, ""
	a.async(func() {
		var err error
		if biometric {
			ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
			defer cancel()
			err = a.Security.UnlockBiometric(ctx)
		} else {
			err = a.Security.Unlock(password)
		}
		a.post(func() {
			a.unlocking = false
			if err != nil {
				a.issue = err.Error()
				return
			}
			a.resume()
		})
	})
}

func (a *App) openSettings() {
	state := a.Security.State()
	a.settingsForm = security.Options{LockEnabled: state.LockEnabled, BiometricEnabled: state.BiometricEnabled}
	a.confirmPassword, a.settingsIssue, a.settingsOpen = "", "", true
}

func (a *App) saveSettings() {
	if a.settingsBusy {
		return
	}
	if a.settingsForm.NewPassword != a.confirmPassword {
		a.settingsIssue = "两次输入的新密码不一致"
		return
	}
	options := a.settingsForm
	a.settingsBusy, a.settingsIssue = true, ""
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := a.Security.Configure(ctx, options)
		a.post(func() {
			a.settingsBusy = false
			if err != nil {
				a.settingsIssue = err.Error()
				return
			}
			a.settingsOpen, a.settingsForm, a.confirmPassword = false, security.Options{}, ""
			a.status = "安全设置已保存"
		})
	})
}

func (a *App) checkUpdates(show bool) {
	if a.Updates == nil || a.updateBusy {
		return
	}
	a.updateOpen = a.updateOpen || show
	a.updateBusy, a.updateState.Status = true, "checking"
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_ = a.Updates.Check(ctx)
		state := a.Updates.State()
		a.post(func() {
			a.updateBusy, a.updateState = false, state
			if state.Status == "available" {
				a.updateOpen = true
			}
		})
	})
}

func (a *App) downloadUpdate() {
	if a.updateBusy {
		return
	}
	a.updateBusy, a.updateState.Status = true, "downloading"
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		_ = a.Updates.Download(ctx, func(state update.State) { a.post(func() { a.updateState = state }) })
		state := a.Updates.State()
		a.post(func() { a.updateBusy, a.updateState = false, state })
	})
}

func (a *App) installUpdate() {
	a.whenClean(func() {
		if a.busy || a.settingsBusy {
			a.issue = "请等待当前操作完成"
			return
		}
		if err := a.Updates.StartInstaller(); err != nil {
			a.updateState.Message = err.Error()
			return
		}
		a.allowClose = true
		a.quit()
	})
}

func (a *App) open(location string) {
	if err := a.openURL(location); err != nil {
		a.issue = err.Error()
	}
}

func (a *App) raw(copy bool) {
	if a.current == nil {
		return
	}
	link := a.current.Files[a.selected].RawURL
	if link == "" {
		a.issue = "此文件暂无 Raw 链接"
		return
	}
	if copy {
		a.copyText(link)
		a.status = "Raw 链接已复制"
	} else {
		a.open(link)
	}
}

func (a *App) RequestClose() bool {
	if a.allowClose {
		return true
	}
	if a.busy || a.settingsBusy {
		a.issue = "请等待当前操作完成后退出"
		return false
	}
	if !a.dirty() {
		a.saveLayout()
		return true
	}
	a.ask("退出前放弃改动？", "当前编辑内容尚未保存到 GitHub。", "放弃并退出", func() { a.allowClose = true; a.saveLayout(); a.quit() })
	return false
}

func (a *App) saveLayout() {
	_ = storage.WriteJSON(filepath.Join(a.DataDir, "layout.json"), map[string]any{"sidebar": a.sidebarWidth, "files": a.filesWidth, "wrap": a.wrap})
}

func (a *App) LoadLayout() {
	var layout struct {
		Sidebar float32 `json:"sidebar"`
		Files   float32 `json:"files"`
		Wrap    *bool   `json:"wrap"`
	}
	data, err := os.ReadFile(filepath.Join(a.DataDir, "layout.json"))
	if err != nil || json.Unmarshal(data, &layout) != nil {
		return
	}
	if layout.Sidebar >= 220 && layout.Sidebar <= 460 {
		a.sidebarWidth = layout.Sidebar
	}
	if layout.Files >= 150 && layout.Files <= 350 {
		a.filesWidth = layout.Files
	}
	if layout.Wrap != nil {
		a.wrap = *layout.Wrap
	}
}
