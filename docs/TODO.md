# TODO

- [ ] **每视图网络模拟**：MyGo v0.10.0 没有公开的逐 WebView 弱网 API。Windows 可在 MyGo 内部通过 WebView2 CDP `Network.emulateNetworkConditions` 扩展离线、延迟和上下行限速；macOS/Linux 还需要各自的原生代理或请求拦截。当前版本不提供 JS fetch/XHR 假限速，避免漏掉导航、子资源、Worker 和 WebSocket。
- [ ] **Windows Authenticode 签名**：本机和 GitHub 仓库均没有可用证书。`.github/workflows/release.yml` 已预留 `WINDOWS_CERTIFICATE_BASE64` 与 `WINDOWS_CERTIFICATE_PASSWORD` 签名入口；配置有效证书后自动签名，当前 Release 仍为未签名包。
- [ ] **更新包签名验证**：当前直接更新通过 HTTPS、允许域名和同源 `SHA256SUMS` 校验完整性，但发布账号被控时攻击者可同时替换安装包与清单。需要为清单增加公钥签名，并在 Windows 安装前验证 Authenticode；在签名密钥和证书配置完成前，不能宣称更新链已验证发布者身份。
- [ ] **Windows 更新异常恢复**：helper 启动后的失败会保留安装备份，避免误删旧版本；但安装成功后的重启失败目前只写临时错误，尚未持久化恢复状态，运行中的 helper/旧 exe 也可能阻止临时目录清理。
- [ ] **macOS 应用内更新**：当前 Release 没有 macOS 安装包，也没有 macOS CI。直接更新在 Windows/Linux 可用；macOS 暂时回退到发布页。
