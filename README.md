# Gist管理器

使用 Go 和 [MyGo](https://github.com/egoist/mygo) 构建的原生 GitHub Gist 桌面客户端，支持 Windows 和 macOS。通过三栏工作区管理 Gist、浏览文件和编辑代码，提供浅色与深色主题、安全锁定及应用内更新。

## 下载与安装

在 [GitHub Releases](https://github.com/Oranjekop/GistManager/releases/latest) 下载适合设备的安装包。首个正式版本为 **v1.0.0**。

| 系统 | 架构 | v1.0.0 安装包 |
| --- | --- | --- |
| Windows 10 / 11 | x64 | `GistManager-Setup-1.0.0-windows-amd64.exe` |
| Windows 10 / 11 | ARM64 | `GistManager-Setup-1.0.0-windows-arm64.exe` |
| macOS 12 及以上 | Intel | `GistManager-1.0.0-darwin-amd64.dmg` |
| macOS 12 及以上 | Apple Silicon | `GistManager-1.0.0-darwin-arm64.dmg` |

Windows 运行安装器，按提示选择目录；macOS 打开 DMG，将应用拖入“应用程序”。安装包内已包含程序运行所需文件。

macOS 包在未配置 Developer ID 证书和公证凭据时使用 ad-hoc 签名，首次打开可能需要在“系统设置 → 隐私与安全性”中允许。

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

Token 保存在 Windows 凭据管理器或 macOS Keychain 中。启用锁定后，可通过软件密码或 Windows Hello / PIN、Touch ID 解锁；系统身份验证是否可用取决于设备配置。锁定会清空工作区并取消进行中的请求。切换、刷新、锁定及退出前会提示处理未保存的修改。

应用会自动检查更新，也可在界面中手动检查。下载时验证 Ed25519 签名、平台、文件大小和 SHA-256，完成后选择“重启并安装”。安装器和 DMG 用于首次安装，Release 中的 ZIP 与 `native-update-<系统>-<架构>.json` 用于应用内更新。

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

测试覆盖 GitHub HTTP 接口、原生控件交互、安全设置与签名更新。`GIST_SNAPSHOTS` 可指定 UI 测试截图输出目录，`--demo --smoke <输出.png>` 可运行原生窗口、截图并退出。

## 构建与发布

本地构建：

```sh
go run ./cmd/release build -platform windows/amd64,windows/arm64 -version 1.0.0
go run ./cmd/release build -platform darwin/amd64,darwin/arm64 -version 1.0.0
```

输出位于 `dist/<系统>-<架构>/`。Windows 产出 NSIS 安装器；macOS 的应用签名与 DMG 打包需要在 Mac 上执行。

[发布工作流](.github/workflows/native.yml) 在 Windows、macOS Intel 和 Apple Silicon 上运行测试，并构建四个平台的安装包及更新包：

- 正式版：更新 `mygo.json` 的版本号后，推送匹配的 `v<版本>` 标签，例如 `v1.0.0`。
- 测试版：推送 `main`，发布包含运行编号和提交哈希的滚动 `pre-release`；同一提交已有正式标签时跳过。
- 全部测试、构建和产物校验通过后，才公开 Release。

| Actions Secret | 用途 |
| --- | --- |
| `GIST_UPDATE_PRIVATE_KEY` | 必填；与 `resources/update-public.key` 匹配的 Base64 Ed25519 私钥，用于签名更新包 |
| `CSC_LINK`、`CSC_KEY_PASSWORD` | 可选；macOS Developer ID 证书及密码，证书支持 Base64 或 HTTPS 地址 |
| `APPLE_ID`、`APPLE_TEAM_ID`、`APPLE_APP_PASSWORD` | 可选；与 Developer ID 证书一起使用，提交 Apple 公证并附加凭证 |

私钥应安全备份，存放于已被 Git 忽略的 `.keys/update-private.key`。新项目可在公私钥均未配置时执行 `go run ./cmd/release keygen` 生成密钥；签名工具会校验公私钥是否匹配。

```sh
go run ./cmd/release pack -platform windows-amd64 -version 1.0.0
go run ./cmd/release pack -platform darwin-arm64 -version 1.0.0
```

## 结构

```text
main.go              原生入口与窗口生命周期
mygo.json            版本、图标、打包配置
internal/app/        原生界面、编辑器、工作区与交互测试
internal/gist/       GitHub HTTP 客户端、分页与 Raw 内容
internal/security/   凭据库、密码与系统身份验证
internal/update/     更新通道、签名验证、下载与安装
cmd/release/         构建、生成密钥、签名更新包
resources/           图标与更新公钥
```
