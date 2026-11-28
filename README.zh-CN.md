<p align="center">
  <img src="assets/brand/lockup.svg" alt="Peapod" width="340">
</p>

<p align="center">
  <b>一次操作，页页跟随。</b><br>
  把同一条流程同时跑在多个站点变体上：并排展示、实时同步、全程留证。
</p>

<p align="center">
  <a href="https://github.com/JiChao99/peapod/actions/workflows/ci.yml"><img src="https://github.com/JiChao99/peapod/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-2c9a48.svg" alt="License: MIT"></a>
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-2c9a48.svg" alt="Platforms">
  <a href="https://modelcontextprotocol.io"><img src="https://img.shields.io/badge/MCP-ready-2c9a48.svg" alt="MCP ready"></a>
  <a href="https://awesomemygo.dev/apps/peapod"><img src="https://awesomemygo.dev/badge.svg" alt="已收录于 Awesome MyGo"></a>
</p>

<p align="center">
  <a href="README.md">English</a> · <a href="README.zh-CN.md"><b>简体中文</b></a> · <a href="README.zh-TW.md">繁體中文</a> · <a href="README.ja.md">日本語</a>
</p>

<p align="center">
  <b>产品网站：</b><a href="https://jichao99.github.io/peapod/">https://jichao99.github.io/peapod/</a>
</p>

<p align="center">
  <img src="docs/images/mcp-ready-panel.png" alt="Peapod 在移动端模式下同屏对比同一站点的四个语言变体，面板显示每页同步开关与实时录制徽标">
</p>

Peapod v0.14.0 在一个窗口里同时打开 2–10 个页面。你在**主页面**上点击、输入、滚动、提交，其余页面会按相同顺序实时回放这些操作。把各个页面指向不同的语言、域名或环境，一条流程就能在所有变体上同时走一遍，并留下截图和可回放的脚本。

