package app

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/Oranjekop/GistManager/internal/security"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func (a *App) View(c *ui.Context) {
	theme := legacyTheme(*c.Theme())
	theme.Radius, theme.FontSize, theme.Spacing = 6, 14, 4
	if runtime.GOOS == "windows" {
		theme.Font = "Microsoft YaHei UI"
	}
	if runtime.GOOS == "darwin" {
		theme.Font = "PingFang SC"
	}
	c.SetTheme(&theme)
	p := colors(theme.Dark)
	ui.Column(c).Fill().Gradient(p.pageTop, p.page, 90).Children(func() {
		a.header(c)
		if a.securityNotice != "" && !a.Security.State().Locked {
			ui.Row(c).Padding(8, 20).Gap(12).Background(tint(c.Theme().Success)).Children(func() {
				ui.Text(c, a.securityNotice).FontSize(12).Grow(1).MinWidth(0).SingleLine().Tooltip(a.securityNotice)
				if ui.Button(c, "知道了").Clicked() {
					a.securityNotice = ""
				}
			})
		}
		if a.Security.State().Locked {
			a.lockScreen(c)
		} else if a.user == nil {
			a.welcome(c)
		} else {
			ui.Split(c, &a.sidebarWidth, func() { a.gistSidebar(c) }, func() {
				ui.Split(c, &a.filesWidth, func() { a.fileSidebar(c) }, func() { a.workspace(c) }).Fill()
			}).Grow(1).FillWidth()
		}
		a.statusbar(c)
	})
	if !a.Security.State().Locked {
		if !a.recoveryOpen && !a.passwordOpen {
			a.settingsDialog(c)
		}
		if !a.recoveryOpen {
			a.passwordDialog(c)
		}
		a.tokenDialog(c)
		a.renameDialog(c)
	}
	a.passwordRecoveryDialog(c)
	a.updateDialog(c)
	if result := ui.AlertDialog(c, &a.confirmOpen, a.confirmTitle, a.confirmMessage, "取消", a.confirmAction); result == 1 {
		fn := a.confirmed
		a.confirmed = nil
		if fn != nil {
			fn()
		}
	} else if result == ui.AlertDismissed {
		a.confirmed = nil
	}
	if a.user != nil && !a.Security.State().Locked {
		if c.Shortcut(ui.Cmd, ui.KeyS) {
			a.save()
		}
		if c.Shortcut(ui.Cmd, ui.KeyN) {
			a.compose("create")
		}
		if c.Shortcut(ui.Cmd, ui.KeyR) {
			a.refresh()
		}
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyL) {
			a.lock()
		}
	}
}

func (a *App) header(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(68).Shrink(0).Padding(12, 20).Gap(14).Background(colors(t.Dark).panel).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if a.Icon != nil {
			ui.Image(c, a.Icon).Size(38, 38).Fit(ui.Contain)
		} else {
			ui.Box(c).Size(38, 38).Radius(8).Background(t.Accent).Center().Children(func() { ui.Text(c, "G/").Bold().FontSize(23).TextColor(t.AccentText) })
		}
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, "Gist管理器").Bold().FontSize(18)
			label := "代码片段 · 触手可及"
			if a.Demo {
				label = "演示数据 · 不连接真实账号"
			}
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		})
		ui.Spacer(c)
		if a.user != nil && !a.Security.State().Locked {
			ui.Text(c, "@"+a.user.Login).TextColor(t.TextMuted).MaxWidth(180).MinWidth(0).SingleLine().Tooltip("@" + a.user.Login)
		}
		if ui.Button(c, "检查更新").Clicked() {
			a.checkUpdates(true)
		}
		if !a.Security.State().Locked {
			if ui.Button(c, "系统设置").Disabled(a.settingsBusy).Clicked() {
				a.openSettings()
			}
			lockLabel := "启用锁定"
			if a.Security.State().LockEnabled {
				lockLabel = "立即锁定"
			}
			if ui.Button(c, lockLabel).Disabled(a.settingsBusy || a.recoveryBusy).Clicked() {
				a.lock()
			}
		}
	})
}

