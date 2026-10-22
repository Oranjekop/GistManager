package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Oranjekop/GistManager/internal/gist"
)

// DemoAPI is used only with --demo and in tests; it never reaches GitHub.
type DemoAPI struct {
	mu    sync.Mutex
	items []gist.Gist
	next  int
}

func NewDemoAPI() *DemoAPI {
	now := time.Now()
	file := func(name, language, content string) gist.File {
		return gist.File{Filename: name, Language: language, Content: content, Size: int64(len(content)), RawURL: "https://gist.githubusercontent.com/demo/raw/" + name}
	}
	return &DemoAPI{next: 3, items: []gist.Gist{
		{ID: "demo-1", Description: "常用开发工具与配置", HTMLURL: "https://gist.github.com/", Public: false, UpdatedAt: now, Files: map[string]gist.File{
			"main.go":       file("main.go", "Go", "package main\n\nimport (\n\t\"fmt\"\n\t\"time\"\n)\n\n// 每一个值得保存的想法，都从一个片段开始。\nfunc main() {\n\tmessage := \"Hello, Gist!\"\n\tfmt.Println(message)\n\tfmt.Println(time.Now().Format(time.RFC3339))\n}\n"),
			"README.md":     file("README.md", "Markdown", "# 开发工具箱\n\n这里保存常用代码与配置。\n\n- 默认只读，点击编辑后修改\n- 支持查找替换和撤销\n- Ctrl+S 保存到 GitHub\n"),
			"settings.json": file("settings.json", "JSON", "{\n  \"editor.fontSize\": 14,\n  \"editor.wordWrap\": \"on\",\n  \"files.autoSave\": \"off\"\n}\n")}},
		{ID: "demo-2", Description: "API 请求示例", HTMLURL: "https://gist.github.com/", Public: true, UpdatedAt: now.Add(-48 * time.Hour), Files: map[string]gist.File{
			"request.py": file("request.py", "Python", "import requests\n\nresponse = requests.get(\n    \"https://api.github.com/gists\",\n    timeout=30,\n)\nresponse.raise_for_status()\nprint(response.json())\n")}},
	}}
}

func (*DemoAPI) Session(context.Context, string) (gist.User, error) {
	return gist.User{Login: "demo", Name: "演示账号"}, nil
}
func (d *DemoAPI) List(context.Context, string) ([]gist.Gist, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	items := make([]gist.Gist, len(d.items))
	for i, g := range d.items {
		items[i] = clone(g)
	}
	return items, nil
}
func (d *DemoAPI) Get(_ context.Context, _, id string) (gist.Gist, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, g := range d.items {
		if g.ID == id {
			return clone(g), nil
		}
	}
	return gist.Gist{}, errors.New("Gist 不存在")
}
func (d *DemoAPI) Create(_ context.Context, _ string, patch gist.Patch) (gist.Gist, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	g := gist.Gist{ID: fmt.Sprintf("demo-%d", d.next), Files: map[string]gist.File{}, HTMLURL: "https://gist.github.com/", Public: patch.Public}
	d.next++
	applyPatch(&g, patch)
	d.items = append([]gist.Gist{g}, d.items...)
	return clone(g), nil
}
func (d *DemoAPI) Update(_ context.Context, _, id string, patch gist.Patch) (gist.Gist, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.items {
		if d.items[i].ID == id {
			applyPatch(&d.items[i], patch)
			return clone(d.items[i]), nil
		}
	}
	return gist.Gist{}, errors.New("Gist 不存在")
}
func (d *DemoAPI) Delete(_ context.Context, _, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.items {
		if d.items[i].ID == id {
			d.items = append(d.items[:i], d.items[i+1:]...)
			return nil
		}
	}
	return errors.New("Gist 不存在")
}
func clone(g gist.Gist) gist.Gist {
	files := make(map[string]gist.File, len(g.Files))
	for n, f := range g.Files {
		files[n] = f
	}
	g.Files = files
	return g
}
func applyPatch(g *gist.Gist, patch gist.Patch) {
	if patch.Description != nil {
		g.Description = *patch.Description
	}
	g.UpdatedAt = time.Now()
	for name, change := range patch.Files {
		if change == nil {
			delete(g.Files, name)
			continue
		}
		file := g.Files[name]
		file.Filename = name
		if change.Filename != "" {
			delete(g.Files, name)
			name, file.Filename = change.Filename, change.Filename
		}
		if change.Content != nil {
			file.Content = *change.Content
			file.Size = int64(len(file.Content))
		}
		file.RawURL = "https://gist.githubusercontent.com/demo/raw/" + name
		g.Files[name] = file
	}
}

type DemoVault struct{ Value string }

func (v *DemoVault) Load() (string, error)   { return v.Value, nil }
func (v *DemoVault) Save(value string) error { v.Value = value; return nil }
func (v *DemoVault) Delete() error           { v.Value = ""; return nil }

type DemoBiometric struct{}

func (DemoBiometric) Available(context.Context) (bool, string) {
	return false, "演示模式使用软件密码"
}
func (DemoBiometric) Verify(context.Context) error {
	return errors.New("演示模式不调用系统身份验证")
}

func (a *App) LoadDemo() {
	a.Demo = true
	api, ok := a.API.(*DemoAPI)
	if !ok {
		return
	}
	user, _ := api.Session(context.Background(), "demo")
	a.user = &user
	a.gists, _ = api.List(context.Background(), "demo")
	for _, name := range a.gists[0].Filenames() {
		if strings.HasSuffix(name, ".go") {
			a.showGist(a.gists[0], name)
			break
		}
	}
	_ = a.Security.SaveToken("demo-token")
	a.status = "演示工作区 · 所有改动仅保存在内存中"
}
