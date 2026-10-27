# Kai

[English](README_EN.md) | 中文

常驻系统托盘的翻译工具，支持输入翻译、截图翻译与复制键取词翻译，并内置 OCR。

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/dtapps/kai?style=flat-square)](https://github.com/dtapps/kai/releases) [![Downloads](https://img.shields.io/github/downloads/dtapps/kai/total?style=flat-square)](https://github.com/dtapps/kai/releases) [![Stars](https://img.shields.io/github/stars/dtapps/kai?style=flat-square)](https://github.com/dtapps/kai/stargazers) [![Forks](https://img.shields.io/github/forks/dtapps/kai?style=flat-square)](https://github.com/dtapps/kai/network/members) [![License](https://img.shields.io/github/license/dtapps/kai?style=flat-square)](LICENSE) [![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows-007EC6?style=flat-square)](https://github.com/dtapps/kai) [![Build](https://img.shields.io/github/actions/workflow/status/dtapps/kai/push.yml?style=flat-square)](https://github.com/dtapps/kai/actions/workflows/push.yml) [![Codecov](https://img.shields.io/codecov/c/github/dtapps/kai?style=flat-square&logo=codecov&label=codecov)](https://codecov.io/gh/dtapps/kai) [![Coveralls](https://img.shields.io/coverallsCoverage/github/dtapps/kai?style=flat-square&logo=coveralls&label=coveralls)](https://coveralls.io/github/dtapps/kai?branch=master) ![下载请求数](https://img.shields.io/endpoint?label=下载请求数&url=https%3A%2F%2Fdl-stats.dtapp.top%2Fshields.json%3Fplatform%3Dgithub.com%26repository%3D%2Fdtapps%2Fkai)

[![CNB Release](https://cnb.cool/dtapp/kai/-/badge/release)](https://cnb.cool/dtapp/kai/-/badge/release.link) [![CNB Stars](https://cnb.cool/dtapp/kai/-/badge/star)](https://cnb.cool/dtapp/kai) [![CNB Forks](https://cnb.cool/dtapp/kai/-/badge/fork)](https://cnb.cool/dtapp/kai) [![CNB Build](https://cnb.cool/dtapp/kai/-/badge/git/latest/ci/status/push)](https://cnb.cool/dtapp/kai) [![CNB Coverage](https://cnb.cool/dtapp/kai/-/badge/git/latest/testing/unit/coverage?branch=master)](https://cnb.cool/dtapp/kai) [![CNB Coverage PR](https://cnb.cool/dtapp/kai/-/badge/git/latest/testing/unit/coverage-pr?branch=master)](https://cnb.cool/dtapp/kai) ![下载请求数](https://img.shields.io/endpoint?label=下载请求数&url=https%3A%2F%2Fdl-stats.dtapp.net%2Fshields.json%3Fplatform%3Dcnb.cool%26repository%3D%2Fdtapp%2Fkai)

[![CNB Vulnerability Full](https://cnb.cool/dtapp/kai/-/badge/security/vulnerability/full/latest?branch=master)](https://cnb.cool/dtapp/kai/-/badge/security/vulnerability/full/latest.link?branch=master) [![CNB Vulnerability Diff](https://cnb.cool/dtapp/kai/-/badge/security/vulnerability/diff/latest?branch=master)](https://cnb.cool/dtapp/kai/-/badge/security/vulnerability/diff/latest.link?branch=master) [![CNB License Full](https://cnb.cool/dtapp/kai/-/badge/security/license/full/latest?branch=master)](https://cnb.cool/dtapp/kai/-/badge/security/license/full/latest.link?branch=master) [![CNB License Diff](https://cnb.cool/dtapp/kai/-/badge/security/license/diff/latest?branch=master)](https://cnb.cool/dtapp/kai/-/badge/security/license/diff/latest.link?branch=master) [![CNB SBOM Full](https://cnb.cool/dtapp/kai/-/badge/security/sbom/full/latest?branch=master)](https://cnb.cool/dtapp/kai/-/badge/security/sbom/full/latest.link?branch=master) [![CNB SBOM Diff](https://cnb.cool/dtapp/kai/-/badge/security/sbom/diff/latest?branch=master)](https://cnb.cool/dtapp/kai/-/badge/security/sbom/diff/latest.link?branch=master)

</div>

## 1. 产品功能

| 功能     | 默认快捷键                | 默认启用                 | 说明                                                     |
|----------|---------------------------|--------------------------|----------------------------------------------------------|
| 输入翻译 | mac: `⌥+A` / win: `Alt+A` | 是                       | 唤起翻译主窗口，手动输入文本翻译                         |
| 截图翻译 | mac: `⌥+S`                | 否（需在设置中手动开启） | 框选区域 → 识别文字 → 翻译，结果在截图窗口展示；仅 macOS |

> 全局快捷键仅上述两类。另有复制键（默认 mac `⌘+C` / win `Ctrl+C`），由程序模拟按下，把当前选区复制后填入输入框翻译；可在「设置 → 快捷键」中改键或关闭。

- **翻译引擎**：内置 Google、DeepL、OpenAI、Anthropic、Gemini、百度、腾讯、有道，以及 macOS 系统翻译。Google 与 macOS 系统翻译开箱即用；其余需自行填入 API Key
- **OCR**：macOS 使用系统离线识别（Vision，无需安装）；也可选装本机 tesseract。OCR 目前只随截图翻译入口在 macOS 使用，Windows 没有截图入口
- **自动读取剪贴板**：开启后按输入翻译快捷键直接读取剪贴板内容并翻译，不模拟复制键；开启期间复制键自动停用，避免双重触发
- **界面语言**：内置中文 / 英文，可跟随系统或在设置中切换
- **自动更新**：启动静默检查 + 托盘菜单「检查更新」；更新源默认按界面语言选择（中文走 CNB，英文走 GitHub），也可手动指定；预发布通道默认开启，可收到预发布版本

## 2. 匿名使用统计（PostHog）

内置可选的匿名使用统计（**默认关闭**，需在「设置 → 通用」手动开启），用于了解功能使用情况以改进产品。开启后**仅上报下列匿名数据，不含任何个人信息**（不采集文本内容、翻译结果、API Key、账号等）：

- **匿名设备标识**：由机器指纹经 HMAC 单向派生的不可逆随机 ID，用于区分独立设备、统计留存；该 ID 不保存在应用数据里，删除应用数据不会重置。
- **地理位置（粗粒度）**：开启后 PostHog 按上报请求的来源 IP 做 GeoIP 归属（国家 / 地区 / 城市），不含精确坐标或运营商详情，仅用于地域分布统计。
- **应用标识（app_name）**：固定为 `kai`，用于在同一 PostHog 实例区分不同应用 / 构建。

| 事件                   | 触发时机               | 附带属性（均非敏感）                            |
|------------------------|------------------------|-------------------------------------------------|
| `app_installed`        | 首次安装启动（仅一次） | 版本、渠道                                      |
| `app_started`          | 每次启动               | 版本、操作系统、界面语言、渠道                  |
| `translate_input`      | 输入翻译               | 使用的引擎、源/目标语言、文本长度分桶、触发方式 |
| `translate_screenshot` | 截图翻译               | 使用的引擎、OCR 提供方、是否重试                |
| `settings_opened`      | 打开设置页             | 所在标签页                                      |
| `feature_toggled`      | 切换功能开关           | 功能名、开/关                                   |
| `error_occurred`       | 翻译/OCR 失败          | 错误类型、涉及的引擎/OCR 提供方                 |

说明：

- 文本长度仅上报**分桶**（`0` / `1-20` / `21-100` / `101-500` / `501-2000` / `2000+`），不上报实际长度或内容。
- 关闭开关后立即停止上报并断开连接；开发构建、未配置统计 Key 的构建均不会上报。
- 数据经 PostHog（`https://us.i.posthog.com`）上报。

## 3. 平台说明

- **macOS**：完整能力（系统翻译 / 系统 OCR / 截图翻译 / 复制键 / 自动更新）
- **Windows**：输入翻译、复制键翻译、在线翻译引擎与自动更新可用；不支持系统翻译与截图翻译（区域截图仅 macOS）
- 当前仅支持 macOS 与 Windows，不支持 Linux

## 4. 仓库与发布

### 仓库

| 平台    | 地址                          |
|---------|-------------------------------|
| CNB     | https://cnb.cool/dtapp/kai    |
| GitHub  | https://github.com/dtapps/kai |
| Gitea   | https://gitea.com/dtapps/kai  |
| GitLab  | https://gitlab.com/dtapps/kai |
| Gitee   | https://gitee.com/dtapps/kai  |
| GitCode | https://gitcode.com/dtapp/kai |

> Gitea / GitCode 仅作代码镜像，不发布 Release。

### 发布

- **正式发布**：手动触发 Release 工作流并输入版本号，构建 macOS / Windows 双平台（arm64 + amd64）并发布 GitHub Release
- **预发布**：同一工作流勾选 `prerelease`（CNB 侧传 `pre_release=true`）发布预发布版本，不再有每日 Nightly 定时构建
- 构建产物从 GitHub Release 同步到 CNB / GitLab / Gitee

## 5. 文档

开发与维护文档见 [docs/README.md](docs/README.md)：文档索引、开发/配置/发布手册、CI 专题与 ADR。

仓库根目录另有三个协作文件：[CONTRIBUTING.md](CONTRIBUTING.md)（贡献指南）、[SECURITY.md](SECURITY.md)（安全策略）、[CHANGELOG.md](CHANGELOG.md)（版本变更记录）。

## 6. 许可证

详见仓库 `LICENSE` 文件。

## 7. 已知问题

> 以下为当前版本（Wails v3.0.0）的已知限制，多为上游框架或 macOS 平台行为所致，非本应用逻辑缺陷。

- **macOS 快捷键首次唤起截图窗口可能被遮挡**：在其它应用处于前台时，通过快捷键（`⌥+S`）第一次唤起截图翻译窗口，偶发被原前台应用窗口盖住。再次唤起（窗口已存在于事件循环中）则正常。原因：Kai 为无 Dock 图标的辅助型（accessory）应用，在 macOS（尤其 Tahoe / macOS 27）下从后台抢回前台焦点的能力受系统限制，纯 Wails 方案下首次存在竞态。托盘菜单点击唤起不受影响（系统原生点击天然带前台转移）。**临时规避**：若窗口被挡，可先点击一下 Kai 托盘图标或再次触发快捷键。

## 8. 致谢

<a href="https://wails.io">
  <img src="https://wails.io/img/wails-button-light.svg" width="90" height="24" alt="Built with Wails">
</a>

设计灵感参考 [Bob](https://github.com/ripperhe/Bob) 与 [Easydict](https://github.com/tisfeng/Easydict)。