func (a *App) welcome(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Children(func() {
		ui.Column(c).Width(520).MaxWidthPercent(100).Padding(36).Gap(22).Radius(16).Border(1, t.Border).Background(t.Background).
			Shadow(0, 12, 45, 0, ui.RGBA(10, 40, 70, 0.06)).Children(func() {
			ui.Text(c, "你的代码，妥善归档。").FontSize(27).Bold()
			ui.Text(c, "连接 GitHub，浏览、编辑和整理你的全部 Gist。").TextColor(t.TextMuted)
			ui.Field(c, "GitHub Personal Access Token", func() {
				if legacyInput(c, &a.tokenDraft).Password().Placeholder("github_pat_… / ghp_…").Label("GitHub Token").Disabled(a.busy).Submitted() {
					a.connectDraft()
				}
			}).Description("需要 gist 权限。Token 仅用于请求 GitHub，并保存到系统凭据库。")
			if a.issue != "" {
				ui.Text(c, a.issue).TextColor(t.Danger)
			}
			label := "连接 GitHub"
			if a.busy {
				label = "正在连接…"
			}
			if ui.PrimaryButton(c, label).Disabled(a.busy).Clicked() {
				a.connectDraft()
			}
			if ui.Button(c, "前往 GitHub 创建 Token").Clicked() {
				a.open("https://github.com/settings/tokens/new?scopes=gist&description=GistManager")
			}
			ui.Text(c, "连接后即可搜索、编辑和管理 Gist，随时可以更换账号。").FontSize(12).TextColor(t.TextMuted)
		})
	})
}

func (a *App) lockScreen(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Children(func() {
		ui.Column(c).Width(440).MaxWidthPercent(100).Gap(20).Padding(32).Radius(16).Border(1, t.Border).Background(colors(t.Dark).panel).Label("解锁面板").Children(func() {
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, "工作区已锁定").FontSize(26).Bold()
				ui.Text(c, "验证身份后，继续管理你的代码片段。").FontSize(13).TextColor(t.TextMuted)
			})
			if a.Security.State().BiometricEnabled && a.biometricAvailable {
				if ui.Button(c, "使用 "+a.biometricName()).FillWidth().Disabled(a.unlocking).Clicked() {
					a.unlock(true)
				}
				ui.Text(c, "或使用程序密码").FontSize(12).TextColor(t.TextMuted)
			}
			ui.Row(c).Justify(ui.SpaceBetween).Children(func() {
				ui.Text(c, "程序密码").FontSize(13)
				if a.biometricAvailable {
					if ui.Button(c, "忘记密码").FontSize(12).TextColor(t.Accent).Disabled(a.unlocking || a.recoveryBusy).Clicked() {
						a.openPasswordRecovery()
					}
				}
			})
			if legacyInput(c, &a.unlockPassword).Password().Placeholder("输入程序密码").Label("输入程序密码").Disabled(a.unlocking).AutoFocus().Submitted() {
				a.unlock(false)
			}
			if a.issue != "" {
				ui.Text(c, a.issue).FontSize(12).TextColor(t.Danger)
			}
			if ui.PrimaryButton(c, "解锁").FillWidth().Disabled(a.unlocking).Clicked() {
				a.unlock(false)
			}
			if a.unlocking {
				ui.Progress(c, -1).Label("正在验证身份")
			}
		})
	})
}

func (a *App) filtered() []int {
	query := strings.ToLower(strings.TrimSpace(a.search))
	result := make([]int, 0, len(a.gists))
	for i, g := range a.gists {
		text := g.Description + " " + g.ID + " " + strings.Join(g.Filenames(), " ")
		if query == "" || strings.Contains(strings.ToLower(text), query) {
			result = append(result, i)
		}
	}
	return result
}

