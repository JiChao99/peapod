const translations = {
  en: {
    skip: 'Skip to main content',
    'nav.label': 'Main navigation',
    'nav.product': 'Product',
    'nav.cases': 'Use cases',
    'nav.how': 'How it works',
    'nav.mobile': 'Mobile navigation',
    'brand.home': 'Peapod home',
    'language.label': 'Language',
    'menu.open': 'Open menu',
    'menu.close': 'Close menu',
    'page.title': 'Peapod — Repeat less. Know more.',
    'meta.description': 'Repeat less. Know more. Run one flow across languages, environments, and devices — with evidence at every step.',
    'hero.badge': 'v0.13.0 · Open-source desktop app',
    'hero.title1': 'Repeat less. ',
    'hero.title2': 'Know more.',
    'hero.description': 'Run one flow across languages, environments, and devices — side by side, in sync, with evidence at every step.',
    'hero.github': 'Get Peapod',
    'hero.demo': 'See how it works',
    'hero.preview': 'Peapod synchronized pages preview',
    'hero.live': 'Live sync',
    'hero.primary': 'Primary',
    'hero.add': 'Add to bag',
    'hero.you': 'You',
    'hero.synced': 'Synced',
    'hero.zhName': 'Chinese',
    'hero.jaName': 'Japanese',
    'hero.follow1': 'Same action.<br>Synced.',
    'hero.follow2': 'One action.<br>Every page follows.',
    'hero.recording': 'Recording actions & screenshots',
    'hero.pages': 'pages ready',
    'hero.noteTitle': 'No repeated work',
    'hero.noteBody': 'Click, type, scroll, submit',
    'hero.local': 'Task data stays local',
    'hero.noTelemetry': 'No telemetry · No cloud sync',
    'stats.pages': 'pages, one workspace',
    'stats.local': 'local task data',
    'stats.telemetry': 'telemetry',
    'stats.mcp': 'built in for AI agents',
    'product.kicker': 'More than a preview. A complete flow.',
    'product.title': 'One flow. Every version.<br>Evidence at every step.',
    'product.description': 'Responsive previewers only change viewport sizes. Peapod runs real user journeys across languages, domains, and environments — then keeps the evidence.',
    'product.preview': 'Real product preview',
    'product.running': 'Task running',
    'product.imageAlt': 'Peapod runs four locale variants side by side with live sync and recording',
    'product.locale': '4 languages, one pass',
    'product.record': 'Actions & screenshots, captured',
    'features.syncTitle': 'Live sync, with a primary-only lock',
    'features.syncBody': 'Click, type, scroll, and submit on the primary page. Followers replay each action in order. Lock follower input to prevent accidents without pausing sync or automation.',
    'features.click': 'Click',
    'features.input': 'Input',
    'features.submit': 'Submit',
    'features.allSynced': 'All synced',
    'features.runtimeTitle': 'Change URLs, keep your session',
    'features.runtimeBody': 'Edit any running page and reopen it in place without losing storage or cookies. Primary navigation still maps to each follower’s own host, path, and query variants.',
    'features.canvasTitle': 'Free canvas, proportional views',
    'features.canvasBody': 'Drag pages into place and resize each view proportionally. Capture the whole arranged canvas as one PNG, with a labeled placeholder if a page fails.',
    'features.captureTitle': 'Screenshots at configured resolution',
    'features.captureBody': 'Batch capture renders every running page at its configured pixel size, even when it is off screen. The scaled preview never limits the output.',
    'features.profilesTitle': 'Reusable setup profiles',
    'features.profilesBody': 'Save pages, URL templates, resolutions, User-Agent, and toggles under one name. Reload the whole setup in one click.',
    'features.profileName': 'Checkout QA',
    'features.evidenceTitle': 'Record, replay, and keep popup evidence',
    'features.evidenceBody': 'Browse a step timeline, replay sessions, run text/DOM/URL/HTTP/console checks, and export PASS/FAIL reports with per-page screenshots and cross-page differences.',
    'features.mcpTitle': 'Let an AI agent take the wheel',
    'features.mcpBody': 'A token-protected local MCP server lets agents start, navigate, operate, replay, screenshot, verify, diagnose, and stop tasks without using the UI.',
    'features.injectTitle': 'Inject test scripts per view',
    'features.injectBody': 'Run JavaScript at document start on each page, manipulate its DOM after DOMContentLoaded, and send bounded test events back to the host for diagnostics.',
    'features.privacyTitle': 'Local-first. Private by default.',
    'features.privacyBody': 'Passwords are never recorded, and sessions are wiped when a task stops. Task files stay on your machine. Optional GitHub release-metadata checks are separate and include no task data.',
    'features.loopback': 'Loopback access only',
    'cases.kicker': 'Built for real work',
    'cases.title': 'Turn repeated checks<br>into one parallel pass.',
    'cases.description': 'From the final check before release to regression testing between iterations, keep every version moving together.',
    'cases.localeTitle': 'Multi-locale QA',
    'cases.localeBody': 'Run one checkout flow across English, Chinese, Japanese, and Korean. Find the one locale that breaks before your users do.',
    'cases.zh': 'Chinese',
    'cases.ja': 'Japanese',
    'cases.ko': 'Korean',
    'cases.passed': 'All flows passed',
    'cases.envTitle': 'Environment comparison',
    'cases.envBody': 'Put dev, staging, and prod side by side and confirm a release behaves the same everywhere.',
    'cases.rolesTitle': 'Multi-account & roles',
    'cases.rolesBody': 'Use isolated sessions to sign in as admin, editor, and guest, then verify the experience for every permission level.',
    'cases.isolated': 'Isolated sessions',
    'cases.isolatedBody': 'Login states stay independent',
    'how.kicker': 'Simple enough to start now',
    'how.title': 'Add pages.<br>Start operating.<br>Keep the evidence.',
    'how.description': 'No automation script required. Use the primary page as you normally would. Peapod keeps every other page in step.',
    'how.docs': 'Read the full quick start',
    'how.step1Title': 'Add your pages',
    'how.step1Body': 'Paste a URL list or use a variable template. Configure up to 10 language, environment, or device pages at once.',
    'how.step2Title': 'Choose and save a setup',
    'how.step2Body': 'Pick a layout, resolutions, User-Agent, session isolation, primary-only input, and recording. Save it all as a reusable profile.',
    'how.step3Title': 'Operate as usual',
    'how.step3Body': 'Complete the flow on the primary page. Followers stay in sync. Edit any running URL in place without losing its session or cookies.',
    'how.step4Title': 'Capture, replay, export',
    'how.step4Body': 'Take configured-resolution screenshots, review recordings, run verification checks, then export PASS/FAIL and difference reports.',
    'install.kicker': 'v0.13.0 · Install or build from source',
    'install.title': 'One download.<br>A better QA loop.',
    'install.description': 'Prebuilt Windows installer, macOS Universal DMG and Linux packages are available on GitHub Releases, or build from source with no extra browser to install. Source builds need Go 1.27.4+ and your system WebView.',
    'install.releases': 'Download from GitHub Releases',
    'install.webview': 'System WebView',
    'install.binary': 'Single binary',
    'install.copy': 'Copy',
    'install.copyAria': 'Copy build commands',
    'install.terminal': 'Terminal',
    'install.copied': 'Copied',
    'install.ready': 'Build complete. Launch Peapod.',
    'release.kicker': 'New in v0.13.0',
    'release.title': 'Universal macOS.<br>Built in CI.',
    'release.macosTitle': 'One DMG for Apple Silicon & Intel',
    'release.macosBody': 'A macOS runner builds Peapod.app, checks both architectures and creates the Universal disk image.',
    'release.ciTitle': 'Verified bundle, not just a cross-compile',
    'release.ciBody': 'CI checks the bundle ID, version, lipo architectures, ad-hoc signature and DMG integrity.',
    'release.signTitle': 'Signing stays honest',
    'release.signBody': 'Developer ID signing and notarization remain TODO; Gatekeeper may block the current ad-hoc build.',
    'faq.kicker': 'FAQ',
    'faq.title': 'Good to know.',
    'faq.description': 'How Peapod works — and where its boundaries are.',
    'faq.q1': 'How is this different from a responsive previewer?',
    'faq.a1': 'Responsive previewers usually show one URL at several viewport sizes. Peapod runs several versions of a complete flow, with mapped navigation, reusable setups, and evidence for every step.',
    'faq.q2': 'Does Peapod send my task data anywhere?',
    'faq.a2': 'Peapod has no telemetry. Task URLs, page content, cookies, screenshots, and recordings are not uploaded. Updates fetch public metadata and, only after Download and install, the matching public package without task data.',
    'faq.q3': 'How do update checks work?',
    'faq.a3': 'Checks are manual by default. On Windows/Linux, Download and install fetches the matching package, verifies SHA256, installs, and restarts Peapod. macOS opens the release page, where the Universal DMG is available.',
    'faq.q4': 'Can it reproduce Safari-specific bugs?',
    'faq.a4': 'User-Agent presets only change the UA string. Pages still render in your platform WebView, so engine-specific Safari bugs cannot be reproduced this way.',
    'faq.q5': 'Which platforms are supported?',
    'faq.a5': 'Windows, macOS, and Linux via WebView2, WKWebView, and WebKitGTK respectively. All three build and pass CI; macOS publishes a Universal DMG, while Windows remains the most thoroughly runtime-tested platform.',
    'faq.q6': 'Can I simulate a slow network?',
    'faq.a6': 'Not yet. Per-view offline, latency, and throughput control requires extending the WebView backend; see the project TODO.',
    'cta.kicker': 'Open source · Local-first · Built for focus',
    'cta.title': 'Repeat less. Know more.',
    'cta.description': 'Let Peapod watch every other page while you focus on the one that matters.',
    'cta.button': 'Get Peapod',
    'cta.docs': 'Quick start',
    'footer.tagline': 'Repeat less. Know more.',
    'footer.docs': 'Docs',
    'footer.contributors': 'Peapod contributors',
    'footer.back': 'Back to top'
  },
  zh: {
    skip: '跳到主要内容',
    'nav.label': '主导航',
    'nav.product': '产品',
    'nav.cases': '使用场景',
    'nav.how': '工作原理',
    'nav.mobile': '移动端导航',
    'brand.home': 'Peapod 首页',
    'language.label': '语言',
    'menu.open': '打开菜单',
    'menu.close': '关闭菜单',
    'page.title': 'Peapod — 一次操作，页页跟随。',
    'meta.description': '一次操作，页页跟随。跨语言、跨环境、跨设备执行同一条流程，并留下每一步的证据。',
    'hero.badge': 'v0.13.0 · 开源桌面应用',
    'hero.title1': '一次操作，',
    'hero.title2': '页页跟随。',
    'hero.description': '在同一个窗口里，让多个语言、环境或设备版本一起完成同一条流程——并排展示、实时同步，每一步都有证据。',
    'hero.github': '获取 Peapod',
    'hero.demo': '看看它如何工作',
    'hero.preview': 'Peapod 多页面同步预览',
    'hero.live': '实时同步',
    'hero.primary': '主页面',
    'hero.add': '加入购物袋',
    'hero.you': '你',
    'hero.synced': '已同步',
    'hero.zhName': '中文',
    'hero.jaName': '日语',
    'hero.follow1': '相同操作。<br>同步完成。',
    'hero.follow2': '一次操作。<br>页页跟随。',
    'hero.recording': '正在记录操作与截图',
    'hero.pages': '页面就绪',
    'hero.noteTitle': '无需重复操作',
    'hero.noteBody': '点击、输入、滚动、提交',
    'hero.local': '任务数据留在本地',
    'hero.noTelemetry': '无遥测 · 无云端同步',
    'stats.pages': '个页面，一个工作区',
    'stats.local': '任务数据存于本地',
    'stats.telemetry': '遥测',
    'stats.mcp': '内置 AI Agent 接口',
    'product.kicker': '不只是预览，而是完整流程',
    'product.title': '一个流程，所有版本。<br>每一步，都有证据。',
    'product.description': '响应式预览器只改变视口尺寸。Peapod 让不同语言、域名和环境真正走完同一个用户旅程，并保留完整证据。',
    'product.preview': '真实产品预览',
    'product.running': '任务运行中',
    'product.imageAlt': 'Peapod 并排运行四个语言版本，并显示实时同步与录制状态',
    'product.locale': '4 种语言，一次验证',
    'product.record': '操作与截图自动留存',
    'features.syncTitle': '实时同步，锁定主页面操作',
    'features.syncBody': '在主页面点击、输入、滚动或提交，跟随页按顺序实时重放。开启“仅允许操作主页面”可防止误操作，同步回放与自动化仍照常运行。',
    'features.click': '点击',
    'features.input': '输入',
    'features.submit': '提交',
    'features.allSynced': '全部同步',
    'features.runtimeTitle': '修改 URL，会话不中断',
    'features.runtimeBody': '运行中可直接编辑任意页面的 URL，在原页面内重新打开，存储与 Cookie 都不会丢失。主页面导航仍会映射到各跟随页自己的域名、路径和查询参数。',
    'features.canvasTitle': '自由画布，视图等比例缩放',
    'features.canvasBody': '拖动页面自由排列，每个视图均可等比例缩放。整张画布可导出为一张 PNG；单个页面失败时会保留带名称的占位图。',
    'features.captureTitle': '按配置分辨率截图',
    'features.captureBody': '批量截图会按每页配置的像素尺寸渲染所有运行中的页面，即使它不在屏幕上。缩放预览不会限制最终输出。',
    'features.profilesTitle': '一键复用设置方案',
    'features.profilesBody': '把页面、URL 模板、分辨率、User-Agent 和开关保存为具名方案，下次一键载入整套配置。',
    'features.profileName': '结账流程 QA',
    'features.evidenceTitle': '录制、回放，保留弹窗证据',
    'features.evidenceBody': '在时间线中回顾每一步、回放会话、运行文本/DOM/URL/HTTP/Console 断言，并导出带逐页截图和跨页面差异的 PASS/FAIL 报告。',
    'features.mcpTitle': '让 AI Agent 来驱动',
    'features.mcpBody': '内置 token 保护的本地 MCP 服务，让 Agent 无需操作界面，即可启动、导航、操作、回放、截图、验证、诊断和停止任务。',
    'features.injectTitle': '每个视图独立注入测试脚本',
    'features.injectBody': '在每页文档开始时执行 JavaScript，在 DOMContentLoaded 后操作 DOM，并把受限的测试事件回传主机用于诊断。',
    'features.privacyTitle': '本地优先，隐私默认',
    'features.privacyBody': '密码永不录制，任务停止后会话随即清除，任务文件只保存在你的电脑上。可选的 GitHub 发布元数据检查独立运行，不包含任何任务数据。',
    'features.loopback': '仅本机回环访问',
    'cases.kicker': '为真实工作而设计',
    'cases.title': '把重复验证，<br>变成一次并行检查。',
    'cases.description': '从上线前的最后一遍检查，到持续迭代中的回归验证，让每个版本保持同步。',
    'cases.localeTitle': '多语言 QA',
    'cases.localeBody': '用同一套结账流程检查英语、中文、日语和韩语，在用户发现之前定位只在某个语言出现的问题。',
    'cases.zh': '中文',
    'cases.ja': '日语',
    'cases.ko': '韩语',
    'cases.passed': '流程全部通过',
    'cases.envTitle': '多环境对比',
    'cases.envBody': '把 dev、staging 和 prod 并排放在一起，确认一次发布在每个环境中的行为一致。',
    'cases.rolesTitle': '多账号与多角色',
    'cases.rolesBody': '用独立会话分别登录管理员、编辑和访客，验证不同权限下的体验。',
    'cases.isolated': '会话彼此隔离',
    'cases.isolatedBody': '登录状态互不影响',
    'how.kicker': '简单到可以马上开始',
    'how.title': '添加页面。<br>开始操作。<br>留下证据。',
    'how.description': '不需要编写自动化脚本。像平时一样使用主页面，Peapod 负责让其他页面跟上。',
    'how.docs': '阅读完整快速开始',
    'how.step1Title': '添加你的页面',
    'how.step1Body': '粘贴 URL 列表或使用变量模板，一次配置最多 10 个语言、环境或设备页面。',
    'how.step2Title': '选择并保存设置',
    'how.step2Body': '设置布局、分辨率、User-Agent、会话隔离、仅主页面操作和录制，再保存为可复用的方案。',
    'how.step3Title': '像平时一样操作',
    'how.step3Body': '在主页面完成流程，跟随页实时同步。运行中可直接修改 URL，页面会话与 Cookie 都不会丢失。',
    'how.step4Title': '截图、回放、导出',
    'how.step4Body': '按配置分辨率截图、查看录制、运行验证断言，再导出 PASS/FAIL 与差异报告。',
    'install.kicker': 'v0.13.0 · 安装或从源码构建',
    'install.title': '一次下载，<br>进入更高效的 QA。',
    'install.description': '可在 GitHub Releases 获取 Windows 安装包、macOS Universal DMG 和 Linux 包，也可从源码构建，无需安装额外浏览器。源码构建需要 Go 1.27.4 或更新版本，以及系统 WebView。',
    'install.releases': '从 GitHub Releases 下载',
    'install.webview': '系统 WebView',
    'install.binary': '单二进制应用',
    'install.copy': '复制',
    'install.copyAria': '复制构建命令',
    'install.terminal': '终端',
    'install.copied': '已复制',
    'install.ready': '构建完成，启动 Peapod',
    'release.kicker': 'v0.13.0 新功能',
    'release.title': 'Universal macOS。<br>由 CI 构建。',
    'release.macosTitle': '一个 DMG，支持 Apple Silicon 与 Intel',
    'release.macosBody': 'macOS Runner 构建 Peapod.app、校验双架构并生成 Universal 磁盘映像。',
    'release.ciTitle': '不只交叉编译，还验证完整 bundle',
    'release.ciBody': 'CI 校验 bundle ID、版本、lipo 架构、ad-hoc 签名和 DMG 完整性。',
    'release.signTitle': '如实说明签名状态',
    'release.signBody': 'Developer ID 签名与公证仍在 TODO；Gatekeeper 可能拦截当前 ad-hoc 构建。',
    'faq.kicker': '常见问题',
    'faq.title': '你可能想知道。',
    'faq.description': '关于 Peapod 的工作方式和边界。',
    'faq.q1': '它和响应式预览工具有什么不同？',
    'faq.a1': '响应式预览器通常只展示同一个 URL 的多种尺寸。Peapod 面向多个版本的完整流程，支持导航映射、设置方案复用，并记录每一步证据。',
    'faq.q2': 'Peapod 会把我的任务数据发到别处吗？',
    'faq.a2': 'Peapod 没有遥测，不会上传任务 URL、页面内容、Cookie、截图或录制文件。更新只获取公开元数据；点击“下载并安装”后才会获取不含任务数据的对应公开包。',
    'faq.q3': '更新检查如何工作？',
    'faq.a3': '默认手动检查。Windows/Linux 上点击“下载并安装”，会获取匹配包、校验 SHA256、安装并重启；macOS 会打开发布页，可在那里下载 Universal DMG。',
    'faq.q4': '可以复现 Safari 特有的问题吗？',
    'faq.a4': 'User-Agent 预设只改变 UA 字符串，页面仍由当前系统的 WebView 渲染，因此无法复现依赖 Safari 引擎本身的问题。',
    'faq.q5': '支持哪些平台？',
    'faq.a5': '支持 Windows、macOS 和 Linux，分别使用 WebView2、WKWebView 和 WebKitGTK。三者均可构建并通过 CI；macOS 发布 Universal DMG，Windows 仍是运行测试最充分的平台。',
    'faq.q6': '可以模拟弱网吗？',
    'faq.a6': '暂不支持。每视图离线、延迟和限速需要扩展 WebView 后端，详见项目 TODO。',
    'cta.kicker': '开源 · 本地优先 · 为效率而生',
    'cta.title': '一次操作，页页跟随。',
    'cta.description': '让 Peapod 替你照看其他页面，你只需要关注真正重要的那一个。',
    'cta.button': '获取 Peapod',
    'cta.docs': '快速开始',
    'footer.tagline': '一次操作，页页跟随。',
    'footer.docs': '文档',
    'footer.contributors': 'Peapod 贡献者',
    'footer.back': '回到顶部'
  },
  'zh-TW': {
    skip: '跳至主要內容',
    'nav.label': '主導覽',
    'nav.product': '產品',
    'nav.cases': '使用情境',
    'nav.how': '運作方式',
    'nav.mobile': '行動版導覽',
    'brand.home': 'Peapod 首頁',
    'language.label': '語言',
    'menu.open': '開啟選單',
    'menu.close': '關閉選單',
    'page.title': 'Peapod — 一次操作，頁頁跟隨。',
    'meta.description': '一次操作，頁頁跟隨。跨語言、跨環境、跨裝置執行同一條流程，並留下每一步的證據。',
    'hero.badge': 'v0.13.0 · 開源桌面應用程式',
    'hero.title1': '一次操作，',
    'hero.title2': '頁頁跟隨。',
    'hero.description': '在同一視窗中，讓不同語言、環境或裝置版本一起完成同一條流程——並排顯示、即時同步，每一步都有證據。',
    'hero.github': '取得 Peapod',
    'hero.demo': '看看它如何運作',
    'hero.preview': 'Peapod 多頁面同步預覽',
    'hero.live': '即時同步',
    'hero.primary': '主頁',
    'hero.add': '加入購物袋',
    'hero.you': '你',
    'hero.synced': '已同步',
    'hero.zhName': '中文',
    'hero.jaName': '日文',
    'hero.follow1': '相同操作。<br>同步完成。',
    'hero.follow2': '一次操作。<br>頁頁跟隨。',
    'hero.recording': '正在錄製操作與截圖',
    'hero.pages': '頁面就緒',
    'hero.noteTitle': '不必重複操作',
    'hero.noteBody': '點擊、輸入、捲動、送出',
    'hero.local': '任務資料留在本機',
    'hero.noTelemetry': '無遙測 · 無雲端同步',
    'stats.pages': '個頁面，一個工作區',
    'stats.local': '任務資料存於本機',
    'stats.telemetry': '遙測',
    'stats.mcp': '內建 AI Agent 介面',
    'product.kicker': '不只是預覽，而是完整流程',
    'product.title': '一條流程，所有版本。<br>每一步，都有證據。',
    'product.description': '響應式預覽器只改變檢視區尺寸。Peapod 讓不同語言、網域和環境真正走完同一段使用者旅程，並保留完整證據。',
    'product.preview': '真實產品預覽',
    'product.running': '任務執行中',
    'product.imageAlt': 'Peapod 並排執行四個語言版本，並顯示即時同步與錄製狀態',
    'product.locale': '4 種語言，一次驗證',
    'product.record': '操作與截圖自動留存',
    'features.syncTitle': '即時同步，鎖定主頁操作',
    'features.syncBody': '在主頁點擊、輸入、捲動或送出，跟隨頁面會依序即時重播。開啟「僅限主頁操作」可避免誤觸，同步回放與自動化仍會正常運作。',
    'features.click': '點擊',
    'features.input': '輸入',
    'features.submit': '送出',
    'features.allSynced': '全部同步',
    'features.runtimeTitle': '修改 URL，工作階段不中斷',
    'features.runtimeBody': '執行中可直接編輯任一頁面的 URL，在原頁面內重新開啟，儲存空間與 Cookie 都不會遺失。主頁導覽仍會對應到各跟隨頁自己的網域、路徑與查詢參數。',
    'features.canvasTitle': '自由畫布，檢視區等比例縮放',
    'features.canvasBody': '拖曳頁面自由排列，每個檢視區都能等比例縮放。整張畫布可匯出為一張 PNG；單一頁面失敗時會保留含名稱的佔位圖。',
    'features.captureTitle': '依設定解析度截圖',
    'features.captureBody': '批次截圖會依每頁設定的像素尺寸渲染所有執行中的頁面，即使它不在螢幕上。縮放預覽不會限制最終輸出。',
    'features.profilesTitle': '一鍵載入設定檔',
    'features.profilesBody': '把頁面、URL 範本、解析度、User-Agent 與開關儲存為具名設定檔，下次一鍵載入整套配置。',
    'features.profileName': '結帳流程 QA',
    'features.evidenceTitle': '錄製、回放，保留彈出視窗證據',
    'features.evidenceBody': '在時間軸中回顧每一步、重播工作階段、執行文字/DOM/URL/HTTP/Console 斷言，並匯出帶逐頁截圖與跨頁面差異的 PASS/FAIL 報告。',
    'features.mcpTitle': '讓 AI Agent 接手操作',
    'features.mcpBody': '內建由 token 保護的本機 MCP 服務，讓 Agent 無需操作介面，即可啟動、導覽、操作、回放、截圖、驗證、診斷與停止任務。',
    'features.injectTitle': '每個檢視獨立注入測試指令碼',
    'features.injectBody': '在每頁文件開始時執行 JavaScript，於 DOMContentLoaded 後操作 DOM，並把受限的測試事件回傳主機用於診斷。',
    'features.privacyTitle': '本機優先，預設維護隱私',
    'features.privacyBody': '密碼永不錄製，任務停止後工作階段隨即清除，任務檔案只留在你的電腦上。選用的 GitHub 發布元資料檢查會獨立執行，不包含任何任務資料。',
    'features.loopback': '僅限本機迴環存取',
    'cases.kicker': '為真實工作而設計',
    'cases.title': '把重複驗證，<br>變成一次並行檢查。',
    'cases.description': '從上線前的最後檢查，到持續迭代中的迴歸測試，讓每個版本保持同步。',
    'cases.localeTitle': '多語言 QA',
    'cases.localeBody': '用同一套結帳流程檢查英文、中文、日文與韓文，在使用者發現前，找出只在某個語言出問題的地方。',
    'cases.zh': '中文',
    'cases.ja': '日文',
    'cases.ko': '韓文',
    'cases.passed': '流程全部通過',
    'cases.envTitle': '環境比較',
    'cases.envBody': '把 dev、staging 與 prod 並排比較，確認同一個發布在各環境中的行為一致。',
    'cases.rolesTitle': '多帳號與角色',
    'cases.rolesBody': '使用獨立工作階段，分別以管理員、編輯與訪客身分登入，驗證不同權限下的體驗。',
    'cases.isolated': '工作階段彼此隔離',
    'cases.isolatedBody': '登入狀態互不影響',
    'how.kicker': '簡單到現在就能開始',
    'how.title': '新增頁面。<br>開始操作。<br>留下證據。',
    'how.description': '不需要撰寫自動化腳本。像平常一樣使用主頁，Peapod 會讓其他頁面跟上。',
    'how.docs': '閱讀完整快速開始',
    'how.step1Title': '新增你的頁面',
    'how.step1Body': '貼上 URL 清單或使用變數範本，一次設定最多 10 個語言、環境或裝置頁面。',
    'how.step2Title': '選擇並儲存設定',
    'how.step2Body': '設定版面、解析度、User-Agent、工作階段隔離、僅限主頁操作與錄製，再儲存為可重用的設定檔。',
    'how.step3Title': '像平常一樣操作',
    'how.step3Body': '在主頁完成流程，跟隨頁面即時同步。執行中可直接修改 URL，頁面工作階段與 Cookie 都不會遺失。',
    'how.step4Title': '截圖、回放、匯出',
    'how.step4Body': '依設定解析度截圖、查看錄製、執行驗證斷言，再匯出 PASS/FAIL 與差異報告。',
    'install.kicker': 'v0.13.0 · 安裝或從原始碼建置',
    'install.title': '一次下載，<br>進入更高效的 QA。',
    'install.description': '可於 GitHub Releases 取得 Windows 安裝套件、macOS Universal DMG 與 Linux 套件，也可從原始碼建置，無需額外安裝瀏覽器。原始碼建置需要 Go 1.27.4 或更新版本，以及系統 WebView。',
    'install.releases': '從 GitHub Releases 下載',
    'install.webview': '系統 WebView',
    'install.binary': '單一執行檔',
    'install.copy': '複製',
    'install.copyAria': '複製建置命令',
    'install.terminal': '終端機',
    'install.copied': '已複製',
    'install.ready': '建置完成，啟動 Peapod',
    'release.kicker': 'v0.13.0 新功能',
    'release.title': 'Universal macOS。<br>由 CI 建置。',
    'release.macosTitle': '一個 DMG，支援 Apple Silicon 與 Intel',
    'release.macosBody': 'macOS Runner 建置 Peapod.app、驗證雙架構並產生 Universal 磁碟映像。',
    'release.ciTitle': '不只交叉編譯，還驗證完整 bundle',
    'release.ciBody': 'CI 驗證 bundle ID、版本、lipo 架構、ad-hoc 簽署與 DMG 完整性。',
    'release.signTitle': '如實說明簽署狀態',
    'release.signBody': 'Developer ID 簽署與公證仍在 TODO；Gatekeeper 可能攔截目前的 ad-hoc 建置。',
    'faq.kicker': '常見問題',
    'faq.title': '你可能想知道。',
    'faq.description': '關於 Peapod 的運作方式與界線。',
    'faq.q1': '它與響應式預覽工具有何不同？',
    'faq.a1': '響應式預覽器通常只顯示同一個 URL 的多種尺寸。Peapod 面向多個版本的完整流程，支援導覽對應、設定檔重用，並記錄每一步證據。',
    'faq.q2': 'Peapod 會把任務資料傳到別處嗎？',
    'faq.a2': 'Peapod 沒有遙測，不會上傳任務 URL、頁面內容、Cookie、截圖或錄製檔案。更新只取得公開元資料；點擊「下載並安裝」後才會取得不含任務資料的相符公開套件。',
    'faq.q3': '更新檢查如何運作？',
    'faq.a3': '預設手動檢查。Windows/Linux 上點擊「下載並安裝」，會取得相符套件、校驗 SHA256、安裝並重新啟動；macOS 會開啟發行頁，可在那裡下載 Universal DMG。',
    'faq.q4': '可以重現 Safari 特有的問題嗎？',
    'faq.a4': 'User-Agent 預設只改變 UA 字串，頁面仍由目前系統的 WebView 渲染，因此無法重現依賴 Safari 引擎本身的問題。',
    'faq.q5': '支援哪些平台？',
    'faq.a5': '支援 Windows、macOS 與 Linux，分別使用 WebView2、WKWebView 和 WebKitGTK。三者均可建置並通過 CI；macOS 發布 Universal DMG，Windows 仍是執行期測試最充分的平台。',
    'faq.q6': '可以模擬弱網嗎？',
    'faq.a6': '暫不支援。每檢視離線、延遲和限速需要擴充 WebView 後端，詳見專案 TODO。',
    'cta.kicker': '開源 · 本機優先 · 為效率而生',
    'cta.title': '一次操作，頁頁跟隨。',
    'cta.description': '讓 Peapod 替你看著其他頁面，你只需專注在真正重要的那一個。',
    'cta.button': '取得 Peapod',
    'cta.docs': '快速開始',
    'footer.tagline': '一次操作，頁頁跟隨。',
    'footer.docs': '文件',
    'footer.contributors': 'Peapod 貢獻者',
    'footer.back': '回到頂端'
  },
  ja: {
    skip: 'メインコンテンツへ移動',
    'nav.label': 'メインナビゲーション',
    'nav.product': '製品',
    'nav.cases': 'ユースケース',
    'nav.how': '使い方',
    'nav.mobile': 'モバイルナビゲーション',
    'brand.home': 'Peapod ホーム',
    'language.label': '言語',
    'menu.open': 'メニューを開く',
    'menu.close': 'メニューを閉じる',
    'page.title': 'Peapod — 繰り返しを減らして、気づきを増やす。',
    'meta.description': '繰り返しを減らして、気づきを増やす。言語や環境、デバイスをまたいで同じフローを実行し、各ステップの証拠を残せます。',
    'hero.badge': 'v0.13.0 · オープンソースのデスクトップアプリ',
    'hero.title1': '繰り返しを減らして、',
    'hero.title2': '気づきを増やす。',
    'hero.description': '言語や環境、デバイスの違いをまたいで、同じフローを並行実行。リアルタイムで同期し、各ステップの証拠も残せます。',
    'hero.github': 'Peapod を入手',
    'hero.demo': '使い方を見る',
    'hero.preview': 'Peapod のページ同期プレビュー',
    'hero.live': 'ライブ同期',
    'hero.primary': 'メインページ',
    'hero.add': 'カートに追加',
    'hero.you': 'あなた',
    'hero.synced': '同期済み',
    'hero.zhName': '中国語',
    'hero.jaName': '日本語',
    'hero.follow1': '同じ操作。<br>同期完了。',
    'hero.follow2': 'ひとつの操作。<br>すべてのページが追随。',
    'hero.recording': '操作とスクリーンショットを記録中',
    'hero.pages': 'ページ準備完了',
    'hero.noteTitle': '繰り返しは不要',
    'hero.noteBody': 'クリック、入力、スクロール、送信',
    'hero.local': 'タスクデータはローカルに保持',
    'hero.noTelemetry': 'テレメトリなし · クラウド同期なし',
    'stats.pages': 'ページをひとつのワークスペースに',
    'stats.local': 'タスクデータはローカル',
    'stats.telemetry': 'テレメトリ',
    'stats.mcp': 'AI エージェント用機能を内蔵',
    'product.kicker': 'プレビューではなく、ひとつの完結したフロー',
    'product.title': 'ひとつのフロー。すべてのバージョン。<br>各ステップの証拠を残す。',
    'product.description': 'レスポンシブプレビューは表示サイズを変えるだけ。Peapod は言語、ドメイン、環境の異なる実際のユーザー体験を並行実行し、証拠を保存します。',
    'product.preview': '実際の製品プレビュー',
    'product.running': 'タスク実行中',
    'product.imageAlt': 'Peapod が4つの言語バリアントを並行実行し、ライブ同期と録画状態を表示',
    'product.locale': '4言語をひとつのフローで検証',
    'product.record': '操作とスクリーンショットを自動保存',
    'features.syncTitle': 'ライブ同期とメインページ限定ロック',
    'features.syncBody': 'メインページでクリック、入力、スクロール、送信を行うと、ほかのページが順番に再生されます。追随ページの入力をロックしても、同期再生や自動化は停止しません。',
    'features.click': 'クリック',
    'features.input': '入力',
    'features.submit': '送信',
    'features.allSynced': 'すべて同期済み',
    'features.runtimeTitle': 'URL を変更してもセッションを維持',
    'features.runtimeBody': '実行中の任意のページで URL を編集し、同じページ内で再表示できます。ストレージや Cookie は失われません。メインページのナビゲーションは、各追随ページのホスト、パス、クエリに対応します。',
    'features.canvasTitle': '自由キャンバスと等比例のビュー',
    'features.canvasBody': 'ページをドラッグして配置し、各ビューを等比例でリサイズできます。配置したキャンバス全体を1枚の PNG に保存でき、失敗したページには名前付きのプレースホルダーを残します。',
    'features.captureTitle': '設定した解像度でスクリーンショット',
    'features.captureBody': '一括撮影では、画面に表示されていない実行中のページも、設定されたピクセルサイズでレンダリングします。縮小プレビューが出力を制限することはありません。',
    'features.profilesTitle': '再利用できるセットアッププロファイル',
    'features.profilesBody': 'ページ、URL テンプレート、解像度、User-Agent、各種トグルを名前付きで保存し、ワンクリックで設定全体を読み込めます。',
    'features.profileName': '購入フロー QA',
    'features.evidenceTitle': '録画、再生、ポップアップの証拠',
    'features.evidenceBody': 'ステップのタイムラインを確認し、セッションを再生し、テキスト/DOM/URL/HTTP/Console アサーションを実行して、ページ別スクリーンショットとページ間差分を含む PASS/FAIL レポートを出力できます。',
    'features.mcpTitle': 'AI エージェントに操作を任せる',
    'features.mcpBody': 'トークンで保護されたローカル MCP サーバーにより、UI を操作せずにタスクの開始、ナビゲーション、操作、再生、撮影、検証、診断、停止を行えます。',
    'features.injectTitle': 'ビュー別にテストスクリプトを注入',
    'features.injectBody': '各ページのドキュメント開始時に JavaScript を実行し、DOMContentLoaded 後に DOM を操作して、限定されたテストイベントをホストへ返して診断できます。',
    'features.privacyTitle': 'ローカル優先。プライバシーを標準に。',
    'features.privacyBody': 'パスワードは記録されず、タスク停止時にセッションは消去されます。タスクファイルはコンピューター内に保存され、任意利用の GitHub リリースメタデータ確認にはタスクデータが含まれません。',
    'features.loopback': 'ループバック接続のみ',
    'cases.kicker': '実際の仕事のために設計',
    'cases.title': '繰り返す確認を、<br>ひとつの並行チェックに。',
    'cases.description': 'リリース前の最終確認から、日々の反復テストまで、すべてのバージョンを同期させます。',
    'cases.localeTitle': '多言語 QA',
    'cases.localeBody': '同じ購入フローを英語、中国語、日本語、韓国語で実行し、ユーザーが気づく前に、問題のある言語を特定します。',
    'cases.zh': '中国語',
    'cases.ja': '日本語',
    'cases.ko': '韓国語',
    'cases.passed': 'すべてのフローが成功',
    'cases.envTitle': '環境比較',
    'cases.envBody': 'dev、staging、prod を並べて表示し、同じリリースが各環境で同じように動作することを確認します。',
    'cases.rolesTitle': '複数アカウントと権限',
    'cases.rolesBody': '独立したセッションで管理者、編集者、ゲストとしてログインし、権限ごとの体験を検証します。',
    'cases.isolated': 'セッションを分離',
    'cases.isolatedBody': 'ログイン状態は互いに独立',
    'how.kicker': '今すぐ始められるシンプルさ',
    'how.title': 'ページを追加。<br>操作を開始。<br>証拠を残す。',
    'how.description': '自動化スクリプトは不要です。メインページをいつもどおり操作すれば、Peapod がほかのページを同期させます。',
    'how.docs': 'クイックスタートを読む',
    'how.step1Title': 'ページを追加',
    'how.step1Body': 'URL のリストを貼り付けるか、変数テンプレートを使います。言語、環境、デバイスを合わせて最大10ページまで設定できます。',
    'how.step2Title': '設定を選んで保存',
    'how.step2Body': 'レイアウト、解像度、User-Agent、セッション分離、メインページ限定入力、録画を設定し、再利用できるプロファイルとして保存します。',
    'how.step3Title': 'いつもどおり操作',
    'how.step3Body': 'メインページでフローを実行すると、追随ページが同期します。実行中の URL を直接編集しても、セッションや Cookie は失われません。',
    'how.step4Title': '撮影、再生、出力',
    'how.step4Body': '設定解像度で撮影し、録画を確認し、検証アサーションを実行して、PASS/FAIL と差分レポートを出力します。',
    'install.kicker': 'v0.13.0 · インストールまたはソースからビルド',
    'install.title': 'ひとつのダウンロードで、<br>QA をもっと軽やかに。',
    'install.description': 'Windows インストーラー、macOS Universal DMG、Linux パッケージは GitHub Releases から入手できます。ソースからのビルドには Go 1.27.4 以上とシステムの WebView が必要ですが、別途ブラウザーを入れる必要はありません。',
    'install.releases': 'GitHub Releases からダウンロード',
    'install.webview': 'システム WebView',
    'install.binary': '単一の実行ファイル',
    'install.copy': 'コピー',
    'install.copyAria': 'ビルドコマンドをコピー',
    'install.terminal': 'ターミナル',
    'install.copied': 'コピーしました',
    'install.ready': 'ビルド完了。Peapod を起動します。',
    'release.kicker': 'v0.13.0 の新機能',
    'release.title': 'Universal macOS。<br>CI でビルド。',
    'release.macosTitle': 'Apple Silicon と Intel に対応する 1 つの DMG',
    'release.macosBody': 'macOS Runner が Peapod.app をビルドし、両方のアーキテクチャを検証して Universal ディスクイメージを作成します。',
    'release.ciTitle': 'クロスビルドだけでなく bundle を検証',
    'release.ciBody': 'CI は bundle ID、バージョン、lipo アーキテクチャ、ad-hoc 署名、DMG の整合性を確認します。',
    'release.signTitle': '署名状態を明確に表示',
    'release.signBody': 'Developer ID 署名と公証は TODO のままです。現在の ad-hoc ビルドは Gatekeeper にブロックされる可能性があります。',
    'faq.kicker': 'よくある質問',
    'faq.title': '知っておきたいこと。',
    'faq.description': 'Peapod の仕組みと適用範囲について。',
    'faq.q1': 'レスポンシブプレビューとの違いは？',
    'faq.a1': 'レスポンシブプレビューは通常、同じ URL を複数の表示サイズで見せます。Peapod は複数バージョンのフローを対象に、ナビゲーションの対応、設定の再利用、各ステップの証拠を提供します。',
    'faq.q2': 'タスクデータをどこかに送信しますか？',
    'faq.a2': 'Peapod にはテレメトリがなく、タスクの URL、ページ内容、Cookie、スクリーンショット、録画ファイルをアップロードしません。更新は公開メタデータのみを取得し、「ダウンロードしてインストール」を押した場合だけ、タスクデータを含まない対応する公開パッケージを取得します。',
    'faq.q3': '更新確認はどのように動作しますか？',
    'faq.a3': '既定は手動確認です。Windows/Linux では「ダウンロードしてインストール」から対応パッケージを取得し、SHA256 を検証してインストール後再起動します。macOS はリリースページを開き、Universal DMG をダウンロードできます。',
    'faq.q4': 'Safari 固有の不具合を再現できますか？',
    'faq.a4': 'User-Agent プリセットは UA 文字列だけを変更します。ページは各プラットフォームの WebView で描画されるため、Safari エンジン固有の不具合は再現できません。',
    'faq.q5': '対応プラットフォームは？',
    'faq.a5': 'Windows、macOS、Linux に対応し、それぞれ WebView2、WKWebView、WebKitGTK を使用します。3 つともビルドと CI を通過し、macOS は Universal DMG を公開します。実行時のテストは Windows が最も充実しています。',
    'faq.q6': '低速ネットワークをシミュレートできますか？',
    'faq.a6': '未実装です。ビュー別のオフライン、遅延、スループット制御には WebView バックエンドの拡張が必要です。プロジェクトの TODO を参照してください。',
    'cta.kicker': 'オープンソース · ローカル優先 · 集中するために',
    'cta.title': '繰り返しを減らして、気づきを増やす。',
    'cta.description': 'ほかのページは Peapod に任せて、大切なひとつのページに集中できます。',
    'cta.button': 'Peapod を入手',
    'cta.docs': 'クイックスタート',
    'footer.tagline': '繰り返しを減らして、気づきを増やす。',
    'footer.docs': 'ドキュメント',
    'footer.contributors': 'Peapod コントリビューター',
    'footer.back': 'ページ上部へ'
  }
};

