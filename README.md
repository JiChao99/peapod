<p align="center">
  <img src="assets/brand/lockup.svg" alt="Peapod" width="340">
</p>

<p align="center">
  <b>Operate once. Every page follows.</b><br>
  A desktop app that runs one flow across many versions of a site — side by side, in sync, with evidence.
</p>

<p align="center">
  <a href="https://github.com/JiChao99/peapod/actions/workflows/ci.yml"><img src="https://github.com/JiChao99/peapod/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-2c9a48.svg" alt="License: MIT"></a>
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-2c9a48.svg" alt="Platforms">
  <a href="https://modelcontextprotocol.io"><img src="https://img.shields.io/badge/MCP-ready-2c9a48.svg" alt="MCP ready"></a>
  <a href="https://awesomemygo.dev/apps/peapod"><img src="https://awesomemygo.dev/badge.svg" alt="Listed on Awesome MyGo"></a>
</p>

<p align="center">
  English · <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <img src="docs/images/mcp-ready-panel.png" alt="Peapod running four locale variants of one site side by side in mobile mode, with per-page sync toggles and a live recording badge">
</p>

Peapod opens 2–10 pages in one window. You click, type, scroll and submit on the **primary** page; every other page replays the same operations live, in order. Point the pages at different languages, domains or environments and you can walk through one flow on all of them at once, then keep screenshots and a replayable script of what happened.