func (a *App) gistSidebar(c *ui.Context) {
	t := c.Theme()
	p := colors(t.Dark)
	ui.Column(c).Fill().Background(p.panel).Children(func() {
		ui.Column(c).Padding(16).Gap(12).Shrink(0).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, "全部 Gist").Bold().Grow(1)
				ui.Badge(c, fmt.Sprint(len(a.gists)))
				if ui.Button(c, "刷新").Disabled(a.busy).Clicked() {
					a.refresh()
				}
			})
			legacyInput(c, &a.search).Background(p.input).Placeholder("搜索描述、文件名或 ID").Label("搜索 Gist")
			if ui.PrimaryButton(c, "新建 Gist").Disabled(a.busy).Clicked() {
				a.compose("create")
			}
		})
		visible := a.filtered()
		a.gistIndex = -1
		for i, index := range visible {
			if a.current != nil && a.gists[index].ID == a.current.ID {
				a.gistIndex = i
			}
		}
		a.gistList.Selected = &a.gistIndex
		a.gistList.Key = func(i int) any { return a.gists[visible[i]].ID }
		a.gistList.Label = func(i int) string { return a.gists[visible[i]].Title() }
		if len(visible) == 0 {
			ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Gap(10).Label("Gist 搜索空状态").Children(func() {
				message, hint := "还没有 Gist", "创建第一个片段，开始整理代码。"
				if strings.TrimSpace(a.search) != "" {
					message, hint = "没有匹配的 Gist", "试试其他描述、文件名或 ID。"
				}
				ui.Text(c, message).Bold()
				ui.Text(c, hint).TextColor(t.TextMuted).FontSize(12)
				if a.search != "" {
					if ui.Button(c, "清空搜索").Clicked() {
						a.search = ""
					}
				}
			})
		} else {
			listTheme := *t
			// The card paints its own hover and selection. Transparent row fills
			// avoid a second background leaking outside the card's rounded corners.
			listTheme.Radius = 10
			listTheme.Accent, listTheme.SurfaceHover, listTheme.AccentText = ui.Color{}, ui.Color{}, p.text
			c.SetTheme(&listTheme)
			list := ui.List(c, &a.gistList, len(visible), func(i int) {
				g := a.gists[visible[i]]
				muted := t.TextMuted
				selected := a.current != nil && a.current.ID == g.ID
				card := ui.Column(c).Padding(14).Gap(7).MinHeight(96).FillWidth().Radius(10).Label("Gist 卡片 " + g.ID)
				background, border := p.card, p.border
				if selected {
					background, border = p.card.Mix(p.accent, .1), p.accent.Alpha(.6)
				} else if card.Hovered() && !a.busy {
					background, border = p.card.Mix(p.accent, .055), p.accent.Alpha(.35)
				}
				card.Background(background).Border(1, border).Children(func() {
					ui.Text(c, g.Title()).TextColor(p.text).Bold().SingleLine().Tooltip(g.Title())
					names := strings.Join(g.Filenames(), ", ")
					ui.Text(c, names).FontSize(12).TextColor(muted).SingleLine().Tooltip(names)
					visibility := "私密"
					if g.Public {
						visibility = "公开"
					}
					ui.Text(c, fmt.Sprintf("%s · %d 个文件 · %s", visibility, len(g.Files), relativeDate(g.UpdatedAt))).FontSize(11).TextColor(muted).SingleLine()
				})
			}).Grow(1).Label("Gist 列表").Disabled(a.busy).Gap(8).Padding(0, 16, 12)
			c.SetTheme(t)
			if list.Changed() && a.gistIndex >= 0 && a.gistIndex < len(visible) {
				a.selectGist(a.gists[visible[a.gistIndex]].ID)
			}
		}
		ui.Row(c).Padding(12, 16).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, fmt.Sprintf("%d / %d 个 Gist", len(visible), len(a.gists))).FontSize(11).TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, "更换 Token").Disabled(a.busy).Clicked() {
				a.whenClean(func() { a.tokenOpen, a.tokenDraft, a.issue = true, "", "" })
			}
		})
	})
}

