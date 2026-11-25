const translations = {
  zh: {
    skip: '跳到主要内容',
    'nav.product': '产品',
    'nav.cases': '使用场景',
    'nav.how': '工作原理',
    'menu.open': '打开菜单',
    'menu.close': '关闭菜单',
    'hero.badge': '开源桌面应用 · Go + MyGo',
    'hero.title1': '操作一次。',
    'hero.title2': '所有页面同步跟随。',
    'hero.description': '在同一个窗口里，让多个语言、环境或设备版本的网站一起完成同一个流程。少一点重复验证，多一点确定性。',
    'hero.github': '在 GitHub 上查看',
    'hero.demo': '看看它如何工作',
    'hero.live': '实时同步',
    'hero.primary': '主页面',
    'hero.add': '加入购物袋',
    'hero.you': '你',
    'hero.synced': '已同步',
    'hero.follow1': '相同操作<br>同步完成',
    'hero.follow2': '同じ操作を<br>一度で実行',
    'hero.recording': '正在记录操作与截图',
    'hero.pages': '页面就绪',
    'hero.noteTitle': '无需重复操作',
    'hero.noteBody': '点击、输入、滚动、提交',
    'hero.local': '数据留在本地',
    'hero.noTelemetry': '无遥测 · 无云端上传',
    'stats.pages': '个页面，同屏协作',
    'stats.local': '本地运行',
    'stats.telemetry': '遥测与数据上传',
    'stats.mcp': '为 AI Agent 做好准备',
    'product.kicker': '不只是预览，而是完整流程',
    'product.title': '一个流程，所有版本。<br>不再漏掉任何细节。',
    'product.description': '响应式预览器只改变尺寸。Peapod 让不同语言、域名和环境真正走完同一个用户旅程，并保存每一步的证据。',
    'product.preview': '真实产品预览',
    'product.running': '任务运行中',
    'product.locale': '4 种语言，一次验证',
    'product.record': '操作与截图自动留存',
    'features.syncTitle': '实时同步，不静默掉队',
    'features.syncBody': '在主页面点击、输入、滚动或提交，其他页面按顺序实时重放。任何一步失败都会暂停并明确报错。',
    'features.click': '点击',
    'features.input': '输入',
    'features.submit': '提交',
    'features.allSynced': '全部同步',
    'features.navTitle': '导航自动映射',
    'features.navBody': '跳转和重定向会映射到每个页面自己的域名、路径和查询参数，始终停留在对应版本。',
    'features.evidenceTitle': '每一步，都有证据',
    'features.evidenceBody': '按配置分辨率批量截图，保留操作时间线和可重放脚本，一键导出为可附到 PR 的 HTML 报告。',
    'features.deviceTitle': '尺寸、设备与会话',
    'features.deviceBody': '自由画布独立缩放每个视图，200–3840px 自定义分辨率、常用 User-Agent 预设，以及独立或共享的临时会话。',
    'features.mcpTitle': '让 AI Agent 来驱动',
    'features.mcpBody': '内置本地 MCP 服务，支持启动、导航、操作、回放、截图和诊断。',
    'features.privacyTitle': '本地优先，隐私默认',
    'features.privacyBody': '密码永不录制，任务结束即清除会话。截图与记录只保存在你的电脑上。',
    'features.loopback': '仅本机回环访问',
    'cases.kicker': '为真实工作而设计',
    'cases.title': '把重复验证，<br>变成一次并行检查。',
    'cases.description': '从上线前的最后一遍检查，到持续迭代中的回归验证，让每个版本保持同步。',
    'cases.localeTitle': '多语言 QA',
    'cases.localeBody': '用同一套结账流程检查英语、中文、日语和韩语，快速定位只在某个语言出现的问题。',
    'cases.passed': '流程全部通过',
    'cases.envTitle': '环境对比',
    'cases.envBody': '把 dev、staging 和 prod 并排放在一起，确认一次发布在每个环境中的行为一致。',
    'cases.rolesTitle': '多账号与角色',
    'cases.rolesBody': '每个页面使用独立会话，分别登录管理员、编辑和访客，验证不同权限下的体验。',
    'cases.isolated': '会话彼此隔离',
    'cases.isolatedBody': '登录状态互不影响',
    'how.kicker': '简单到可以马上开始',
    'how.title': '添加页面。<br>开始操作。<br>拿走证据。',
    'how.description': '不需要编写自动化脚本。像平时一样使用主页面，Peapod 负责让其他页面跟上。',
    'how.docs': '阅读完整快速开始',
    'how.step1Title': '添加你的页面',
    'how.step1Body': '粘贴 URL 列表或使用变量模板，一次配置最多 10 个语言、环境或设备页面。',
    'how.step2Title': '选择任务设置',
    'how.step2Body': '设置桌面、移动或自由画布，调整分辨率、User-Agent 和会话隔离方式，按需开启录制。',
    'how.step3Title': '像平时一样操作',
    'how.step3Body': '只在主页面完成流程，其他页面实时同步。任何异常都会立即显示。',
    'how.step4Title': '截图、回放、导出',
    'how.step4Body': '从时间线回顾每一步，按配置分辨率截图或导出 HTML 报告，把证据带进评审。',
    'install.kicker': '从源码构建',
    'install.title': '几分钟，<br>开始你的并行验证。',
    'install.description': 'Peapod 目前从源码构建，无需安装额外浏览器。需要 Go 1.27.4 或更新版本，以及系统 WebView。',
    'install.webview': '系统 WebView',
    'install.binary': '单二进制应用',
    'install.copy': '复制',
    'install.copyAria': '复制安装命令',
    'install.ready': '构建完成，启动 Peapod',
    'faq.kicker': '常见问题',
    'faq.title': '你可能想知道。',
    'faq.description': '关于 Peapod 的工作方式和边界。',
    'faq.q1': '它和响应式预览工具有什么不同？',
    'faq.a1': '响应式预览器通常只展示同一个 URL 的多种尺寸。Peapod 面向多个版本的完整流程，支持不同语言、域名和环境，并同步导航、记录每一步。',
    'faq.q2': '它会把我的数据发送到哪里？',
    'faq.a2': '不会。Peapod 没有遥测，也不会主动发起网络请求。页面只访问你打开的网站，截图和录制文件只保存在本地。',
    'faq.q3': '可以复现 Safari 特有的问题吗？',
    'faq.a3': 'User-Agent 预设只改变 UA 字符串，页面仍使用当前系统的 WebView 渲染。因此无法用它复现依赖 Safari 引擎本身的问题。',
    'faq.q4': '支持哪些平台？',
    'faq.a4': '支持 Windows、macOS 和 Linux，分别使用 WebView2、WKWebView 和 WebKitGTK。Windows 是目前测试最充分的平台。',
    'cta.kicker': '开源 · 本地 · 为效率而生',
    'cta.title': '少重复一次，<br>多确定一步。',
    'cta.description': '让 Peapod 替你照看其他页面，你只需要关注真正重要的那一个。',
    'cta.button': '探索 Peapod',
    'cta.docs': '快速开始',
    'footer.tagline': '操作一次，所有页面同步跟随。',
    'footer.docs': '文档',
    'footer.back': '回到顶部'
  },
  en: {
    skip: 'Skip to main content',
    'nav.product': 'Product',
    'nav.cases': 'Use cases',
    'nav.how': 'How it works',
    'menu.open': 'Open menu',
    'menu.close': 'Close menu',
    'hero.badge': 'Open-source desktop app · Go + MyGo',
    'hero.title1': 'Operate once.',
    'hero.title2': 'Every page follows.',
    'hero.description': 'Walk multiple language, environment, or device versions of a site through one flow — side by side, in sync, with evidence.',
    'hero.github': 'View on GitHub',
    'hero.demo': 'See how it works',
    'hero.live': 'Live sync',
    'hero.primary': 'Primary',
    'hero.add': 'Add to bag',
    'hero.you': 'You',
    'hero.synced': 'Synced',
    'hero.follow1': 'Same action.<br>Perfectly mirrored.',
    'hero.follow2': 'One action.<br>Every page follows.',
    'hero.recording': 'Recording actions & screenshots',
    'hero.pages': 'pages ready',
    'hero.noteTitle': 'No repeated work',
    'hero.noteBody': 'Click, type, scroll, submit',
    'hero.local': 'Your data stays local',
    'hero.noTelemetry': 'No telemetry · No cloud upload',
    'stats.pages': 'pages, working together',
    'stats.local': 'local-first',
    'stats.telemetry': 'telemetry or cloud upload',
    'stats.mcp': 'ready for AI agents',
    'product.kicker': 'More than a preview. A complete flow.',
    'product.title': 'One flow. Every version.<br>No detail left behind.',
    'product.description': 'Responsive previewers only change viewport sizes. Peapod runs real user journeys across languages, domains, and environments — and keeps evidence of every step.',
    'product.preview': 'Real product preview',
    'product.running': 'Task running',
    'product.locale': '4 languages, one pass',
    'product.record': 'Actions & screenshots, captured',
    'features.syncTitle': 'Live sync that never fails silently',
    'features.syncBody': 'Click, type, scroll, and submit on the primary page. Every other page replays each action in order, pausing with a clear error if anything fails.',
    'features.click': 'Click',
    'features.input': 'Input',
    'features.submit': 'Submit',
    'features.allSynced': 'All synced',
    'features.navTitle': 'Navigation, automatically mapped',
    'features.navBody': 'Redirects and history moves map to each follower’s own host, path, and query parameters, so every version stays on the same step.',
    'features.evidenceTitle': 'Evidence for every step',
    'features.evidenceBody': 'Capture every page at its configured resolution, keep a step timeline and replayable scripts, and export a self-contained HTML report for your next PR or bug.',
    'features.deviceTitle': 'Sizes, devices, and sessions',
    'features.deviceBody': 'Scale each view independently on a free canvas, with custom resolutions from 200 to 3840px, common User-Agent presets, and ephemeral sessions that can be isolated or shared.',
    'features.mcpTitle': 'Let an AI agent take the wheel',
    'features.mcpBody': 'A built-in local MCP server lets agents start, navigate, operate, replay, screenshot, and diagnose tasks.',
    'features.privacyTitle': 'Local-first. Private by default.',
    'features.privacyBody': 'Passwords are never recorded, and sessions are wiped when a task stops. Screenshots and recordings never leave your machine.',
    'features.loopback': 'Loopback access only',
    'cases.kicker': 'Built for real work',
    'cases.title': 'Turn repeated checks<br>into one parallel pass.',
    'cases.description': 'From the final check before release to regression testing between iterations, keep every version moving together.',
    'cases.localeTitle': 'Multi-locale QA',
    'cases.localeBody': 'Run one checkout flow across English, Chinese, Japanese, and Korean. Find the one locale that breaks before your users do.',
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
    'how.step2Title': 'Choose your setup',
    'how.step2Body': 'Pick desktop, mobile, or a free canvas, then set resolutions, User-Agent presets, and session isolation. Turn on recording when you need it.',
    'how.step3Title': 'Operate as usual',
    'how.step3Body': 'Complete the flow on the primary page. Every follower stays live in sync, and any problem is surfaced immediately.',
    'how.step4Title': 'Screenshot, replay, export',
    'how.step4Body': 'Review every step in the timeline, capture at the configured resolution or export an HTML report, and bring the evidence to your review.',
    'install.kicker': 'Build from source',
    'install.title': 'From clone to checkout<br>in a few minutes.',
    'install.description': 'Peapod is currently built from source, with no extra browser to install. You need Go 1.27.4+ and your system WebView.',
    'install.webview': 'System WebView',
    'install.binary': 'Single binary',
    'install.copy': 'Copy',
    'install.copyAria': 'Copy install commands',
    'install.ready': 'Build complete. Launch Peapod.',
    'faq.kicker': 'FAQ',
    'faq.title': 'Good to know.',
    'faq.description': 'How Peapod works — and where its boundaries are.',
    'faq.q1': 'How is this different from a responsive previewer?',
    'faq.a1': 'Responsive previewers usually show one URL at several viewport sizes. Peapod is built for several versions of a complete flow, with mapped navigation and a record of every step.',
    'faq.q2': 'Does Peapod send my data anywhere?',
    'faq.a2': 'No. Peapod has no telemetry and makes no network calls of its own. Pages only visit the sites you open, and screenshots and recordings stay on your machine.',
    'faq.q3': 'Can it reproduce Safari-specific bugs?',
    'faq.a3': 'User-Agent presets only change the UA string. Pages still render in your platform WebView, so engine-specific Safari bugs cannot be reproduced this way.',
    'faq.q4': 'Which platforms are supported?',
    'faq.a4': 'Windows, macOS, and Linux via WebView2, WKWebView, and WebKitGTK respectively. Windows is currently the most thoroughly tested platform.',
    'cta.kicker': 'Open source · Local-first · Built for focus',
    'cta.title': 'Repeat less.<br>Know more.',
    'cta.description': 'Let Peapod watch every other page while you focus on the one that matters.',
    'cta.button': 'Explore Peapod',
    'cta.docs': 'Quick start',
    'footer.tagline': 'Operate once. Every page follows.',
    'footer.docs': 'Docs',
    'footer.back': 'Back to top'
  }
};

