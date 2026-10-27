# 用户手册

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

面向安装和使用 Kai 的人。产品能力概览见根目录 `README.md`；出问题先看 `troubleshooting.md`。

## 1. 安装

### 1.1 下载

| 平台             | 文件                                      | 说明               |
|------------------|-------------------------------------------|--------------------|
| macOS Apple 芯片 | `Kai-<version>-darwin-arm64.zip`          | 解压得到 `Kai.app` |
| macOS Intel      | `Kai-<version>-darwin-amd64.zip`          | 解压得到 `Kai.app` |
| Windows x64      | `Kai-<version>-windows-amd64-install.exe` | NSIS 安装包        |
| Windows ARM64    | `Kai-<version>-windows-arm64-install.exe` | NSIS 安装包        |

发布页：

- GitHub：`https://github.com/dtapps/kai/releases`
- CNB：`https://cnb.cool/dtapp/kai/-/releases`

`updater-` 开头的压缩包是自动更新专用文件，不要手动下载安装。Release 工作流不打 `.dmg`；本地需要 dmg 时用 `make darwin-package-dmg` 自己构建。

### 1.2 macOS

1. 解压 zip，把 `Kai.app` 拖进「应用程序」。
2. 首次打开。若提示「无法验证开发者」，在「系统设置 → 隐私与安全性」点「仍要打开」，或右键 App → 打开。
3. 按 §2 授予辅助功能与屏幕录制权限。

Kai 是辅助型应用（Accessory）：启动后没有 Dock 图标，只在菜单栏显示托盘图标。

### 1.3 Windows

1. 运行 `Kai-<version>-windows-<arch>-install.exe`。
2. 若出现 SmartScreen 提示，点「更多信息 → 仍要运行」（未配置 Authenticode 签名的构建会这样）。
3. 需要 WebView2 Runtime。Windows 11 通常已内置；缺失时安装微软官方 Runtime。

Windows 版支持输入翻译、复制键取词、在线引擎和自动更新；**没有截图翻译**。

## 2. 首次启动与权限

启动后会显示设置窗口，菜单栏/托盘出现 Kai 图标。点击托盘图标可切换设置窗口显隐。

### 2.1 macOS 辅助功能

复制键取词需要「辅助功能」权限（模拟 `⌘+C`）。路径：

```text
系统设置 → 隐私与安全性 → 辅助功能 → 勾选 Kai
```

Kai 设置页「快捷键」顶部也有授权状态卡片，可直接点「打开设置」。

### 2.2 macOS 屏幕录制

截图翻译需要「屏幕录制」权限（调用 `screencapture`）。路径：

```text
系统设置 → 隐私与安全性 → 屏幕录制 → 勾选 Kai
```

授权后通常需要重启 Kai 才生效。

## 3. 三种取词方式

### 3.1 输入翻译

- 默认快捷键：macOS `⌥+A`，Windows `Alt+A`。
- 唤起翻译窗口，手动输入或粘贴文本，按翻译。
- 翻译窗口会把结果按引擎并排展示，每个引擎完成一条就追加一条，不互相阻塞。

### 3.2 复制键取词

- 默认复制键：macOS `⌘+C`，Windows `Ctrl+C`。
- 按下输入翻译快捷键时，Kai 会：备份剪贴板 → 清空 → 模拟复制当前选区 → 读取文本 → 还原剪贴板 → 把文本填进翻译窗口。
- 如果自定义复制键没有取到内容，可在设置里打开「回退系统复制键」，让 Kai 再用系统默认复制键试一次。
- 复制键不生效，优先检查辅助功能权限（macOS）。

### 3.3 自动读取剪贴板

在翻译窗口打开「自动读取剪贴板」后：

- 你先自己复制文本，再按输入翻译快捷键，Kai 直接读剪贴板并翻译。
- 不再模拟复制键；设置页的复制键开关会被自动禁用。
- 关闭该开关后，复制键恢复到开启前的状态（通过 `copy_key_snapshot` 快照恢复）。

## 4. 截图翻译（仅 macOS）

1. 在「设置 → 快捷键」启用截图翻译，默认 `⌥+S`。
2. 按快捷键，屏幕进入框选；拖拽选择区域。
3. Kai 先显示截图窗口，再做 OCR，然后逐引擎翻译，结果增量出现。
4. 在截图窗口可以：
   - 「重新截图」：重新框选，不复用上次结果；
   - 改源/目标语言：复用最近一次 OCR 原文重新翻译，不重新截图；
   - 关闭窗口：只隐藏，不销毁。

OCR 引擎在「设置 → 引擎」里选。同一时刻只启用一个 OCR 引擎。

### 4.1 系统 OCR（默认，macOS）

- 引擎标识 `vision`，走 macOS Vision 框架，离线、免安装。
- 可配置：语言校正（`correct_text`）、超时秒数（`timeout_sec`，默认 60）、失败重试次数（`retry_count`，默认 2）。
- 语言校正关闭会更快，但准确率略降。

### 4.2 tesseract（可选）