const supportedLanguages = ['en', 'zh', 'zh-TW', 'ja'];
const languageAttributes = {
  en: 'en',
  zh: 'zh-CN',
  'zh-TW': 'zh-Hant',
  ja: 'ja'
};

const getSavedLanguage = () => {
  try {
    return localStorage.getItem('peapod-language');
  } catch {
    return null;
  }
};

const savedLanguage = getSavedLanguage();
let currentLanguage = supportedLanguages.includes(savedLanguage) ? savedLanguage : 'en';

function setLanguage(language, persist = true) {
  if (!supportedLanguages.includes(language)) return;
  currentLanguage = language;
  const dictionary = translations[language];
  document.documentElement.lang = languageAttributes[language];
  document.title = dictionary['page.title'];
  document.querySelectorAll('[data-i18n]').forEach((element) => {
    const key = element.dataset.i18n;
    if (dictionary[key]) element.innerHTML = dictionary[key];
  });
  document.querySelectorAll('[data-i18n-aria]').forEach((element) => {
    const key = element.dataset.i18nAria;
    if (dictionary[key]) element.setAttribute('aria-label', dictionary[key]);
  });
  document.querySelectorAll('[data-i18n-alt]').forEach((element) => {
    const key = element.dataset.i18nAlt;
    if (dictionary[key]) element.setAttribute('alt', dictionary[key]);
  });
  document.querySelectorAll('[data-i18n-content]').forEach((element) => {
    const key = element.dataset.i18nContent;
    if (dictionary[key]) element.setAttribute('content', dictionary[key]);
  });
  document.querySelectorAll('[data-lang]').forEach((button) => {
    button.setAttribute('aria-pressed', String(button.dataset.lang === language));
  });
  syncMenuLabel();
  if (!persist) return;
  try {
    localStorage.setItem('peapod-language', language);
  } catch {
    // Language switching still works when storage is unavailable.
  }
}

