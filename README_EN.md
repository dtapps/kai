# Kai

A translation tool that lives in the system tray, with selection, screenshot and input translation plus built-in OCR.

[English](./README_EN.md) | [中文](./README.md)

## 1. Features

| Feature | Default Shortcut | Enabled by Default | Notes |
|---------|------------------|--------------------|-------|
| Input Translation | mac: `⌥+A` / win: `Alt+A` | Yes | Opens the translation window; type text to translate |
| Screenshot Translation | mac: `⌥+S` / win: `Alt+S` | No (enable manually in Settings) | Capture a region → recognize text → translate; result shown in the screenshot window |

> Only the two global shortcuts above exist. There is also a copy key (`⌘+C` / `Ctrl+C`) that sends the selected text to the clipboard for the translator to read.

- **Translation engines**: DeepL, Google, OpenAI, Baidu, Tencent, Youdao, plus macOS system translation. Google and system translation work out of the box; the rest require your own API key
- **OCR**: macOS uses the system offline recognition (no install needed); local tesseract is optional. Region-capture trigger is macOS only
- **UI language**: built-in Chinese / English, follows the system or switches in Settings
- **Auto update**: silent check on startup + "Check for Updates" in the tray menu; the update source defaults to UI language (Chinese → CNB, English → GitHub) and can also be set manually; turning on the pre-release channel receives the daily nightly builds

## 2. Platform Support
- **macOS**: full capabilities (system translation / system OCR / copy key / auto update)
- **Windows**: online translation and OCR (requires local tesseract) work, copy key works, auto update works; system translation is not supported
- Currently only macOS and Windows are supported; Linux is not supported

## 3. Repositories & Releases

### Repositories
| Platform | URL |
|----------|-----|
| CNB | https://cnb.cool/dtapp/kai |
| GitHub | https://github.com/dtapps/kai |
| Gitea | https://gitea.com/dtapps/kai |
| GitLab | https://gitlab.com/dtapps/kai |
| Gitee | https://gitee.com/dtapps/kai |
| GitCode | https://gitcode.com/dtapp/kai |

### Releases
- **Stable release**: manually trigger the Release workflow with a version number; macOS / Windows are built and published
- **Nightly**: a `nightly` pre-release is built automatically every day
- Build artifacts are downloaded from the GitHub Release and then published to CNB

## 4. License
See the `LICENSE` file in the repository.

## 5. Known Issues

> The following are known limitations in the current version (Wails v3 beta.9), mostly caused by upstream framework or macOS platform behavior rather than application logic defects.

- **macOS: screenshot window may be covered on first hotkey invocation**: When invoking the screenshot translation window via the hotkey (`⌥+S`) for the first time while another app is in the foreground, it may occasionally be covered by that app's window. A subsequent invocation (window already in the event loop) works normally. Cause: Kai is an accessory app (no Dock icon), so on macOS (especially Tahoe / macOS 27) its ability to reclaim foreground focus from a background state is limited by the system; a pure Wails solution has a race on the first invocation. Tray-menu clicks are unaffected (native clicks naturally transfer foreground focus). **Workaround**: if covered, click the Kai tray icon once or trigger the hotkey again.
- **Custom title bar needs a click to activate**: On macOS, the custom-drawn title bar (frameless + transparent, used to follow the in-app light/dark theme) occasionally requires one extra click on the traffic lights (close/minimize/fullscreen) right after the window appears. This is platform behavior of an accessory app's window activation timing; interaction is normal once activated.

## 6. Acknowledgements
Design inspired by [Bob](https://github.com/ripperhe/Bob) and [Easydict](https://github.com/tisfeng/Easydict).