- 引擎标识 `tesseract`，调用本机 `tesseract` 可执行文件。
- 安装：

```bash
brew install tesseract
brew install tesseract-lang   # 需要中文等语言包时
```

- 如果 Kai 找不到，可在引擎配置的「二进制路径」里填绝对路径，例如 `/opt/homebrew/bin/tesseract`。
- 识别语言用 `+` 拼接，例如 `chi_sim+eng`；候选项来自 `engine.OcrLangsOptions()`。

## 5. 翻译引擎

在「设置 → 引擎」里添加或启用。每个引擎类型只能配置一份；系统内置引擎（`apple` / `vision`）不可删除，但可以停用。

| 引擎        | 是否需要 Key | 配置项                                                      | 申请入口（自行搜索最新地址） |
|-------------|--------------|-------------------------------------------------------------|------------------------------|
| `google`    | 否           | 可选 `endpoint`，留空用公开端点                             | —                            |
| `apple`     | 否           | 无（macOS 系统翻译）                                        | —                            |
| `deepl`     | 是           | `api_key`、`endpoint`（免费版默认，Pro 改 `api.deepl.com`） | DeepL API                    |
| `openai`    | 是           | `api_key`、`endpoint`（Base URL）、`model`、`timeout_sec`   | OpenAI Platform              |
| `anthropic` | 是           | `api_key`、`endpoint`、`model`、`timeout_sec`               | Anthropic Console            |
| `gemini`    | 是           | `api_key`、`endpoint`、`model`、`timeout_sec`               | Google AI Studio             |
| `baidu`     | 是           | `api_key`=AppID、`secret`=密钥                              | 百度翻译开放平台             |
| `tencent`   | 是           | `api_key`=SecretId、`secret`=SecretKey                      | 腾讯云机器翻译 TMT           |
| `youdao`    | 是           | `api_key`=appKey、`secret`=appSecret                        | 有道智云                     |

保存时只校验必填字段；启用时也会再校验一次，缺 Key 会被拒绝。具体 schema 与默认值见 `engines.md`。

## 6. 语言、主题、统计

- **界面语言**：设置 → 通用，`auto` / 中文 / English。`auto` 时前端按系统语言解析；后端部分文案（托盘、通知）目前可能仍按中文回退，见 `configuration.md` §2.3。
- **主题**：`auto` / 浅色 / 深色。`auto` 跟随系统外观。
- **匿名统计**：默认关闭。开启后只上报功能使用情况和匿名设备 ID，不上报文本、译文、API Key；关闭后立即停止上报。

## 7. 自动更新

- 启动后 1–3 分钟随机静默检查一次；托盘菜单「检查更新」可手动触发。
- 托盘菜单「预发布版」控制是否接收预发布版本，默认开启；切换后建议重启应用，让更新器重新读取通道。
- 更新源默认按界面语言选择：中文 → CNB，英文 → GitHub；也可在 `settings.json` 的 `updater.source` 强制指定（无 UI，改后需重启）。
- 检查到更新会打开内置更新窗口，确认后下载；下载完成重启生效。
- 更新只使用 `updater-` 前缀的升级包，并校验 `SHA256SUMS`。

## 8. 数据与日志

| 内容           | 路径                                     |
|----------------|------------------------------------------|
| 界面配置       | `~/.kai/settings.json`                   |
| 引擎配置与凭据 | `~/.kai/data/config.db`                  |
| 翻译历史       | `~/.kai/data/history.db`                 |
| HTTP 请求日志  | `~/.kai/data/httplog.db`                 |
| 运行日志       | `~/.kai/logs/kai.log`                    |
| 前端日志       | `~/.kai/logs/frontend.log`               |
| Swift 桥日志   | `~/.kai/logs/kai-bridge.log`（仅 macOS） |

开发构建使用 `~/.kai.dev/`。日志按天归档为 `kai-YYYY-MM-DD.log`，保留 30 天并压缩（默认）。

重置：

```bash
# 退出 Kai 后
rm -rf ~/.kai
```

这会删除配置、历史、日志和匿名统计标记；下次启动重新初始化。只重置引擎/历史可只删 `~/.kai/data/` 下的对应 db。

## 9. 常见问题

- **快捷键没反应**：macOS 检查辅助功能/屏幕录制；确认快捷键已启用、没有被其它应用占用。
- **截图窗口第一次被挡住**：这是 macOS 辅助型应用从后台抢焦点的限制。再按一次快捷键或点一下托盘图标即可。详见 `README.md` §7 与 `troubleshooting.md`。
- **系统翻译/系统 OCR 不可用**：确认是 macOS；开发构建可能需要 `make swift-build`。
- **API Key 明明填了却报缺失**：把 `config.db` 拷到别的机器会导致凭据无法解密；在本机重新填写。
- **更新检查失败**：CNB 源匿名请求可能 401；在 `settings.json` 把 `updater.source` 改成 `github`，或检查代理/DNS。

更细的排查命令见 `troubleshooting.md`。
