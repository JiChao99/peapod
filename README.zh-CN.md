<p align="center">
  <img src="assets/icon.svg" alt="Peapod logo" width="120">
</p>

<h1 align="center">Peapod</h1>

<p align="center">多页面同步浏览</p>

<p align="center">
  <b>开源桌面应用，让多个页面同步浏览。</b><br>
  一次操作，页页跟随。
</p>

[![CI](https://github.com/jichao99/peapod/actions/workflows/ci.yml/badge.svg)](https://github.com/jichao99/peapod/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[English](README.md) | 简体中文

Peapod 在一个窗口中打开 2–10 个页面。你操作**主页面**——点击、输入、滚动、表单提交——其余页面按顺序实时回放同样的操作。适用于对比环境（dev / staging / prod）、演练多账号流程、检查响应式布局、批量截图。

基于 [MyGo](https://github.com/egoist/mygo) 构建：原生 Go UI、单二进制、不内置浏览器引擎。

## 功能

- **实时动作同步** —— 在主页面捕获 click / input / change / select / scroll / submit，并回放到每一个同步目标。每页独立同步开关；回放失败的目标会暂停并报错，而不是静默失步。
- **桌面与移动模式** —— 单个大页面，或所有页面并排并使用移动端 User-Agent。每页分辨率 200–3840 px 可调。
- **隔离或共享存储** —— 默认每页一个临时会话；可选跨页共享 cookies 与缓存。
- **批量截图** —— 一键将每个运行中的页面截为 PNG，无论当前屏幕上显示哪一页。
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
2. 选择桌面或移动模式，设置每页分辨率、是否共享存储。
3. 启动任务并操作主页面——其余页面跟随。点击页面卡片可将其置前。
4. 停止任务即销毁所有页面并清除其数据。

## 许可证

[MIT](LICENSE)