document.documentElement.classList.remove('no-js');

document.querySelectorAll('[data-lang]').forEach((button) => {
  button.addEventListener('click', () => setLanguage(button.dataset.lang));
});

const menuToggle = document.querySelector('.menu-toggle');
const mobileNav = document.querySelector('.mobile-nav');
function syncMenuLabel() {
  const isOpen = menuToggle.getAttribute('aria-expanded') === 'true';
  menuToggle.setAttribute('aria-label', translations[currentLanguage][isOpen ? 'menu.close' : 'menu.open']);
}
function closeMenu() {
  menuToggle.setAttribute('aria-expanded', 'false');
  mobileNav.classList.remove('is-open');
  mobileNav.inert = true;
  document.body.classList.remove('menu-open');
  syncMenuLabel();
}
setLanguage(currentLanguage, false);
menuToggle.addEventListener('click', () => {
  const isOpen = menuToggle.getAttribute('aria-expanded') === 'true';
  menuToggle.setAttribute('aria-expanded', String(!isOpen));
  mobileNav.classList.toggle('is-open', !isOpen);
  mobileNav.inert = isOpen;
  document.body.classList.toggle('menu-open', !isOpen);
  syncMenuLabel();
});
mobileNav.querySelectorAll('a').forEach((link) => link.addEventListener('click', closeMenu));
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && menuToggle.getAttribute('aria-expanded') === 'true') {
    closeMenu();
    menuToggle.focus();
  }
});
window.addEventListener('resize', () => {
  if (window.innerWidth > 860) closeMenu();
});

