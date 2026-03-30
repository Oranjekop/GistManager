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
- 支持 Windows 安装包打包

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
```

打包完成后，安装包输出到：

```text
release/GistManager-Setup-1.0.1.exe
```

## Token 说明

程序需要 GitHub Personal Access Token，建议至少包含：

- `gist`

使用规则：

- 首次打开时输入 Token
- 本地已保存 Token 时会自动连接
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

## 已生成文件

- `release/GistManager-Setup-1.0.1.exe`

## 后续可扩展

- Token 本地加密存储
- 自动更新
- Gist 搜索与筛选增强
- 多标签页编辑
- 更多文件语言支持
