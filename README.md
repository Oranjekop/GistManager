# Gist管理器

一个基于 `Vue 3 + Electron` 的 GitHub Gist 单体桌面客户端。

程序启动后直接以客户端形态运行，界面通过 Electron 主进程直接访问 GitHub API，不再依赖单独的本地后端服务或额外端口。

## 功能特性

- 通过 GitHub Token 管理当前账号下全部 Gist
- 首次启动输入 Token，后续自动连接
- 支持随时更换 Token
- 浏览 Gist 列表、文件树和文件详情
- 创建 Gist、添加文件、重命名文件、删除文件、删除 Gist
- 集成 Monaco 编辑器，适合代码和配置文件编辑
- 编辑器默认查看模式，点击“编辑”后才允许修改，减少误操作
- 支持复制 Raw 链接、打开 Raw 链接、在网页中打开当前 Gist
- 支持启动锁定、软件解锁密码与手动立即锁定
- Windows 使用 Windows Hello / PIN，macOS 使用 Touch ID，不可用时自动回退到软件密码
- GitHub Token 通过 Electron `safeStorage` 使用系统凭据加密后保存
- 支持自动与手动检查更新、下载进度显示以及重启安装
- 可在正式版与测试版更新通道之间切换，并记住用户选择
- 支持 Windows 与 macOS 安装包打包
- 推送版本标签自动构建正式版，推送 `main` 提交自动构建滚动测试版

## 技术栈

- `Vue 3`
- `Vite`
- `Electron`
- `Monaco Editor`

## 项目结构

```text
.
├─ client/          界面代码
├─ electron/        Electron 主进程、预加载桥与 GitHub 客户端能力
├─ build/           图标与打包资源
├─ release/         打包输出目录
└─ README.md
```

## 运行方式

### 安装依赖

```bash
npm install
npm install --prefix client
```

### 启动桌面版

```bash
npm run dev:desktop
```

启动后会自动：

- 启动 Vite 前端开发服务
- 打开 Electron 桌面客户端
- 由 Electron 直接处理 GitHub Gist 请求

## 打包

```bash
npm run dist:win
npm run dist:mac
```

打包完成后，安装包输出到：

```text
release/GistManager-Setup-1.0.1-x64.exe
```

## 自动构建

- 推送 `v1.2.3` 形式的 tag：构建 Windows x64/arm64 与 macOS x64/arm64 正式版，并发布对应 GitHub Release。
- 推送到 `main`：生成带提交短哈希的 beta 版本，并覆盖 `pre-release` 滚动测试版。
- 同一提交已经存在正式版本 tag 时，会跳过测试版构建。
- Release 会同时上传安装包、差分更新 blockmap 和更新通道 YAML 元数据。

## 软件更新

应用启动约 8 秒后会自动检查一次更新，也可以点击右上角“检查更新”手动检查。

- `正式版`：只接收稳定 GitHub Release。
- `测试版`：接收标记为 Pre-release 的 beta 版本。
- 检测到更新后由用户确认下载；下载完成后点击“重启并安装”才会退出应用并启动安装。
- 测试版切换回正式版时，允许回到 GitHub 上最新的稳定版本。

macOS 自动更新要求发布包使用同一开发者证书签名；在仓库 Secrets 中配置 `CSC_LINK` 和 `CSC_KEY_PASSWORD` 后，工作流会自动签名。未配置证书时仍可构建安装包，但 macOS 可能拒绝自动安装更新。

## 解锁设置

在应用右上角的“系统设置”中可以：

- 设置或修改至少 6 位的软件解锁密码
- 决定是否在启动时锁定应用
- 在系统已配置相应能力时启用 Windows Hello / PIN 或 macOS Touch ID
- 随时点击“立即锁定”隐藏工作区并阻止主进程继续访问 Gist API

系统身份验证取消、失败或临时不可用时，锁屏会保留软件密码解锁入口。

## Token 说明

程序需要 GitHub Personal Access Token，建议至少包含：

- `gist`

使用规则：

- 首次打开时输入 Token
- 本地已保存 Token 时会从系统加密存储读取并自动连接
- 已连接后界面不会明文展示 Token
- 可通过“更换 Token”重新绑定账号

## 支持的操作

### Gist

- 加载全部 Gist
- 创建 Gist
- 删除 Gist
- 在网页中打开当前 Gist

### 文件

- 选择文件
- 查看文件内容
- 进入编辑模式并保存修改
- 重命名文件
- 删除文件
- 新增文件
- 打开 Raw 链接
- 复制 Raw 链接

## 桌面客户端说明

- 不依赖独立本地后端服务
- 不需要手动启动额外 API 进程
- 已隐藏 Electron 原生菜单栏
- 支持高分辨率图标
- 支持跟随系统浅色 / 深色主题

## 后续可扩展

- 自动更新
- Gist 搜索与筛选增强
- 多标签页编辑
- 更多文件语言支持