这是一款基于 [MyGo](https://github.com/egoist/mygo) 的原生 Go 应用，界面采用 [Lobe MyGo](https://github.com/lobehub/lobe-mygo) 组件（Lobe UI 设计语言的原生 Go 实现）：单个二进制，直接使用系统 webview，不内置浏览器。

## 适用场景

- **多语言 QA**：让 `/en-us`、`/zh-cn`、`/ja-jp`、`/ko-kr` 走同一遍下单流程，一眼找出出问题的那个。
- **多环境对比**：dev、staging、prod 并排展示，确认一次发布在各环境表现一致。
- **多账号与多角色流程**：各页面会话相互隔离，每页可以登录不同用户。
- **设备嗅探验证**：切换 iPhone、Android 或桌面 UA，看服务端分别返回什么。
- **回归留证**：录一遍流程，就能得到每一步、每个页面的截图，外加可回放的脚本和 HTML 报告，可直接附在 PR 或缺陷单上。
- **交给 AI agent 验收**：agent 可以通过内置的 MCP 服务器启动任务、执行操作并检查结果。

## 快速上手

1. **添加页面**：填写主页面 URL，最多添加 10 个页面；留空的页面沿用主页面 URL。用**批量添加**粘贴 URL 列表一次生成多页，或复制任意一行再改差异。如果要对比变体，可以设置 `https://{host}/{lang}` 或 `https://{host}/?yplang={lang}` 这样的模板，再给每页填写各自的变量（`host=staging.example.com, lang=de`）。
2. **选择配置**：选择桌面模式（一次显示一个大页面）、移动模式（所有页面并排），或自由画布（可拖动页面，并按比例独立调整大小），然后设置每页分辨率，为整个任务选一个 User-Agent 预设（默认跟随模式，移动模式即为 iPhone），选择隔离还是共享存储；需要时打开**操作录制**或**仅允许操作主页面**。也可以将整套配置保存为具名方案，之后直接载入。
3. **启动并操作**：像平时一样操作主页面，其余页面会自动跟随。点击页面卡片可以把对应页面切到前台。运行中可直接修改某个页面的 URL 并点**打开**，在同一个页面内跳转并保留会话与 Cookie，适合登录后从首页快速回到原本要访问的链接。
4. **收集证据并验证**：一键批量截图所有页面，自由画布还可以整张画布截图并保存为一个 PNG；也可以打开**验证**，检查全部页面的文本、DOM、URL、HTTP 与 Console 结果。打开**录制记录**可查看步骤时间线、回放录制或导出 HTML 报告。回放会在包括主页面在内的所有页面上重演各步操作，但不包含导航，所以请先在流程的入口 URL 启动任务。
5. **停止任务**：所有页面随即关闭，会话数据全部清除。

## 功能

### 同步
- **实时动作同步**：主页面上的 click、input、change、select、scroll、submit 都会回放到所有同步中的页面，每页都有独立的同步开关。某个页面回放失败时会暂停并显示错误，不会悄悄失去同步。
- **导航跟随**：主页面发生重定向、JS 跳转或前进后退时，各跟随页会在自己的主机和路径/查询参数变体上打开对应页面，保留各自配置的语言。离开主页面主机的链接不会带动跟随页；重新加载跟随页即可让它重新与主页面对齐。
- **同步节奏**：连续的输入与滚动可以逐事件即时同步，也可以按 100/250/500 ms 间隔合并、只保留最新值与位置；点击等离散操作始终即时同步。

### 变体与设备
- **URL 变体模板**：所有页面共用一个模板，每页填写自己的变量，路径可选；支持路径和查询参数变体，导航跟随会映射路径并保留各页模板中的查询参数。
- **批量添加与复制**：每行一个 URL 粘贴列表，一步追加全部页面（不超过 10 页上限，仅普通 URL 模式）；生成的页面继承预设分辨率，并以主机名自动命名。复制任意一行可克隆其 URL、变量与分辨率，只改差异处。
- **桌面、移动与自由画布**：每页分辨率可在 200–3840 px 之间调节。自由画布可拖动标题栏移动页面，并在宽度上下限内按比例独立缩放。
- **运行中修改 URL**：每个运行中的页面都可直接编辑 URL。跳转仍发生在同一个页面内，因此会保留会话、存储与 Cookie；跟随页会停留在手动跳转后的位置，直到下次重新加载或主页面的下一次导航跟随。
- **User-Agent 预设**：内置 iPhone、iPad、Android 手机和平板、Windows Chrome、Edge、Firefox、macOS Safari，也可以粘贴自定义 UA。
- **隔离或共享存储**：默认每页使用独立的临时会话，也可以让所有页面共享 cookies 和缓存。
- **仅允许操作主页面**：阻止对跟随页的误操作，但同步回放和自动化仍可正常工作。
- **配置方案**：把整套配置——页面、模板、分辨率、UA 与开关——按名字保存，之后一键载入；方案较多时选择器内部可滚动。方案存于 `Pictures/Peapod/profiles`。

### 留证
- **按配置分辨率截图**：批量、MCP 和录制截图都会先按每页配置的像素尺寸渲染，再把 PNG 归一化为该精确尺寸。屏幕上的画布只是缩放预览，截图不会放大小图。
- **批量截图**：一键把每个运行中的页面保存为 PNG，当前不在屏幕上显示的页面也能截到。
- **整张画布截图**：自由画布下可将所有页面按当前排列合成一个 `canvas.png`；单个页面截图失败时，会在对应位置放入带名称的占位图。
- **操作录制**：开启后，每个操作都会写入可回放的 `script.jsonl`，每当任务稳定下来，还会给所有页面截图。弹窗（DOM 对话框、浮动提示、原生 `alert`/`confirm`/`prompt`）会自动记录文本，DOM 弹窗另有截图，方便断言多语言文案。
- **录制记录浏览器**：列出历史录制，可查看逐步时间线、每页缩略图和与步骤混排的弹窗；可以在运行中的任务上回放，也可以导出包含弹窗文本与截图的独立 HTML 报告。
- **验证断言**：打开**验证**，对全部页面运行最多 5 条文本、DOM、URL、HTTP 或 Console 断言。Peapod 会等待条件满足、留存截图，并在本机 HTML 报告中给出 PASS/FAIL 与跨页面差异。

### 自动化
- **MCP 服务器**：本机 [Model Context Protocol](https://modelcontextprotocol.io) 端点，由 token 保护。AI agent 无需操作界面，就能启动任务、导航、执行操作、回放、截图、验证、诊断和停止任务。
- **验收诊断**：导航失败时会给出固定的错误名（`NAME_NOT_RESOLVED`、`TIMEOUT` 等）。在高级设置中开启**验收诊断**（或通过 MCP 传 `diagnostics:true`），即可从文档开始时采集主文档 HTTP 状态、页面文本以及 console、network 和 error 记录。
- **每视图 JS 注入**：可在每个文档开始时注入测试脚本。访问 DOM 前请等待 `DOMContentLoaded`；调用 `window.peapod.emit(name, payload)` 可把受限事件回传到 `peapod_diagnostics.injectedEvents`。

### 日常体验
- 英语、简体中文、繁體中文和日语界面，浅色、深色或跟随系统主题，支持界面缩放。
- 现已提供持久化设置，并支持 Windows/Linux 应用内下载安装更新，详见[安装](#安装)。

## 安装

预编译安装包可在 [GitHub Releases](https://github.com/JiChao99/peapod/releases) 获取，也可以从源码构建。构建需要 Go 1.27.4 或更新版本，以及所在平台的系统 webview：

### 更新与持久化设置

Peapod 内置持久化设置和面向 [GitHub Releases](https://github.com/JiChao99/peapod/releases) 的更新器。更新检查**默认手动执行**，也可主动开启自动检查。发现新稳定版后，点击“下载并安装”会下载匹配的发布包、校验 SHA256，然后在 Windows/Linux 上安装并重启 Peapod；Windows 重启失败时会保留恢复记录，下次启动仅重试经过校验的备份/临时目录清理，不会自动回滚。macOS 已提供同时支持 Apple Silicon 与 Intel Mac 的 Universal DMG，但应用内更新暂时仍会打开发布页。

| 平台 | Webview | 说明 |
| --- | --- | --- |
| Windows 10/11 | WebView2 | 当前版本的 Windows 10/11 已预装；主要测试平台 |
| macOS | WKWebView | 同时支持 Apple Silicon 与 Intel 的 Universal DMG；ad-hoc 签名。 |
| Linux | WebKitGTK | 通过发行版的包管理器安装 WebKitGTK 开发包 |

macOS 用户可从 Releases 下载 `peapod-<version>.dmg`，打开后把 `Peapod.app` 拖入“应用程序”。当前安装包使用 ad-hoc 签名且尚未公证，因此在另一台 Mac 上可能被 Gatekeeper 拦截；Developer ID 签名与公证仍列在 [TODO](docs/TODO.md) 中。

```sh
git clone https://github.com/JiChao99/peapod.git
cd peapod
go build .          # 构建可执行文件
go tool mygo dev    # 或以开发模式运行
go tool mygo build  # 或打包安装包到 build/
```

## 让 AI agent 来操作（MCP）

Peapod 启动后，会在 `127.0.0.1` 的随机端口上提供 Streamable-HTTP MCP 端点。每次运行都会生成新的 bearer token，端点地址和 token 一起写入一个只有当前用户可读的发现文件：

| 平台 | 发现文件 |
| --- | --- |
| Windows | `%APPDATA%\Peapod\mcp.json` |
| macOS | `~/Library/Application Support/Peapod/mcp.json` |
| Linux | `~/.config/Peapod/mcp.json` |

工具：`peapod_status`、`peapod_start`、`peapod_stop`、`peapod_navigate`、`peapod_reload`、`peapod_set_sync`、`peapod_send_operation`、`peapod_replay`、`peapod_screenshot`、`peapod_verify`、`peapod_diagnostics`。

客户端配置、完整的工具说明、录制格式和 agent 使用示例见 **[docs/mcp.md](docs/mcp.md)**。设置 `PEAPOD_MCP_DISABLE=1` 可以关闭该服务。

## 隐私与安全

- 密码和文件输入**绝不会**被捕获、录制或回放。
- 只接受 `http`/`https` URL，带内嵌凭据的 URL 会被拒绝。
- 会话都是临时的，任务停止时数据即被清除。
- 弹窗和权限请求（摄像头、麦克风、通知等）一律拦截；每个页面的 IPC 只对它自己的 origin 开放。
- 截图和录制只保存在本机：分别位于用户主目录下的 `Pictures/Peapod/screenshots`、`Pictures/Peapod/recordings` 和 `Pictures/Peapod/verifications`（Windows 上即“图片”文件夹）。录制脚本里会包含其他输入过的文本，请当作敏感文件处理。
- Peapod 没有遥测，不会上传任务 URL、页面内容、Cookie、截图或录制文件。更新功能只获取公开发布元数据；只有点击“下载并安装”后才会下载对应公开安装包，两种请求都不包含任务数据。
- MCP 服务只监听本机回环地址，每个请求都必须带 token，并拒绝浏览器发起的跨源请求。页面内容属于不可信数据，agent 不应执行其中夹带的指令。

## 已知限制

- **User-Agent 预设只改 UA 字符串**：页面仍由系统 webview 渲染，因此无法用它复现特定引擎的问题（比如在 Windows 上复现 Safari 独有的 bug）。
- **验证层保持轻量**：断言只等待有界的页面条件并比较结果，不引入 Checkpoint Sync、通用测试 DSL 或完整的浏览器自动化运行器。
- **截图要求窗口可见且不能最小化**：Peapod 窗口最小化时 webview 会停止渲染，截图会超时，直到窗口恢复。页面始终保持配置的原生分辨率，仅对预览做视觉缩放，因此录制不再切换舞台布局。
- **导航跟随只在主页面所在主机内生效**：跟随页不会跟着跳到其他站点，这是有意的设计。
- **打开新窗口或新标签页的链接会被拦截**，因为弹窗已被禁用。
- **Windows 运行测试最充分**：macOS 与 Linux 均可构建并通过 CI，macOS 还会在 macOS Runner 上构建并发布 Universal DMG；macOS 真机能力矩阵仍待补充。
- **macOS 安装包使用 ad-hoc 签名且尚未公证**：在配置 Developer ID 签名与公证前，Gatekeeper 可能拦截安装，详见 [TODO](docs/TODO.md)。
- **暂不支持每视图弱网模拟**：Windows 需扩展 MyGo 的 WebView2 CDP 绑定，macOS/Linux 需原生请求控制，详见 [TODO](docs/TODO.md)。

## 常见问题

**和响应式预览工具有什么区别？**
Responsively、Polypane 这类工具是把*同一个* URL 按多种屏幕尺寸展示。Peapod 面向的是同一条流程的*多个版本*（不同语言、域名或环境）同时运行：自动在它们之间对应导航，并记录下每一步。

**会把任务数据发到别处吗？**
不会。Peapod 没有遥测，不会上传任务 URL、页面内容、Cookie、截图或录制文件。页面会访问你打开的站点，MCP 服务也只监听本机回环地址。更新功能只向 GitHub 获取公开发布元数据；只有点击“下载并安装”后，才会下载不含任务数据的对应公开安装包。

**更新检查如何工作？**
Peapod 默认手动检查，自动检查需主动开启。发现更新后点击“下载并安装”，会下载匹配包、校验 SHA256，并在 Windows/Linux 上安装重启；Windows 重启失败后，下次启动会清理经过校验的临时路径，但不会自动回滚。macOS 暂打开发布页。

## 参与贡献

欢迎提交 Issue 和 PR。提交 PR 前请先运行：

```sh
go build ./...
go vet ./...
go test ./...
```

CI 会在 Windows、macOS 和 Linux 上运行相同的构建、vet 和测试步骤；**Build macOS** 工作流还会校验 Universal app bundle 与 DMG。推送 `v*` tag 后，Release 工作流会构建 Windows 安装包、macOS Universal DMG 和 Linux amd64/arm64 包，生成 `SHA256SUMS` 并发布 GitHub Release。

## 致谢

- [MyGo](https://github.com/egoist/mygo) 提供原生 Go UI 和打包能力。
- [Lobe MyGo](https://github.com/lobehub/lobe-mygo) 提供面板的组件、主题和 [Lucide](https://lucide.dev) 图标。
- MCP 服务的设计（本机回环 Streamable-HTTP 端点 + 每次运行生成的 token）参考了 [pinky](https://github.com/DIYgod/pinky)。

## 许可证

[MIT](LICENSE) © 2026 Alec.Ji