func (a *App) fileSidebar(c *ui.Context) {
	t := c.Theme()
	p := colors(t.Dark)
	ui.Column(c).Fill().Background(p.panel).Padding(16).Gap(16).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "文件").Bold().Grow(1)
			if a.current != nil {
				ui.Badge(c, fmt.Sprint(len(a.current.Files)))
			}
		})
		if a.current == nil {
			ui.Text(c, "选择一个 Gist").TextColor(t.TextMuted)
		} else {
			ui.Scroll(c).Grow(1).Label("文件列表").Children(func() {
				fileTheme := *t
				fileTheme.Accent, fileTheme.AccentText = p.muted.Mix(p.accent, .1), p.text
				c.SetTheme(&fileTheme)
				ui.Tree(c, func() {
					for _, name := range a.current.Filenames() {
						c.Key(name)
						chosen := a.selected == name && a.mode == "browse"
						border := ui.Color{}
						if chosen {
							border = p.accent.Alpha(.55)
						}
						if ui.TreeItem(c, name, nil, nil).Selected(chosen).Border(1, border).Height(40).Radius(8).Tooltip(name).Disabled(a.busy).Clicked() {
							a.selectFile(name)
						}
					}
				}).Gap(6)
				c.SetTheme(t)
			})
			if ui.Button(c, "新增文件").Disabled(a.busy).Clicked() {
				a.compose("add")
			}
			if ui.Button(c, "在 GitHub 中打开").Clicked() {
				a.open(a.current.HTMLURL)
			}
			if ui.Button(c, "删除 Gist").TextColor(t.Danger).Disabled(a.busy).Clicked() {
				a.deleteGist()
			}
		}
	})
}

func (a *App) workspace(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(colors(t.Dark).panel).Children(func() {
		if a.current == nil && a.mode == "browse" {
			ui.Column(c).Grow(1).FillWidth().Center().Gap(14).Children(func() {
				ui.Text(c, "让片段各得其所").Bold().FontSize(26)
				ui.Text(c, "从左侧选择 Gist，或开始创建新的代码片段。").TextColor(t.TextMuted)
				if ui.PrimaryButton(c, "创建第一个 Gist").Disabled(a.busy).Clicked() {
					a.compose("create")
				}
			})
			return
		}
		ui.Column(c).Padding(16, 20).Gap(12).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Shrink(0).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				title := a.selected
				if a.mode == "create" {
					title = "新建 Gist"
				}
				if a.mode == "add" {
					title = "新增文件"
				}
				ui.Text(c, title).Bold().FontSize(18).SingleLine().Grow(1).MinWidth(0).Tooltip(title)
				if a.editing {
					if ui.Button(c, "取消编辑").Disabled(a.busy).Clicked() {
						a.cancelEditing()
					}
					label := "保存修改"
					if a.mode != "browse" {
						label = "创建"
					}
					if ui.PrimaryButton(c, label).Disabled(a.busy || (a.mode == "browse" && !a.dirty())).Clicked() {
						a.save()
					}
				} else {
					ui.Badge(c, "只读")
					if ui.PrimaryButton(c, "编辑").Disabled(a.busy).Clicked() {
						a.editing = true
					}
				}
			})
			if a.mode != "browse" {
				if legacyInput(c, &a.filename).Label("文件名").Placeholder("文件名，例如 main.go").Disabled(a.busy).Changed() {
					a.code.Buffer.SetLanguage(languageFor(a.filename))
				}
			}
			if a.mode != "add" {
				if a.editing {
					legacyInput(c, &a.description).Label("Gist 描述").Placeholder("为这个 Gist 添加描述").Disabled(a.busy)
				} else {
					description := strings.TrimSpace(a.description)
					if description == "" {
						description = "暂无描述"
					}
					ui.Text(c, description).Label("Gist 描述").TextColor(t.TextMuted).FontSize(12).MinWidth(0).SingleLine().Tooltip(description)
				}
			}
			if a.mode == "create" {
				ui.Checkbox(c, &a.public, "公开 Gist（所有人可搜索）")
				ui.Text(c, "私密 Gist 仅隐藏于搜索，持有链接的人仍可访问。").TextColor(t.TextMuted).FontSize(12)
			}
		})
		a.code.ReadOnly, a.code.SoftWrap, a.code.NoScrollPastEnd, a.code.Label = !a.editing || a.busy, a.wrap, true, "代码编辑器"
		a.code.Theme = codeTheme
		editor.View(c, a.code).Grow(1).FillWidth().MinHeight(100)
		ui.Row(c).Padding(10, 16).Gap(6).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Shrink(0).Children(func() {
			if a.mode == "browse" {
				if ui.Button(c, "复制 Raw").FontSize(12).Clicked() {
					a.raw(true)
				}
				if ui.Button(c, "打开 Raw").FontSize(12).Clicked() {
					a.raw(false)
				}
				if ui.Button(c, "重命名").FontSize(12).Disabled(a.busy).Clicked() {
					a.whenClean(func() { a.renameDraft, a.renameOpen, a.issue = a.selected, true, "" })
				}
				if ui.Button(c, "删除文件").FontSize(12).Disabled(a.busy).TextColor(t.Danger).Clicked() {
					a.deleteFile()
				}
			}
			ui.Spacer(c)
			ui.Checkbox(c, &a.wrap, "自动换行")
		})
	})
}

