# 排障手册

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

按现象查。先看 §1 收集信息，再跳到对应小节。安装与使用见 `user-guide.md`；配置字段见 `configuration.md`。

## 1. 先收集这些

```bash
# 版本（托盘菜单第一项也会显示）
defaults read /Applications/Kai.app/Contents/Info.plist CFBundleShortVersionString 2>/dev/null || true

# 日志（正式版）
tail -n 200 ~/.kai/logs/kai.log
tail -n 100 ~/.kai/logs/frontend.log
tail -n 100 ~/.kai/logs/kai-bridge.log   # 仅 macOS

# 只看错误
grep -iE 'error|warn|panic|failed' ~/.kai/logs/kai.log | tail -n 50

# 引擎配置（不显示 key 明文）
sqlite3 ~/.kai/data/config.db 'select id,engine,enabled,endpoint from engines order by id;'

# HTTP 请求（可能含敏感内容，贴出来前先删）
sqlite3 ~/.kai/data/httplog.db 'select id,method,url,status_code,is_error,created_at from http_log order by id desc limit 20;'
```

开发构建的目录是 `~/.kai.dev/`。如果日志目录不存在，先确认应用真的启动过（`ls -la ~/.kai`）。

## 2. 启动与窗口

### 2.1 启动后没有窗口 / 只有托盘图标

- Kai 是 Accessory 应用，没有 Dock 图标是正常的。
- 点击托盘图标可切换设置窗口；托盘菜单 →「设置」也能打开。
- 第二次启动不会开新实例：单实例锁会把设置窗口唤到前台（`main.go` 的 `SingleInstance`，UniqueID `cnb.cool.dtapp.kai`）。

### 2.2 macOS 首次用快捷键唤起截图窗口被挡住

- 现象：`⌥+S` 第一次唤起，截图窗口被原前台应用盖住；再按一次或点托盘图标恢复。
- 原因：accessory 应用从后台抢焦点受 macOS 限制，纯 Wails 方案下首次存在竞态。
- 处理：再触发一次；或先点托盘图标。不是截图失败，OCR/翻译通常仍在后台完成。

### 2.3 Windows 窗口崩溃 / `80010108`

- 先确认 WebView2 Runtime 已安装。
- 当前已注入 `--disable-gpu` / `--disable-gpu-compositing`，并把窗口改为创建后 `Hide()`；如果仍复现，收集：
  - `~/.kai/logs/kai.log`
  - Windows 事件查看器 → Windows 日志 → 应用程序，筛选 `Kai.exe`
- 这是上游 WebView2/GPU 竞态，不是业务逻辑错误；不要先删数据库。

## 3. 快捷键与取词

### 3.1 快捷键完全没反应

1. 设置 → 快捷键，确认对应项「已启用」且按键不为空。
2. macOS 检查辅助功能权限（复制键需要）。
3. 确认按键没有被其它应用占用；换一个组合测试。
4. 手改过 `settings.json` 的 `hotkeys`：热重载不会重新注册快捷键，重启应用。

### 3.2 复制键取到空 / 有时成功有时失败

- macOS：辅助功能未授权，或目标应用不支持 `⌘+C`。
- Windows：`SendInput` 可能被 foreground lock 节流；代码已用 `AttachThreadInput` 附着前台线程，但仍可能受目标应用影响。可在设置里开启「回退系统复制键」。
- 自动读取剪贴板开启时，复制键会被自动停用；这是设计行为，不是故障。
- 取词会备份/还原剪贴板；如果还原失败，日志会有 `copykey_restore_clipboard_failed`。

### 3.3 剪贴板内容被改了

- 复制键流程是「备份 → 清空 → 模拟复制 → 读文本 → 清空 → 写回备份」。正常情况下原内容会还原。
- 如果应用在模拟复制期间被系统杀死，可能来不及还原；重新复制一次即可。
- 自动读取剪贴板模式不改剪贴板。

## 4. 翻译引擎

### 4.1 启用引擎时报「缺少字段」

- 该引擎有 `Required=true` 的字段为空。看设置页对应字段，或查 schema：

```bash
# 开发态可直接跑测试/看代码
grep -n '"deepl"\|"openai"\|"baidu"' -A30 internal/engine/engine.go
```

### 4.2 翻译报鉴权错误 / 403 / 401

- 检查 Key、端点、账号额度。
- DeepL：免费 Key 必须用 `api-free.deepl.com`；Pro Key 用 `api.deepl.com`。
- OpenAI/Anthropic/Gemini：`endpoint` 是 Base URL，不要填完整的 `/chat/completions` 或 `/v1/messages`；模型名要存在。
- 百度/腾讯/有道：`api_key` / `secret` 的语义分别是 AppID/密钥、SecretId/SecretKey、appKey/appSecret，别填反。
- 看 `httplog.db` 的 `status_code` 与响应体（注意脱敏）。

### 4.3 所有引擎都失败

- 先测网络：`curl -I https://translate.googleapis.com`。
- 检查 `settings.json` 的 `proxy` / `dns_configs`；代理改后需重启（引擎 HTTP client 在启动时构建）。
- 如果开了自定义 DNS，注意 `dns_configs` 当前不会被 `writeConfig` 写回，可能已被清空（`configuration.md` §4.2）。
- 看 `kai.log` 是否有 `translate_engine_cost` 与错误堆栈。

