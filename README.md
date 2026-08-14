# Kai

常驻系统托盘的翻译工具，支持划词、截图与输入翻译，并内置 OCR。

[English](./README_EN.md) | [中文](./README.md)

## 1. 产品功能

| 功能 | 默认快捷键 | 默认启用 | 说明 |
|------|------------|----------|------|
| 输入翻译 | mac: `⌥+A` / win: `Alt+A` | 是 | 唤起翻译主窗口，手动输入文本翻译 |
| 截图翻译 | mac: `⌥+S` / win: `Alt+S` | 否（需在设置中手动开启） | 框选区域 → 识别文字 → 翻译，结果在截图窗口展示 |

> 全局快捷键仅上述两类。另有复制键（`⌘+C` / `Ctrl+C`），用于把选中文本送入剪贴板供翻译读取。

- **翻译引擎**：内置 DeepL、Google、OpenAI、百度、腾讯、有道，以及 macOS 系统翻译。Google 与系统翻译开箱即用；其余需自行填入 API Key
- **OCR**：macOS 使用系统离线识别（无需安装）；也可选装本机 tesseract。区域截图触发仅 macOS 支持
- **界面语言**：内置中文 / 英文，可跟随系统或在设置中切换
- **自动更新**：启动静默检查 + 托盘菜单「检查更新」；更新源默认按界面语言选择（中文走 CNB，英文走 GitHub），也可手动指定；开启预发布通道可收到每日 nightly 版本

## 2. 平台说明
- **macOS**：完整能力（系统翻译 / 系统 OCR / 复制键 / 自动更新）
- **Windows**：在线翻译与 OCR（需本机装 tesseract）可用，复制键可用，自动更新可用；系统翻译不支持
- 当前仅支持 macOS 与 Windows，不支持 Linux

## 3. 仓库与发布

### 仓库
| 平台 | 地址 |
|------|------|
| CNB | https://cnb.cool/dtapp/kai |
| GitHub | https://github.com/dtapps/kai |
| Gitea | https://gitea.com/dtapps/kai |
| GitLab | https://gitlab.com/dtapps/kai |
| Gitee | https://gitee.com/dtapps/kai |
| GitCode | https://gitcode.com/dtapp/kai |

### 发布
- **正式发布**：手动触发 Release 工作流并输入版本号，macOS / Windows 双平台构建并发布
- **每日构建（Nightly）**：每日自动构建 `nightly` 预发布版本
- 构建产物从 GitHub Release 下载，再发布到 CNB

## 4. 许可证
详见仓库 `LICENSE` 文件。

## 5. 致谢
设计灵感参考 [Bob](https://github.com/ripperhe/Bob) 与 [Easydict](https://github.com/tisfeng/Easydict)。