func (a *App) statusbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(32).Shrink(0).Padding(5, 16).Gap(10).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Background(t.Surface).Children(func() {
		message, color := a.status, t.TextMuted
		if a.busy {
			message = a.busyText
			ui.Spinner(c).Size(14, 14)
		}
		if a.issue != "" {
			message, color = a.issue, t.Danger
		}
		ui.Text(c, message).FontSize(11).TextColor(color).Grow(1).MinWidth(0).SingleLine().Tooltip(message)
		if a.current != nil && !a.Security.State().Locked {
			state := "已保存"
			if a.dirty() {
				state = "未保存"
			}
			ui.Text(c, state).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Text(c, "v"+a.Version).FontSize(11).TextColor(t.TextMuted)
	})
}

func (a *App) settingsDialog(c *ui.Context) {
	t := c.Theme()
	state := a.Security.State()
	modal := ui.Modal(c, &a.settingsOpen, func() {
		ui.Column(c).Width(460).MaxWidthPercent(100).Gap(16).Children(func() {
			ui.Column(c).Gap(5).Children(func() {
				ui.Text(c, "系统设置").FontSize(22).Bold()
				ui.Text(c, "管理程序密码与工作区锁定").FontSize(12).TextColor(t.TextMuted)
			})
			ui.Column(c).Padding(16).Gap(12).Radius(10).Border(1, t.Border).Background(colors(t.Dark).card).Label("程序密码设置").Children(func() {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
						ui.Text(c, "程序密码").Bold()
						hint := "未设置 · 设置后可启用锁定"
						if state.HasPassword {
							hint = "已设置 · 用于本应用解锁"
						}
						ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
					})
					passwordLabel := "设置密码"
					if state.HasPassword {
						passwordLabel = "修改密码"
					}
					if ui.Button(c, passwordLabel).Disabled(a.settingsBusy).Clicked() {
						a.openPasswordSettings()
					}
					if state.HasPassword {
						if ui.Button(c, "清除密码").TextColor(t.Danger).Disabled(a.settingsBusy).Clicked() {
							a.openPasswordRecovery()
						}
					}
				})
			})
			ui.Column(c).Padding(16).Gap(16).Radius(10).Border(1, t.Border).Background(colors(t.Dark).card).Label("启动解锁设置").Children(func() {
				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
						ui.Text(c, "启动锁定").Bold()
						hint := "每次启动先验证身份，再显示工作区"
						if !state.HasPassword {
							hint = "请先设置程序密码"
						}
						ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
					})
					if ui.Switch(c, &a.settingsForm.LockEnabled).Label("启动时锁定应用").Disabled(a.settingsBusy || !state.HasPassword).Changed() {
						if !a.settingsForm.LockEnabled {
							a.settingsForm.BiometricEnabled = false
						}
						a.settingsIssue, a.settingsNotice, a.settingsForm.CurrentPassword = "", "", ""
					}
				})
				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
						ui.Text(c, "系统身份验证").Bold()
						hint := "解锁时自动请求 " + a.biometricName()
						if !a.biometricAvailable {
							hint = "此设备暂不可用，可使用程序密码"
						} else if !a.settingsForm.LockEnabled {
							hint = "开启启动锁定后可用"
						}
						ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
					})
					if ui.Switch(c, &a.settingsForm.BiometricEnabled).Label("自动使用系统身份验证").Disabled(a.settingsBusy || !a.settingsForm.LockEnabled || !a.biometricAvailable && !a.settingsForm.BiometricEnabled).Changed() {
						a.settingsIssue, a.settingsNotice = "", ""
					}
				})
			})
			if state.HasPassword && a.settingsChanged() {
				ui.Field(c, "当前程序密码", func() {
					if legacyInput(c, &a.settingsForm.CurrentPassword).Password().Label("当前程序密码").Placeholder("验证后保存解锁设置").Disabled(a.settingsBusy).Submitted() {
						a.saveSettings()
					}
				})
			}
			if a.settingsIssue != "" {
				ui.Text(c, a.settingsIssue).FontSize(12).TextColor(t.Danger)
			}
			if a.settingsNotice != "" {
				ui.Text(c, a.settingsNotice).FontSize(12).TextColor(t.Success)
			}
			ui.Text(c, "关闭启动锁定会保留程序密码；清除密码需单独验证。").FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "关闭").Disabled(a.settingsBusy).Clicked() {
					a.settingsOpen = false
				}
				label := "保存设置"
				if a.settingsBusy {
					label = "正在保存…"
				}
				if ui.PrimaryButton(c, label).Disabled(a.settingsBusy || !a.settingsChanged()).Clicked() {
					a.saveSettings()
				}
			})
		})
	}).Label("系统设置对话框")
	if modal.Dismissed() && a.settingsBusy {
		a.settingsOpen = true
	}
	if !a.settingsOpen && !a.settingsBusy {
		a.settingsForm = security.Options{}
	}
}

