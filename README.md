<p align="center">
  <img src="assets/icon.svg" alt="Peapod logo" width="120">
</p>

<h1 align="center">Peapod</h1>

<p align="center">Multi-page Browser Sync</p>

<p align="center">
  <b>Open-source desktop app for synchronized multi-page browsing.</b><br>
  Operate once. Every page follows.
</p>

[![CI](https://github.com/jichao99/peapod/actions/workflows/ci.yml/badge.svg)](https://github.com/jichao99/peapod/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

English | [简体中文](README.zh-CN.md)

Peapod opens 2–10 pages in one window. You operate the **primary** page — clicks, typing, scrolling, form submits — and every other page replays the same operations live, in order. Useful for comparing environments (dev / staging / prod), exercising multi-account flows, checking responsive layouts, and capturing batch screenshots.

Built with [MyGo](https://github.com/egoist/mygo): native Go UI, single binary, no bundled browser engine.

## Features

- **Live action sync** — click / input / change / select / scroll / submit captured on the primary page and replayed on every synced target. Per-page sync toggle; a target that fails to replay pauses and reports the error instead of desyncing silently.
- **Navigation follow** — when the primary page navigates without a captured action (JS or server redirect, history navigation), every synced follower loads the same path on its own host, so all environments stay on the same step. Links outside the primary's host leave followers where they are; reloading a follower re-aligns it with the primary.
- **URL variant templates** — optionally define one template such as `https://{host}/{lang}` and give each page its own variables (`host=staging.example.com, lang=de`) plus an optional path. Variants open side by side and navigation follow maps between the rendered prefixes.
- **Desktop & mobile modes** — one large page, or all pages side by side with a mobile user agent. Per-page resolutions from 200 to 3840 px.
- **Isolated or shared storage** — each page gets an ephemeral session by default; optionally share cookies & cache across pages.
- **Batch screenshots** — capture every running page to PNG in one click, no matter which page is on screen.
- **Bilingual UI** — 中文 / English, light / dark / system theme, UI scaling.

## Privacy & security

- Password and file inputs are **never** captured or replayed.
- URLs with embedded credentials are rejected; only http/https are allowed.
- All sessions are ephemeral and their data is wiped when the task stops.
- Popups and permission requests (camera, microphone, notifications, …) are blocked; each page's IPC is locked to its own origin.

## Platforms

Windows (WebView2), macOS (WKWebView), Linux (WebKitGTK). Windows is the primary tested platform.

## Build from source

Requires Go 1.27.4 or newer.

```sh
go build .          # build the executable
go tool mygo dev    # run in development mode
go tool mygo build  # package installers into build/
```
## Usage

1. Enter the primary page URL and add up to 10 pages (a non-primary page with an empty URL inherits the primary).
2. Pick desktop or mobile mode, set per-page resolutions, choose whether storage is shared.
3. Start the task and operate the primary page — the others follow. Click a page card to bring it to the front.
4. Stop the task to tear down every page and wipe its data.

## License

[MIT](LICENSE)