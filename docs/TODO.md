# TODO

- [ ] **每视图网络模拟**：MyGo v0.10.0 没有公开的逐 WebView 弱网 API。Windows 可在 MyGo 内部通过 WebView2 CDP `Network.emulateNetworkConditions` 扩展离线、延迟和上下行限速；macOS/Linux 还需要各自的原生代理或请求拦截。当前版本不提供 JS fetch/XHR 假限速，避免漏掉导航、子资源、Worker 和 WebSocket。
- [ ] **Windows Authenticode 签名**：本机和 GitHub 仓库均没有可用证书。`.github/workflows/release.yml` 已预留 `WINDOWS_CERTIFICATE_BASE64` 与 `WINDOWS_CERTIFICATE_PASSWORD` 签名入口；配置有效证书后自动签名，当前 Release 仍为未签名包。
- [ ] **更新包签名验证**：当前直接更新通过 HTTPS、允许域名和同源 `SHA256SUMS` 校验完整性，但发布账号被控时攻击者可同时替换安装包与清单。需要为清单增加公钥签名，并在 Windows 安装前验证 Authenticode；在签名密钥和证书配置完成前，不能宣称更新链已验证发布者身份。
- [ ] **macOS 应用内更新**：Universal DMG 已纳入 Release，但应用内更新仍打开发布页；待 Developer ID 签名、公证与真机更新验证完成后再实现直接安装。
- [ ] **Checkpoint Sync**：当前验证层只等待有界的页面条件，不让所有业务状态同步后再继续。待真实复杂流程证明 Action Sync 的局限后，再考虑 URL/DOM/业务标记检查点，避免提前重构同步引擎。
- [ ] **MCP 导航策略**：当前接受 HTTP/HTTPS URL，包括本机与内网地址，这对本地 QA 有必要；后续可为 Agent 导航增加可配置的私网/特殊地址警告或允许列表，不默认阻断现有工作流。
- [ ] **macOS Developer ID 签名与公证**：当前 Universal DMG 使用 MyGo 默认 ad-hoc 签名，未配置 Apple Developer 证书或公证凭据；在其他 Mac 上可能被 Gatekeeper 拦截。正式分发前需配置 `MACOS_CERTIFICATE`、`MACOS_CERTIFICATE_PASSWORD`、Apple ID/Team ID/App Password 与 MyGo 公证参数。
- [ ] **macOS 真机能力矩阵**：CI 已覆盖 macOS build/vet/test、Universal app bundle 与 DMG 校验；仍需在 Apple Silicon 与 Intel Mac 上验证截图、诊断、导航、录制、MCP 和更新行为。