func (a *App) passwordDialog(c *ui.Context) {
	t := c.Theme()
	hasPassword := a.Security.State().HasPassword
	modal := ui.Modal(c, &a.passwordOpen, func() {
		ui.Column(c).Width(460).MaxWidthPercent(100).Gap(16).Children(func() {
			title := "设置程序密码"
			if hasPassword {
				title = "修改程序密码"
			}
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, title).FontSize(22).Bold()
				ui.Text(c, "此密码仅用于解锁本应用，与 GitHub 密码无关。").FontSize(12).TextColor(t.TextMuted)
			})
			if hasPassword {
				ui.Field(c, "当前程序密码", func() {
					legacyInput(c, &a.passwordForm.CurrentPassword).Password().Label("当前程序密码").Disabled(a.settingsBusy).AutoFocus()
				})
			}
			ui.Field(c, "新密码", func() {
				input := legacyInput(c, &a.passwordForm.NewPassword).Password().Label("新密码").Placeholder("至少 6 位").Disabled(a.settingsBusy)
				if !hasPassword {
					input.AutoFocus()
				}
			})
			ui.Field(c, "确认新密码", func() {
				if legacyInput(c, &a.passwordConfirm).Password().Label("确认新密码").Placeholder("再次输入新密码").Disabled(a.settingsBusy).Submitted() {
					a.savePassword()
				}
			})
			if a.passwordIssue != "" {
				ui.Text(c, a.passwordIssue).FontSize(12).TextColor(t.Danger)
			}
			hint := "更新密码后，现有的锁定与身份验证设置保持不变。"
			if !hasPassword {
				hint = "设置密码后将开启启动锁定，随时可以在设置中关闭。"
			}
			ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "返回设置").Disabled(a.settingsBusy).Clicked() {
					a.passwordOpen = false
				}
				label := "保存新密码"
				if !hasPassword {
					label = "设置并开启锁定"
				}
				if a.settingsBusy {
					label = "正在保存…"
				}
				if ui.PrimaryButton(c, label).Disabled(a.settingsBusy).Clicked() {
					a.savePassword()
				}
			})
		})
	}).Label("程序密码对话框")
	if modal.Dismissed() && a.settingsBusy {
		a.passwordOpen = true
	}
	if !a.passwordOpen && !a.settingsBusy {
		a.passwordForm, a.passwordConfirm, a.passwordIssue = security.Options{}, "", ""
	}
}