### 4.4 Key 明明填了，重启后变空

- `config.db` 的凭据用本机设备指纹派生密钥加密。换机器、重装系统、hostname 变化都会导致解密失败。
- 处理：在本机重新填写 Key。不要从别的机器复制 `config.db` 当凭据备份。

## 5. OCR 与截图

### 5.1 截图没有反应

- 仅 macOS 支持截图翻译；Windows 没有入口。
- 确认截图快捷键已启用。
- 确认屏幕录制权限已授予；授权后重启应用。
- 用户按 ESC 取消框选时，`screencapture` 返回非 0，这是取消，不是错误。

### 5.2 OCR 报「未找到 tesseract」

- 安装：`brew install tesseract`（需要语言包再 `brew install tesseract-lang`）。
- GUI 应用不继承 shell PATH；Kai 会探测 `/opt/homebrew/bin`、`/usr/local/bin`、`/usr/bin`，也可以手动填绝对路径。
- 验证：`/opt/homebrew/bin/tesseract --version`。

### 5.3 OCR 超时 / 偶尔卡死

- Vision 默认 60s、重试 2 次；可在引擎配置里调小超时、关闭语言校正（更快）或减少重试。
- 大图/复杂排版会慢；先裁小区域再试。
- 日志里搜 `ocr_failed` / `apple_ocr_timeout`。

### 5.4 系统翻译 / 系统 OCR 不可用

- 确认是 macOS；`apple` / `vision` 在 `EngineSupported` 里仅 darwin。
- 开发构建先 `make swift-build`，确认 `pkg/swiftbridge/libkai_bridge.dylib` 存在。
- 看 `kai.log` 是否有 `swiftbridge_loaded` / `swiftbridge_unavailable`。
- 系统翻译依赖 macOS Translation.framework；系统版本过低或语言包缺失会失败（`err.apple_no_source_lang` 等）。

## 6. 自动更新

### 6.1 检查更新没反应 / 报错

- 启动检查有 1–3 分钟随机延迟，属正常。
- 托盘菜单「检查更新」可手动触发。
- 中文界面默认走 CNB，英文走 GitHub；CNB 匿名请求可能 401。临时改 `settings.json`：

```json
"updater": { "prerelease": true, "source": "github" }
```

改完重启（`SetSource` 只在启动时调用）。

### 6.2 预发布版本检测不到

- 预发布 Release 必须带 `BUILD_TIME` 与 `GIT_COMMIT` 两个附件；缺了会报「预发布版本缺少 git commit / build time 附带文件」。
- 确认托盘「预发布版」已勾选。
- 确认本地构建时间/commit 与远端比较逻辑；开发构建没有有效 `BuildTime` 时会跳过 nightly 比较。

### 6.3 下载完成但更新失败

- 更新器只认 `updater-` 前缀的压缩包；不要手动下载安装包替换。
- 校验失败通常是 `SHA256SUMS` 不匹配或下载被代理篡改；换网络/代理重试。
- macOS 自更新替换整个 `.app`，签名不一致会被 Gatekeeper 拦；优先用官方签名构建。

## 7. 数据与配置

### 7.1 历史不新增

- 相同 `(text, engine_id, from_lang, to_lang, from_ocr)` 会被去重，不会重复入库。
- 翻译失败不会写历史。
- 查库：`sqlite3 ~/.kai/data/history.db 'select count(*) from history;'`

### 7.2 设置改了不生效

- 语言/主题：保存即生效；手改文件会热重载。
- 快捷键：保存后重注册；手改文件需重启。
- 代理/DNS/HTTP 日志/更新源：重启。
- `proxy` / `http_log` / `log` / `updater` / `dns_configs` 没有设置页 UI，只能改 `settings.json`；`SaveConfig` 不会回写它们。
- `dns_configs` 还会被 `writeConfig` 丢弃（已知缺陷）。

### 7.3 重置

```bash
# 退出 Kai
rm -rf ~/.kai          # 全量
# 或只删数据库
rm -rf ~/.kai/data
```

删之前先备份 `~/.kai/data`（如果需要历史）。`config.db` 换机不可解密，备份意义有限。

## 8. 崩溃 / 卡死

- macOS 看崩溃报告：`~/Library/Logs/DiagnosticReports/Kai-*.ips`，或 `log show --predicate 'process == "Kai"' --last 10m`。
- Windows 看事件查看器 → 应用程序日志，筛选 `Kai.exe`。
- 先看 `kai.log` 最后 100 行是否有 `panic`；`main.go` 有全局 panic 兜底，会先写 ERROR 再退出。
- 如果只在某个引擎/某张图上崩，记录引擎名、图片大小、是否 OCR，便于复现。
- 不要用 `kill -9` 反复杀进程；可能留下 WAL 文件。正常退出后 WAL 会合并。

## 9. 上报模板

```text
版本：Kai x.y.z（或 commit）
平台：macOS 15.x / Windows 11 24H2
安装方式：GitHub Release / CNB / 本地构建
现象：
复现步骤：
  1.
  2.
期望：
实际：
日志（已脱敏）：
  - kai.log 末尾 50 行
  - 相关 httplog 行（status_code / url，不含 Authorization）
是否改过 settings.json：
是否使用代理/DNS：
```

贴日志前删除：API Key、token、代理密码、翻译原文/译文、剪贴板内容、完整 URL 里的查询参数。
