# Kai

A translation tool that lives in the system tray, with selection, screenshot and input translation plus built-in OCR.

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

## 1. Features

| Feature                | Default Shortcut          | Enabled by Default               | Notes                                                                                |
| ---------------------- | ------------------------- | -------------------------------- | ------------------------------------------------------------------------------------ |
| Input Translation      | mac: `⌥+A` / win: `Alt+A` | Yes                              | Opens the translation window; type text to translate                                 |
| Screenshot Translation | mac: `⌥+S` / win: `Alt+S` | No (enable manually in Settings) | Capture a region → recognize text → translate; result shown in the screenshot window |

> Only the two global shortcuts above exist. There is also a copy key (`⌘+C` / `Ctrl+C`) that sends the selected text to the clipboard for the translator to read.

- **Translation engines**: DeepL, Google, OpenAI, Baidu, Tencent, Youdao, plus macOS system translation. Google and system translation work out of the box; the rest require your own API key
- **OCR**: macOS uses the system offline recognition (no install needed); local tesseract is optional. Region-capture trigger is macOS only
- **UI language**: built-in Chinese / English, follows the system or switches in Settings
- **Auto update**: silent check on startup + "Check for Updates" in the tray menu; the update source defaults to UI language (Chinese → CNB, English → GitHub) and can also be set manually; turning on the pre-release channel receives the daily nightly builds

## 2. Anonymous Analytics (PostHog)

Kai includes optional anonymous usage analytics (**disabled by default**; enable it under Settings → General) to understand how features are used and improve the product. When enabled, **only the anonymous data below is reported, with no personal information** (no text content, translation results, API keys, or accounts are collected):

- **Anonymous device ID**: an irreversible random ID derived one-way (HMAC) from a machine fingerprint, used to distinguish devices and measure retention; it is reset by deleting the app data.
- **Coarse location**: when enabled, PostHog derives coarse geolocation (country / region / city) from the request source IP — no precise coordinates or carrier detail, used only for regional distribution stats.
- **App identifier (app_name)**: always `kai`, used to distinguish this app/build within a shared PostHog instance.

| Event                  | When                              | Properties (all non-sensitive)                                    |
| ---------------------- | --------------------------------- | ----------------------------------------------------------------- |
| `app_installed`        | First launch after install (once) | version, channel                                                  |
| `app_started`          | Every launch                      | version, OS, UI language, channel                                 |
| `translate_input`      | Input translation                 | engines used, source/target language, text-length bucket, trigger |
| `translate_screenshot` | Screenshot translation            | engines used, OCR provider, whether retried                       |
| `settings_opened`      | Settings opened                   | tab                                                               |
| `feature_toggled`      | Feature toggle switched           | feature name, on/off                                              |
| `error_occurred`       | Translation/OCR failure           | error kind, engine/OCR provider involved                          |

Notes:

- Text length is reported only as a **bucket** (`0` / `1-20` / `21-100` / `101-500` / `501-2000` / `2000+`), never the actual length or content.
- Turning the switch off stops reporting immediately and closes the connection; dev builds and builds without a configured analytics key never report.
- Data is reported via PostHog (`https://us.i.posthog.com`).

## 3. Platform Support

- **macOS**: full capabilities (system translation / system OCR / copy key / auto update)
- **Windows**: online translation and OCR (requires local tesseract) work, copy key works, auto update works; system translation is not supported
- Currently only macOS and Windows are supported; Linux is not supported

## 4. Repositories & Releases

### Repositories

| Platform | URL                           |
| -------- | ----------------------------- |
| CNB      | https://cnb.cool/dtapp/kai    |
| GitHub   | https://github.com/dtapps/kai |
| Gitea    | https://gitea.com/dtapps/kai  |
| GitLab   | https://gitlab.com/dtapps/kai |
| Gitee    | https://gitee.com/dtapps/kai  |
| GitCode  | https://gitcode.com/dtapp/kai |

### Releases

- **Stable release**: manually trigger the Release workflow with a version number; macOS / Windows are built and published
- **Nightly**: a `nightly` pre-release is built automatically every day
- Build artifacts are downloaded from the GitHub Release and then published to CNB

## 5. License

See the `LICENSE` file in the repository.

## 6. Known Issues

> The following are known limitations in the current version (Wails v3 beta.9), mostly caused by upstream framework or macOS platform behavior rather than application logic defects.

- **macOS: screenshot window may be covered on first hotkey invocation**: When invoking the screenshot translation window via the hotkey (`⌥+S`) for the first time while another app is in the foreground, it may occasionally be covered by that app's window. A subsequent invocation (window already in the event loop) works normally. Cause: Kai is an accessory app (no Dock icon), so on macOS (especially Tahoe / macOS 27) its ability to reclaim foreground focus from a background state is limited by the system; a pure Wails solution has a race on the first invocation. Tray-menu clicks are unaffected (native clicks naturally transfer foreground focus). **Workaround**: if covered, click the Kai tray icon once or trigger the hotkey again.
- **Custom title bar needs a click to activate**: On macOS, the custom-drawn title bar (frameless + transparent, used to follow the in-app light/dark theme) occasionally requires one extra click on the traffic lights (close/minimize/fullscreen) right after the window appears. This is platform behavior of an accessory app's window activation timing; interaction is normal once activated.

## 7. Acknowledgements

Design inspired by [Bob](https://github.com/ripperhe/Bob) and [Easydict](https://github.com/tisfeng/Easydict).
