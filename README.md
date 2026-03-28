# Gist管理器

一个基于 `Vue 3 + Node.js + Electron` 的 GitHub Gist 桌面管理工具，支持通过 GitHub Token 管理当前账号下的全部 Gist。

## 功能特性

- 自动读取并管理当前账号下全部 Gist
- 首次启动输入 Token，后续可自动连接
- 支持随时更换 Token
- 浏览 Gist 列表、文件树和文件详情
- 创建 Gist、添加文件、重命名文件、删除文件、删除 Gist
- 使用 Monaco 编辑器进行代码编辑
- 支持复制 Raw 链接、打开 Raw 链接、在网页中打开 Gist
- 支持 Windows 桌面打包，生成 `exe` 安装包

## 技术栈

- `Vue 3`
- `Vite`
- `Node.js`
- `Express`
- `Electron`
- `Monaco Editor`

## 项目结构

```text
.
├─ client/          前端界面
├─ server/          本地 API 服务
├─ electron/        Electron 主进程与预加载脚本
├─ build/           应用图标等打包资源
├─ release/         打包输出目录
└─ README.md
```

## 本地开发

### 安装依赖

```bash
npm install
npm install --prefix client
npm install --prefix server
```

### 启动桌面版

```bash
npm run dev:desktop
```

启动后会自动：

- 启动 Vite 前端开发服务
- 启动本地 Node.js 后端
- 打开 Electron 桌面客户端

## 打包

### 生成 Windows 安装包

```bash
npm run dist:win
```

打包完成后，安装包会输出到：

```text
release/GistManager-Setup-1.0.0.exe
```

## Token 说明

程序需要使用 GitHub Personal Access Token。

建议至少开启：

- `gist`

程序特性：

- 首次打开时需要输入 Token
- 本地已有 Token 时会自动连接
- 已连接后界面不会明文显示 Token
- 可通过“更换 Token”重新绑定账号

## 支持的操作

### Gist 级别

- 加载全部 Gist
- 创建 Gist
- 删除 Gist
- 在网页中打开当前 Gist

### 文件级别

- 选择文件
- 编辑文件内容
- 保存文件
- 重命名文件
- 删除文件
- 新增文件
- 打开 Raw 链接
- 复制 Raw 链接

## 接口说明

本地 Node 服务提供以下接口：

- `GET /api/session`：获取当前 Token 对应的 GitHub 用户信息
- `GET /api/gists`：获取当前账号全部 Gist
- `POST /api/gists`：创建新的 Gist
- `GET /api/gists/:gistId`：获取指定 Gist 详情
- `PATCH /api/gists/:gistId`：更新 Gist 内容
- `DELETE /api/gists/:gistId`：删除指定 Gist

## 图标与界面

- 已统一使用应用图标资源
- 支持高分辨率显示
- 桌面窗口已隐藏 Electron 原生菜单栏
- 界面支持跟随系统浅色 / 深色主题切换

## 已生成文件

当前项目中已生成 Windows 安装包：

- `release/GistManager-Setup-1.0.0.exe`

## 后续可扩展

- Token 本地加密存储
- 自动更新
- Gist 搜索与筛选增强
- 多标签页编辑
- 更多文件语言支持