func (a *App) biometricName() string {
	if a.biometricLabel != "" {
		return a.biometricLabel
	}
	return "系统身份验证"
}

func tint(color ui.Color) ui.Color { return color.Alpha(.08) }

func (a *App) passwordRecoveryDialog(c *ui.Context) {
	t := c.Theme()
	modal := ui.Modal(c, &a.recoveryOpen, func() {
		ui.Column(c).Width(460).MaxWidthPercent(100).Gap(16).Children(func() {
			ui.Text(c, "清除程序密码").Bold().FontSize(22)
			ui.Column(c).Padding(14).Gap(6).Background(tint(t.Danger)).Border(1, t.Danger.Alpha(.18)).Radius(8).Children(func() {
				ui.Text(c, "启动锁定和自动身份验证将一并关闭").Bold().FontSize(13)
				ui.Text(c, "GitHub Token 和 Gist 数据会保留。之后可重新设置密码。").FontSize(12).TextColor(t.TextMuted)
			})
			if !a.Security.State().Locked && a.biometricAvailable {
				parts := ui.SegmentedBase(c, &a.recoveryMethod, 2)
				parts.Track.FillWidth().Padding(3).Gap(4).Radius(8).Background(t.Surface).Border(1, t.Border).Label("清除密码验证方式").Disabled(a.recoveryBusy).Children(func() {
					for i, label := range []string{"当前密码", "系统身份验证"} {
						segment := parts.Segment(i).Grow(1).Padding(6, 12).Radius(5)
						if a.recoveryMethod == i {
							segment.Background(colors(t.Dark).input).Border(1, t.Border).TextColor(t.Accent)
						}
						segment.Children(func() { ui.Text(c, label).SingleLine().FontSize(13) })
					}
				})
				if parts.Track.Changed() {
					a.recoveryPassword, a.recoveryIssue = "", ""
				}
			}
			if a.recoveryMethod == 0 {
				ui.Field(c, "当前程序密码", func() {
					if legacyInput(c, &a.recoveryPassword).Password().Label("当前程序密码").Placeholder("输入当前程序密码以确认").Disabled(a.recoveryBusy).AutoFocus().Submitted() {
						a.recoverPassword()
					}
				})
			} else {
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, "使用 "+a.biometricName()).Bold()
					ui.Text(c, "无需原密码，完成系统身份验证后即可清除。").FontSize(12).TextColor(t.TextMuted)
				})
			}
			if a.recoveryIssue != "" {
				ui.Text(c, a.recoveryIssue).FontSize(12).TextColor(t.Danger)
			}
			if a.recoveryBusy {
				ui.Progress(c, -1).Label("正在验证并清除密码")
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Disabled(a.recoveryBusy).Clicked() {
					a.recoveryOpen = false
				}
				label := "验证并清除密码"
				if a.recoveryBusy {
					label = "正在验证…"
				}
				if ui.Button(c, label).Background(tint(t.Danger)).TextColor(t.Danger).Border(1, t.Danger.Alpha(.3)).Disabled(a.recoveryBusy).Clicked() {
					a.recoverPassword()
				}
			})
		})
	}).Label("密码恢复对话框")
	if modal.Dismissed() && a.recoveryBusy {
		a.recoveryOpen = true
	}
	if !a.recoveryOpen && !a.recoveryBusy {
		a.recoveryPassword, a.recoveryIssue = "", ""
	}
}

func (a *App) tokenDialog(c *ui.Context) {
	modal := ui.Modal(c, &a.tokenOpen, func() {
		ui.Column(c).Width(460).Gap(16).Children(func() {
			ui.Text(c, "更换 GitHub Token").Bold().FontSize(22)
			ui.Text(c, "新账号验证成功后将切换工作区。").TextColor(c.Theme().TextMuted)
			if legacyInput(c, &a.tokenDraft).Password().Label("新 Token").Placeholder("github_pat_… / ghp_…").Disabled(a.busy).AutoFocus().Submitted() {
				a.connectDraft()
			}
			if a.issue != "" {
				ui.Text(c, a.issue).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Disabled(a.busy).Clicked() {
					a.tokenOpen, a.tokenDraft = false, ""
				}
				if ui.PrimaryButton(c, "连接并切换").Disabled(a.busy).Clicked() {
					a.connectDraft()
				}
			})
		})
	})
	if modal.Dismissed() {
		if a.busy {
			a.tokenOpen = true
		} else {
			a.tokenDraft = ""
		}
	}
}

