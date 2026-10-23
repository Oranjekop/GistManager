# Gist管理器

使用 Go 和 [MyGo](https://github.com/egoist/mygo) 构建的原生 GitHub Gist 桌面客户端，支持 Windows 和 macOS。通过三栏工作区管理 Gist、浏览文件和编辑代码，提供浅色与深色主题、安全锁定及应用内更新。

## 下载与安装

在 [GitHub Releases](https://github.com/Oranjekop/GistManager/releases/latest) 下载适合设备的安装包。当前正式版本为 **v1.0.3**。

| 系统 | 架构 | v1.0.3 安装包 |
| --- | --- | --- |
| Windows 10 / 11 | x64 | `GistManager-Setup-1.0.3-windows-amd64.exe` |
| Windows 10 / 11 | ARM64 | `GistManager-Setup-1.0.3-windows-arm64.exe` |
| macOS 12 及以上 | Intel | `GistManager-1.0.3-darwin-amd64.dmg` |
| macOS 12 及以上 | Apple Silicon | `GistManager-1.0.3-darwin-arm64.dmg` |

Windows 运行安装器，按提示选择目录；macOS 打开 DMG，将应用拖入“应用程序”。安装包内已包含程序运行所需文件。

macOS 包使用 ad-hoc 签名，首次打开可能需要在“系统设置 → 隐私与安全性”中允许。

## 使用

1. 准备具有 Gist 读写权限的 GitHub Personal Access Token，在启动界面连接账号。
2. 在左侧搜索和选择 Gist，在文件栏切换文件。代码默认只读，点击“编辑”后修改并保存。
3. 使用“新建 Gist”“新增文件”和文件操作管理内容，也可复制 Raw 链接或打开 GitHub 页面。
4. 在设置中配置软件密码、启动锁定和系统身份验证，并选择正式版或测试版更新通道。

支持分页加载全部 Gist，按描述、文件名或 ID 搜索，以及创建、编辑、重命名和删除 Gist 文件。原生编辑器提供语法高亮、撤销与重做、查找替换、多光标、折叠和自动换行，保存时保留文件换行格式。

| 操作 | Windows | macOS |
| --- | --- | --- |
| 保存 | `Ctrl+S` | `Cmd+S` |
| 新建 Gist | `Ctrl+N` | `Cmd+N` |
| 刷新 | `Ctrl+R` | `Cmd+R` |
| 锁定 | `Ctrl+Shift+L` | `Cmd+Shift+L` |

## 安全与更新

Token 保存在 Windows 凭据管理器或 macOS Keychain 中。启用锁定后，可通过软件密码或 Windows Hello / PIN、Touch ID 解锁；启用系统身份验证后，每次启动或锁定会自动请求一次验证，取消后仍可手动重试或输入软件密码。系统身份验证是否可用取决于设备配置。锁定会清空工作区并取消进行中的请求。切换、刷新、锁定及退出前会提示处理未保存的修改。

系统设置将程序密码与启动锁定分开管理。首次设置密码后开启启动锁定；修改密码不会改变锁定设置，关闭启动锁定也会保留程序密码。保存解锁方式的变更需要验证当前程序密码。

在“程序密码”区域点击“清除密码”，可选择输入当前密码或使用系统身份验证。忘记密码时，也可在锁定页点击“忘记密码”，通过 Windows Hello / PIN 或 Touch ID 验证后清除，无需输入原密码，也无需事先开启自动身份验证。清除后关闭启动锁定和自动身份验证，保留 GitHub Token 与 Gist 数据；取消或验证失败不会清除密码。

应用会自动检查更新，也可点击“检查更新”选择正式版或测试版通道。发现新版本后，点击“下载更新”，下载适合当前系统和架构的便携包，通过 SHA-256 校验后点击“重启并安装”。安装前会再次向 GitHub 确认发布信息并校验文件，再替换应用并重新启动。未保存的修改会先提示处理；当前版本未提供适用的便携包时，可前往 GitHub 使用安装包更新。

Release 同时提供安装器、DMG、便携 ZIP 和 `SHA256SUMS.txt` 校验文件。Windows 可解压 ZIP 后运行程序；macOS 可解压 ZIP 后将应用移入“应用程序”。

设置保存在系统应用数据目录下的 `Gist管理器/` 中。`--data-dir <目录>` 可指定独立数据目录，开发热重载使用 `Gist管理器-dev/`。

## 开发

安装 [Go 1.27.1 或更新版本](https://go.dev/dl/)，然后执行：

```sh
go mod download
go run .
```

`go tool mygo dev` 启动原生开发热重载。`go run . --demo` 使用隔离的内存演示数据，可体验界面和编辑操作。

```sh
go test ./... -timeout 120s
go vet ./...
```

测试覆盖 GitHub HTTP 接口、原生控件交互、密码清除与恢复、系统验证流程，以及无私钥更新的下载校验和安装前验证。`GIST_SNAPSHOTS` 可指定 UI 测试截图输出目录，`--demo --smoke <输出.png>` 可运行原生窗口、截图并退出。

## 构建与发布

本地构建：

```sh
go run ./cmd/release build -platform windows/amd64,windows/arm64 -version 1.0.3
go run ./cmd/release build -platform darwin/amd64,darwin/arm64 -version 1.0.3
```

输出位于 `dist/<系统>-<架构>/`。Windows 产出 NSIS 安装器；macOS 的应用签名与 DMG 打包需要在 Mac 上执行。

[发布工作流](.github/workflows/native.yml) 在 Windows、macOS Intel 和 Apple Silicon 上运行测试，并构建四个平台的安装包及便携 ZIP。工作流使用 GitHub 自动提供的 `GITHUB_TOKEN` 发布产物，无需配置私钥或额外 Secret。

- 正式版：更新 `mygo.json` 的版本号后，推送匹配的 `v<版本>` 标签，例如 `v1.0.3`。
- 测试版：推送 `main`，发布包含运行编号和提交哈希的滚动 `pre-release`；同一提交已有正式标签时跳过。
- 全部测试、构建和产物校验通过后，才公开 Release。

本地生成便携 ZIP：

```sh
go run ./cmd/release pack -unsigned -platform windows-amd64 -version 1.0.3
go run ./cmd/release pack -unsigned -platform darwin-arm64 -version 1.0.3
```

## 结构

```text
main.go              原生入口与窗口生命周期
mygo.json            版本、图标、打包配置
internal/app/        原生界面、编辑器、工作区与交互测试
internal/gist/       GitHub HTTP 客户端、分页与 Raw 内容
internal/security/   凭据库、密码与系统身份验证
internal/update/     更新检查、SHA-256 校验、下载与安装
cmd/release/         构建与打包工具
resources/           应用资源
```