const getSavedLanguage = () => {
  try {
    return localStorage.getItem('peapod-language');
  } catch {
    return null;
  }
};

let currentLanguage = getSavedLanguage() === 'en' ? 'en' : 'zh';

function setLanguage(language) {
  currentLanguage = language;
  const dictionary = translations[language];
  document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en';
  document.title = language === 'zh'
    ? 'Peapod — 操作一次，所有页面同步跟随'
    : 'Peapod — Operate once. Every page follows.';
  document.querySelectorAll('[data-i18n]').forEach((element) => {
    const key = element.dataset.i18n;
    if (dictionary[key]) element.innerHTML = dictionary[key];
  });
  document.querySelectorAll('[data-i18n-aria]').forEach((element) => {
    const key = element.dataset.i18nAria;
    if (dictionary[key]) element.setAttribute('aria-label', dictionary[key]);
  });
  document.querySelectorAll('[data-lang]').forEach((button) => {
    button.setAttribute('aria-pressed', String(button.dataset.lang === language));
  });
  syncMenuLabel();
  try {
    localStorage.setItem('peapod-language', language);
  } catch {
    return;
  }
}

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
setLanguage(currentLanguage);
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
  copyButton.querySelector('span').textContent = currentLanguage === 'zh' ? '已复制' : 'Copied';
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