const siteHeader = document.querySelector('.site-header');
const updateHeader = () => siteHeader.classList.toggle('is-scrolled', window.scrollY > 12);
window.addEventListener('scroll', updateHeader, { passive: true });
updateHeader();

const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
const revealElements = document.querySelectorAll('.reveal');
if (reducedMotion || !('IntersectionObserver' in window)) {
  revealElements.forEach((element) => element.classList.add('is-visible'));
} else {
  const revealObserver = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) {
        entry.target.classList.add('is-visible');
        revealObserver.unobserve(entry.target);
      }
    });
  }, { threshold: 0.08, rootMargin: '0px 0px -40px' });
  revealElements.forEach((element) => revealObserver.observe(element));
}

let syncTimer;
function runSyncDemo() {
  const trigger = document.querySelector('[data-sync-demo]');
  const followers = [...document.querySelectorAll('[data-follower]')];
  clearTimeout(syncTimer);
  followers.forEach((page) => page.classList.remove('is-syncing', 'is-complete'));
  trigger.classList.add('is-running');
  followers.forEach((page, index) => {
    window.setTimeout(() => {
      page.classList.add('is-syncing');
      window.setTimeout(() => {
        page.classList.remove('is-syncing');
        page.classList.add('is-complete');
        if (index === followers.length - 1) trigger.classList.remove('is-running');
      }, reducedMotion ? 50 : 520);
    }, reducedMotion ? index * 60 : index * 260);
  });
  syncTimer = window.setTimeout(() => {
    followers.forEach((page) => page.classList.remove('is-complete'));
    trigger.classList.remove('is-running');
  }, reducedMotion ? 1000 : 3600);
}
document.querySelector('[data-sync-demo]').addEventListener('click', runSyncDemo);
window.setTimeout(runSyncDemo, reducedMotion ? 0 : 1300);