It is a native Go app built with [MyGo](https://github.com/egoist/mygo), with its interface made from [Lobe MyGo](https://github.com/lobehub/lobe-mygo) components (the Lobe UI design language for native Go). It ships as a single binary that uses the system webview, with no bundled browser.

## Use cases

- **Multi-locale QA.** Run `/en-us`, `/zh-cn`, `/ja-jp` and `/ko-kr` through the same checkout and spot the one that breaks.
- **Environment comparison.** Put dev, staging and prod side by side and confirm a release behaves the same everywhere.
- **Multi-account and role flows.** Isolated sessions let each page sign in as a different user.
- **Device-sniffing checks.** Serve iPhone, Android or desktop user agents to see what each one gets.
- **Regression evidence.** Record a flow once and get per-step screenshots of every page, a replayable script and an HTML report to attach to a PR or bug.
- **Agent-driven acceptance.** Let an AI agent start a task, drive it, and check the results through the built-in MCP server.

## Quick start

1. **Add pages.** The UI starts in Chinese; switch to English from the language menu in the top bar. Enter the primary URL and add up to 10 pages. A page left empty inherits the primary URL. Paste a URL list under **Bulk add** to generate several pages at once, or duplicate any row and tweak the difference. To work with variants, set a template such as `https://{host}/{lang}` and give each page its own values (`host=staging.example.com, lang=de`).
2. **Choose the setup.** Pick desktop mode (one large page at a time) or mobile mode (all pages side by side), set each page's resolution, pick one User-Agent preset for the whole task (by default it follows the mode, so mobile mode uses iPhone), choose shared or isolated storage, and optionally turn on **Record operations**.
3. **Start and operate.** Use the primary page as you normally would. The other pages follow. Click a page card to bring that page to the front.
4. **Collect evidence.** Take a batch screenshot of every page, or open **Recordings** to browse the step timeline, replay a session, or export an HTML report. Replay runs the steps on every page, including the primary, but doesn't include navigations, so start the task at the flow's entry URL first.
5. **Stop.** Every page is torn down and its session data is wiped.

## Features

### Sync
- **Live action sync.** Clicks, input, change, select, scroll and submit events on the primary page are replayed on every synced page. Each page has its own sync toggle. If a page can't replay a step, it pauses and shows the error instead of falling silently out of sync.
- **Navigation follow.** Redirects, JS navigations and history moves on the primary page are mapped to each follower's own host, so every variant stays on the same step. Links that leave the primary's host don't move the followers, and reloading a follower brings it back in line.
- **Sync pace.** Continuous input and scroll mirror instantly or coalesce at a 100/250/500 ms interval that keeps only the latest value or position. Clicks and other discrete actions always replay immediately.

### Variants and devices
- **URL variant templates.** One template, with different variables and an optional path per page. Navigation follow maps paths between the rendered prefixes.
- **Bulk add and duplicate.** Paste one URL per line to append every row in one step, within the 10-page cap; generated pages inherit the preset resolution and take their host as name. Duplicate any row to clone its URL, variables and resolution, then edit only what differs.
- **Desktop and mobile layouts.** Per-page resolutions from 200 to 3840 px.
- **User-Agent presets.** iPhone, iPad, Android phone and tablet, Windows Chrome, Edge and Firefox, and macOS Safari, or paste a custom string.
- **Isolated or shared storage.** Each page gets its own ephemeral session by default. You can also share cookies and cache across pages.
- **Setup profiles.** Save the whole setup — pages, template, resolutions, UA and toggles — under a name, reload it later in one click. Profiles live in `Pictures/Peapod/profiles`.

### Evidence
- **Batch screenshots.** One click saves a PNG of every running page, including pages that aren't currently on screen.
- **Variant diff.** One click compares every page against the primary pixel by pixel, saving a red-highlight diff PNG per page plus a `diff.json` stats summary under `Pictures/Peapod/diffs`. Pages must share the primary's resolution; a per-channel tolerance absorbs anti-aliasing noise.
- **Operation recording.** When turned on, every operation goes into a replayable `script.jsonl`, and all pages are screenshotted once the task settles.
- **Recordings browser.** Lists past sessions with a step-by-step timeline and per-page thumbnails. You can replay a session on a running task or export a self-contained HTML report.

### Automation
- **MCP server.** A local [Model Context Protocol](https://modelcontextprotocol.io) endpoint, protected by a token, lets an AI agent start, navigate, operate, replay, screenshot, diff, diagnose and stop tasks without using the UI.
- **Acceptance diagnostics.** Failed navigations report stable error names (`NAME_NOT_RESOLVED`, `TIMEOUT`, …). An opt-in, MCP-only collector also reports the main document's HTTP status, the page text and console, network and error activity, so an agent can check actual outcomes rather than judge from screenshots.

### Everyday
- English and 简体中文 UI, light, dark or system theme, and UI scaling.

## Install

Prebuilt packages are not published yet, so for now Peapod is built from source. It needs Go 1.27.4 or newer, plus the system webview for your platform:

| Platform | Webview | Notes |
| --- | --- | --- |
| Windows 10/11 | WebView2 | Preinstalled on current Windows 10/11. This is the primary tested platform. |
| macOS | WKWebView | Part of the OS. |
| Linux | WebKitGTK | Install the WebKitGTK development package from your distro. |

```sh
git clone https://github.com/JiChao99/peapod.git
cd peapod
go build .          # build the executable
go tool mygo dev    # or run in development mode
go tool mygo build  # or package installers into build/
```

## Let an AI agent drive it (MCP)

When Peapod starts, it serves a Streamable-HTTP MCP endpoint on a random `127.0.0.1` port. A new bearer token is generated for each run, and both the endpoint and token are written to a discovery file that only your user can read:

| Platform | Discovery file |
| --- | --- |
| Windows | `%APPDATA%\Peapod\mcp.json` |
| macOS | `~/Library/Application Support/Peapod/mcp.json` |
| Linux | `~/.config/Peapod/mcp.json` |

Tools: `peapod_status`, `peapod_start`, `peapod_stop`, `peapod_navigate`, `peapod_reload`, `peapod_set_sync`, `peapod_send_operation`, `peapod_replay`, `peapod_screenshot`, `peapod_diff`, `peapod_diagnostics`.

Client configuration, the full tool reference, the recording format and an example agent workflow are in **[docs/mcp.md](docs/mcp.md)**. Set `PEAPOD_MCP_DISABLE=1` to turn the server off.

## Privacy and security

- Password and file inputs are **never** captured, recorded or replayed.
- Only `http`/`https` URLs are accepted, and URLs with embedded credentials are rejected.
- Sessions are ephemeral, and their data is wiped when the task stops.
- Popups and permission prompts (camera, microphone, notifications, …) are blocked. Each page's IPC is locked to its own origin.
- Screenshots and recordings stay on your machine, in `Pictures/Peapod/screenshots` and `Pictures/Peapod/recordings`. Recorded scripts do contain other typed text, so treat them as sensitive.
- The MCP server listens on loopback only, requires the token on every request and rejects cross-origin browser calls. Page content is untrusted data, and agents must not follow instructions found in it.

## Known limitations

- **User-Agent presets change only the UA string.** Pages still render in the platform webview, so engine-specific bugs (for example, Safari-only bugs on Windows) can't be reproduced this way.
- **Screenshots need a visible window.** The webview stops rendering while Peapod is minimized or fully covered, so captures time out until the window is visible again.
- **Navigation follow stays on the primary's host.** Followers don't follow links to other sites, by design.
- **Links that open a new window or tab are blocked**, since popups are disabled.
- **Windows gets the most testing.** macOS and Linux are supported. Linux builds and passes CI; macOS isn't covered by CI yet.

## FAQ

**How is this different from a responsive previewer?**
Tools like Responsively or Polypane show *one* URL at several screen sizes. Peapod is built for *several versions* of a flow — different locales, domains or environments — run together, with navigation mapped between them and a record of every step.

**Does it send any data anywhere?**
No. Peapod has no telemetry and makes no network calls of its own. Pages only contact the sites you open, and the MCP server can only be reached from your own machine.

## Contributing

Issues and pull requests are welcome. Before you open a PR, run:

```sh
go build ./...
go vet ./...
go test ./...
```

CI runs the same build, vet and test steps on Windows and Linux.

## Acknowledgements

- [MyGo](https://github.com/egoist/mygo) provides the native Go UI and packaging.
- [Lobe MyGo](https://github.com/lobehub/lobe-mygo) supplies the panel's components, theme and [Lucide](https://lucide.dev) icons.
- The MCP server's shape (a loopback Streamable-HTTP endpoint with a per-run token) follows [pinky](https://github.com/DIYgod/pinky).

## License

[MIT](LICENSE) © 2026 Alec.Ji
