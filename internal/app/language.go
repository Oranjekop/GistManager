package app

import (
	"github.com/Oranjekop/GistManager/internal/l10n"
	"github.com/egoist/mygo/ui"
)

func validLanguage(tag string) bool { return tag == "" || tag == "zh" || tag == "en" }

func (a *App) languageSelector(c *ui.Context) {
	options := []string{l10n.Text("跟随系统"), "中文", "English"}
	tags := []string{"", "zh", "en"}
	selected := options[0]
	for i, tag := range tags {
		if a.language == tag {
			selected = options[i]
		}
	}
	ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, l10n.Text("语言")).Bold().Grow(1)
		if ui.Select(c, &selected, options).Width(200).Label(l10n.Text("界面语言")).Disabled(a.settingsBusy).Changed() {
			for i, option := range options {
				if selected == option {
					a.setLanguage(tags[i])
					break
				}
			}
		}
	})
}

func (a *App) setLanguage(tag string) {
	if !validLanguage(tag) || a.settingsBusy {
		return
	}
	a.language = tag
	a.saveLayout()
	a.applyLocale(tag)
}
