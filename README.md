# Kai

常驻系统托盘的翻译工具，支持划词、截图与输入翻译，并内置 OCR。

[English](./README_EN.md) | [中文](./README.md)

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/dtapps/kai?style=flat-square)](https://github.com/dtapps/kai/releases)
[![Downloads](https://img.shields.io/github/downloads/dtapps/kai/total?style=flat-square)](https://github.com/dtapps/kai/releases)
[![Stars](https://img.shields.io/github/stars/dtapps/kai?style=flat-square)](https://github.com/dtapps/kai/stargazers)
[![License](https://img.shields.io/github/license/dtapps/kai?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows-007EC6?style=flat-square)](https://github.com/dtapps/kai)
[![Build](https://img.shields.io/github/actions/workflow/status/dtapps/kai/push.yml?style=flat-square)](https://github.com/dtapps/kai/actions/workflows/push.yml)

[![CNB Release](https://cnb.cool/dtapp/kai/-/badge/release)](https://cnb.cool/dtapp/kai/-/badge/release.link)
[![CNB Stars](https://cnb.cool/dtapp/kai/-/badge/star)](https://cnb.cool/dtapp/kai)
[![CNB Forks](https://cnb.cool/dtapp/kai/-/badge/fork)](https://cnb.cool/dtapp/kai)
[![CNB Build](https://cnb.cool/dtapp/kai/-/badge/git/latest/ci/status/push)](https://cnb.cool/dtapp/kai)

</div>

## 1. 产品功能

| 功能     | 默认快捷键                | 默认启用                 | 说明                                           |
| -------- | ------------------------- | ------------------------ | ---------------------------------------------- |
| 输入翻译 | mac: `⌥+A` / win: `Alt+A` | 是                       | 唤起翻译主窗口，手动输入文本翻译               |
| 截图翻译 | mac: `⌥+S` / win: `Alt+S` | 否（需在设置中手动开启） | 框选区域 → 识别文字 → 翻译，结果在截图窗口展示 |

> 全局快捷键仅上述两类。另有复制键（`⌘+C` / `Ctrl+C`），用于把选中文本送入剪贴板供翻译读取。

- **翻译引擎**：内置 DeepL、Google、OpenAI、百度、腾讯、有道，以及 macOS 系统翻译。Google 与系统翻译开箱即用；其余需自行填入 API Key
- **OCR**：macOS 使用系统离线识别（无需安装）；也可选装本机 tesseract。区域截图触发仅 macOS 支持
- **界面语言**：内置中文 / 英文，可跟随系统或在设置中切换
- **自动更新**：启动静默检查 + 托盘菜单「检查更新」；更新源默认按界面语言选择（中文走 CNB，英文走 GitHub），也可手动指定；开启预发布通道可收到每日 nightly 版本

## 2. 匿名使用统计（PostHog）

内置可选的匿名使用统计（**默认关闭**，需在「设置 → 通用」手动开启），用于了解功能使用情况以改进产品。开启后**仅上报下列匿名数据，不含任何个人信息**（不采集文本内容、翻译结果、API Key、账号等）：

- **匿名设备标识**：由机器指纹经 HMAC 单向派生的不可逆随机 ID，用于区分独立设备、统计留存；删除应用数据即可重置。
- **地理位置（粗粒度）**：开启后 PostHog 按上报请求的来源 IP 做 GeoIP 归属（国家 / 地区 / 城市），不含精确坐标或运营商详情，仅用于地域分布统计。
- **应用标识（app_name）**：固定为 `kai`，用于在同一 PostHog 实例区分不同应用 / 构建。

| 事件                   | 触发时机               | 附带属性（均非敏感）                            |
| ---------------------- | ---------------------- | ----------------------------------------------- |
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

- **macOS**：完整能力（系统翻译 / 系统 OCR / 复制键 / 自动更新）
- **Windows**：在线翻译与 OCR（需本机装 tesseract）可用，复制键可用，自动更新可用；系统翻译不支持
- 当前仅支持 macOS 与 Windows，不支持 Linux

## 4. 仓库与发布

### 仓库

| 平台    | 地址                          |
| ------- | ----------------------------- |
| CNB     | https://cnb.cool/dtapp/kai    |
| GitHub  | https://github.com/dtapps/kai |
| Gitea   | https://gitea.com/dtapps/kai  |
| GitLab  | https://gitlab.com/dtapps/kai |
| Gitee   | https://gitee.com/dtapps/kai  |
| GitCode | https://gitcode.com/dtapp/kai |

### 发布

- **正式发布**：手动触发 Release 工作流并输入版本号，macOS / Windows 双平台构建并发布
- **每日构建（Nightly）**：每日自动构建 `nightly` 预发布版本
- 构建产物从 GitHub Release 下载，再发布到 CNB

## 5. 许可证

详见仓库 `LICENSE` 文件。

## 6. 已知问题

> 以下为当前版本（Wails v3 beta.9）的已知限制，多为上游框架或 macOS 平台行为所致，非本应用逻辑缺陷。

- **macOS 快捷键首次唤起截图窗口可能被遮挡**：在其它应用处于前台时，通过快捷键（`⌥+S`）第一次唤起截图翻译窗口，偶发被原前台应用窗口盖住。再次唤起（窗口已存在于事件循环中）则正常。原因：Kai 为无 Dock 图标的辅助型（accessory）应用，在 macOS（尤其 Tahoe / macOS 27）下从后台抢回前台焦点的能力受系统限制，纯 Wails 方案下首次存在竞态。托盘菜单点击唤起不受影响（系统原生点击天然带前台转移）。**临时规避**：若窗口被挡，可先点击一下 Kai 托盘图标或再次触发快捷键。
- **自定义标题栏需点击才能激活**：macOS 上自绘标题栏（frameless + 透明标题栏，用于跟随应用内浅/深主题）在窗口刚唤起时，红绿灯（关闭/最小化/全屏）偶尔需多点击一次才响应。这是 accessory 应用窗口激活时机的平台行为，激活后交互正常。

## 7. 致谢

设计灵感参考 [Bob](https://github.com/ripperhe/Bob) 与 [Easydict](https://github.com/tisfeng/Easydict)。