func (a *App) renameDialog(c *ui.Context) {
	ui.Modal(c, &a.renameOpen, func() {
		ui.Column(c).Width(420).Gap(16).Children(func() {
			ui.Text(c, "重命名文件").Bold().FontSize(22)
			if legacyInput(c, &a.renameDraft).Label("新文件名").AutoFocus().Disabled(a.busy).Submitted() {
				a.rename()
			}
			if a.issue != "" {
				ui.Text(c, a.issue).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Disabled(a.busy).Clicked() {
					a.renameOpen = false
				}
				if ui.PrimaryButton(c, "确认重命名").Disabled(a.busy).Clicked() {
					a.rename()
				}
			})
		})
	})
}

func (a *App) updateDialog(c *ui.Context) {
	ui.Modal(c, &a.updateOpen, func() {
		ui.Column(c).Width(440).MaxWidthPercent(100).Gap(16).Children(func() {
			ui.Text(c, "软件更新").Bold().FontSize(22)
			ui.Text(c, "当前版本 "+a.Version).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(8).Children(func() {
				for _, option := range []struct{ label, channel string }{{"正式版", "stable"}, {"测试版", "beta"}} {
					button := ui.Button(c, option.label).Disabled(a.updateBusy)
					if a.updateState.Channel == option.channel {
						button.Border(2, c.Theme().Accent)
					}
					if button.Clicked() && a.Updates != nil {
						if err := a.Updates.SetChannel(option.channel); err != nil {
							a.updateState.Message = err.Error()
						} else {
							a.updateState = a.Updates.State()
							a.checkUpdates(true)
						}
					}
				}
			})
			message := a.updateState.Message
			if a.updateState.Status == "checking" {
				message = "正在检查更新…"
			}
			if message != "" {
				ui.Text(c, message)
			}
			if a.updateState.Status == "downloading" {
				fraction := float64(0)
				if a.updateState.Total > 0 {
					fraction = float64(a.updateState.Downloaded) / float64(a.updateState.Total)
				}
				ui.Progress(c, fraction).Label("更新下载进度")
				ui.Text(c, fmt.Sprintf("%.1f / %.1f MB", float64(a.updateState.Downloaded)/(1<<20), float64(a.updateState.Total)/(1<<20)))
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if a.updateState.Status == "error" {
					if ui.Button(c, "在 GitHub 查看").Clicked() {
						a.open(a.Updates.DownloadPage())
					}
				}
				if ui.Button(c, "关闭").Clicked() {
					a.updateOpen = false
				}
				if a.updateState.Status == "available" {
					if ui.PrimaryButton(c, "下载更新").Disabled(a.updateBusy).Clicked() {
						a.downloadUpdate()
					}
				} else if a.updateState.Status == "downloaded" {
					if ui.PrimaryButton(c, "重启并安装").Clicked() {
						a.installUpdate()
					}
				} else if a.updateState.Status == "manual" {
					if ui.PrimaryButton(c, "前往 GitHub 下载").Clicked() {
						a.open(a.Updates.DownloadPage())
					}
				} else if ui.PrimaryButton(c, "检查更新").Disabled(a.updateBusy).Clicked() {
					a.checkUpdates(true)
				}
			})
		})
	}).Label("软件更新对话框")
}

func relativeDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	days := int(time.Since(value).Hours() / 24)
	if days == 0 {
		return "今天"
	}
	if days == 1 {
		return "昨天"
	}
	if days < 30 {
		return fmt.Sprintf("%d 天前", days)
	}
	return value.Format("2006-01-02")
}
