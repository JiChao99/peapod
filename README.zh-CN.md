<p align="center">
  <img src="assets/icon.svg" alt="Peapod logo" width="120">
</p>

<h1 align="center">Peapod</h1>

<p align="center">多页面同步浏览</p>

<p align="center">
  <b>开源桌面应用，让多个页面同步浏览。</b><br>
  一次操作，页页跟随。
</p>

[![CI](https://github.com/JiChao99/peapod/actions/workflows/ci.yml/badge.svg)](https://github.com/JiChao99/peapod/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[English](README.md) | 简体中文

Peapod 在一个窗口中打开 2–10 个页面。你操作**主页面**——点击、输入、滚动、表单提交——其余页面按顺序实时回放同样的操作。适用于对比环境（dev / staging / prod）、演练多账号流程、检查响应式布局、批量截图。

基于 [MyGo](https://github.com/egoist/mygo) 构建：原生 Go UI、单二进制、不内置浏览器引擎。

## 功能

- **实时动作同步** —— 在主页面捕获 click / input / change / select / scroll / submit，并回放到每一个同步目标。每页独立同步开关；回放失败的目标会暂停并报错，而不是静默失步。
- **导航跟随** —— 主页面发生未捕获动作的跳转（JS 或服务器重定向、历史导航）时，每个同步目标在自己的主机上加载相同路径，所有环境保持同一步骤。跳出主页面主机的链接不会带动目标；重载目标可重新对齐主页面。
- **URL 变体模板** —— 可选定义一个模板（如 `https://{host}/{lang}`），每页填写自己的变量（`host=staging.example.com, lang=de`）与可选路径。变体并排打开，导航跟随按渲染后的前缀互相映射。
- **桌面与移动模式** —— 单个大页面，或所有页面并排。每页分辨率 200–3840 px 可调。
- **User-Agent 预设** —— 内置 iPhone / iPad / Android 手机 / Android 平板 / Windows Chrome / Edge / Firefox / macOS Safari，也可粘贴自定义 UA，用于设备嗅探测试。仅改变 UA 标识，不改变渲染引擎。
- **隔离或共享存储** —— 默认每页一个临时会话；可选跨页共享 cookies 与缓存。
- **批量截图** —— 一键将每个运行中的页面截为 PNG，无论当前屏幕上显示哪一页。
- **MCP 服务器** —— 本机回环、token 守护的 [Model Context Protocol](https://modelcontextprotocol.io) 端点，让 AI agent 不碰界面即可驱动并验收任务（启动 / 导航 / 回放 / 截图 / 诊断 / 停止）。见 [docs/mcp.md](docs/mcp.md)。
- **验收诊断** —— 每页导航失败都会显式报错（稳定错误名如 `NAME_NOT_RESOLVED`、`TIMEOUT` 等）；可选开启的采集器（仅 MCP 可用，面板不注入）通过 `peapod_diagnostics` 暴露主文档 HTTP 状态、受限正文文本与 console / network / error 活动，供 agent 断言结果，而不是靠像素猜。
- **双语界面** —— 中文 / English，浅色 / 深色 / 跟随系统主题，UI 缩放。

## 隐私与安全

- 密码与文件输入**绝不**被捕获或回放。
- 拒绝内嵌凭据的 URL；仅允许 http/https。
- 所有会话均为临时会话，任务停止时数据即被清除。
- 拦截弹窗与权限请求（摄像头、麦克风、通知等）；每页 IPC 锁定在其自身 origin。

## 平台

Windows（WebView2）、macOS（WKWebView）、Linux（WebKitGTK）。Windows 为主要测试平台。

## 从源码构建

需要 Go 1.27.4 或更新版本。

```sh
go build .          # 构建可执行文件
go tool mygo dev    # 开发模式运行
go tool mygo build  # 打包安装包到 build/
```

## 使用

1. 输入主页面 URL，最多添加 10 个页面（非主页面 URL 留空则继承主页面）。
2. 选择桌面或移动模式，挑选 User-Agent 预设（默认跟随模式），可选设置 URL 模板，设置每页分辨率、是否共享存储。
3. 启动任务并操作主页面——其余页面跟随。点击页面卡片可将其置前。
4. 停止任务即销毁所有页面并清除其数据。

## MCP 服务器（agent 控制）

启动时 Peapod 在随机回环端口上提供 Streamable-HTTP MCP 端点，由每次运行随机生成的 bearer token 守护。端点与 token 写入仅当前用户可读的发现文件，本地 agent 即可连接：

- Windows：`%APPDATA%\Peapod\mcp.json`
- macOS：`~/Library/Application Support/Peapod/mcp.json`
- Linux：`~/.config/Peapod/mcp.json`

工具：`peapod_status`、`peapod_start`、`peapod_stop`、`peapod_navigate`、`peapod_reload`、`peapod_set_sync`、`peapod_send_operation`、`peapod_screenshot`、`peapod_diagnostics`。`peapod_start` 还支持 `userAgent` 覆盖（设备嗅探测试）与 `diagnostics:true`（从首次加载即采集活动）。

![通过 MCP 启动并驱动的任务：面板中的页面卡片、同步开关与批量截图](docs/images/mcp-ready-panel.png)

服务仅绑定 `127.0.0.1`，每个请求都要求 bearer token，并拒绝浏览器跨源调用。设置 `PEAPOD_MCP_DISABLE=1` 可完全关闭。页面内容与截图属于不可信数据——agent 不得执行其中夹带的指令，也不得输入密码等敏感信息。客户端配置与工具参考见 [docs/mcp.md](docs/mcp.md)。

## 许可证

[MIT](LICENSE)