const copyButton = document.querySelector('.copy-button');
const installCommand = 'git clone https://github.com/jichao99/peapod.git\ncd peapod\ngo build .';
copyButton.addEventListener('click', () => {
  copyButton.classList.add('is-copied');
  copyButton.querySelector('span').textContent = translations[currentLanguage]['install.copied'];
  copyButton.querySelector('use').setAttribute('href', '#i-check');
  window.setTimeout(() => {
    copyButton.classList.remove('is-copied');
    copyButton.querySelector('span').textContent = translations[currentLanguage]['install.copy'];
    copyButton.querySelector('use').setAttribute('href', '#i-copy');
  }, 1800);
  const textarea = document.createElement('textarea');
  textarea.value = installCommand;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  try {
    if (!document.execCommand('copy') && navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(installCommand).catch(() => {});
    }
  } catch {
    navigator.clipboard?.writeText(installCommand).catch(() => {});
  } finally {
    textarea.remove();
  }
});

document.querySelectorAll('.faq-list details').forEach((detail) => {
  detail.addEventListener('toggle', () => {
    if (detail.open) {
      document.querySelectorAll('.faq-list details').forEach((other) => {
        if (other !== detail) other.open = false;
      });
    }
  });
});

document.querySelector('#year').textContent = new Date().getFullYear();
