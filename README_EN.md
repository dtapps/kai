# Kai

English | [中文](README.md)

A translation tool that lives in the system tray, with input translation, screenshot translation (macOS), and copy-key selection translation, plus built-in OCR.

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/dtapps/kai?style=flat-square)](https://github.com/dtapps/kai/releases) [![Downloads](https://img.shields.io/github/downloads/dtapps/kai/total?style=flat-square)](https://github.com/dtapps/kai/releases) [![Stars](https://img.shields.io/github/stars/dtapps/kai?style=flat-square)](https://github.com/dtapps/kai/stargazers) [![Forks](https://img.shields.io/github/forks/dtapps/kai?style=flat-square)](https://github.com/dtapps/kai/network/members) [![License](https://img.shields.io/github/license/dtapps/kai?style=flat-square)](LICENSE) [![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows-007EC6?style=flat-square)](https://github.com/dtapps/kai) [![Build](https://img.shields.io/github/actions/workflow/status/dtapps/kai/push.yml?style=flat-square)](https://github.com/dtapps/kai/actions/workflows/push.yml) ![Download Requests](https://img.shields.io/endpoint?label=Download%20Requests&url=https%3A%2F%2Fdl-stats.dtapp.top%2Fshields.json%3Fplatform%3Dgithub.com%26repository%3D%2Fdtapps%2Fkai)

[![CNB Release](https://cnb.cool/dtapp/kai/-/badge/release)](https://cnb.cool/dtapp/kai/-/badge/release.link) [![CNB Stars](https://cnb.cool/dtapp/kai/-/badge/star)](https://cnb.cool/dtapp/kai) [![CNB Forks](https://cnb.cool/dtapp/kai/-/badge/fork)](https://cnb.cool/dtapp/kai) [![CNB Build](https://cnb.cool/dtapp/kai/-/badge/git/latest/ci/status/push)](https://cnb.cool/dtapp/kai) ![Download Requests](https://img.shields.io/endpoint?label=Download%20Requests&url=https%3A%2F%2Fdl-stats.dtapp.net%2Fshields.json%3Fplatform%3Dcnb.cool%26repository%3D%2Fdtapp%2Fkai)

</div>

## 1. Features

| Feature                | Default Shortcut          | Enabled by Default               | Notes                                                                                            |
|------------------------|---------------------------|----------------------------------|--------------------------------------------------------------------------------------------------|
| Input Translation      | mac: `⌥+A` / win: `Alt+A` | Yes                              | Opens the translation window; type text to translate                                             |
| Screenshot Translation | mac: `⌥+S`                | No (enable manually in Settings) | Capture a region → recognize text → translate; result shown in the screenshot window; macOS only |

> Only the two global shortcuts above exist. There is also a copy key (mac `⌘+C` / win `Ctrl+C`), simulated by the app to copy the current selection into the input box for translation; rebind or disable it under Settings → Shortcuts.

- **Translation engines**: Google, DeepL, OpenAI, Anthropic, Gemini, Baidu, Tencent, Youdao, plus macOS system translation. Google and macOS system translation work out of the box; the rest require your own API key
- **OCR**: macOS uses the system offline recognition (Vision, no install needed); local tesseract is optional. OCR is currently only reachable through the screenshot-translation entry on macOS; Windows has no screenshot entry
- **Auto-read clipboard**: when enabled, the input-translation shortcut reads the clipboard directly and translates it without simulating the copy key; the copy key is suspended while this is on to avoid double triggering
- **UI language**: built-in Chinese / English, follows the system or switches in Settings
- **Auto update**: silent check on startup + "Check for Updates" in the tray menu; the update source defaults to UI language (Chinese → CNB, English → GitHub) and can also be set manually; the pre-release channel is on by default and receives pre-release builds

## 2. Anonymous Analytics (PostHog)

Kai includes optional anonymous usage analytics (**disabled by default**; enable it under Settings → General) to understand how features are used and improve the product. When enabled, **only the anonymous data below is reported, with no personal information** (no text content, translation results, API keys, or accounts are collected):

- **Anonymous device ID**: an irreversible random ID derived one-way (HMAC) from a machine fingerprint, used to distinguish devices and measure retention; it is reset by deleting the app data.
- **Coarse location**: when enabled, PostHog derives coarse geolocation (country / region / city) from the request source IP — no precise coordinates or carrier detail, used only for regional distribution stats.
- **App identifier (app_name)**: always `kai`, used to distinguish this app/build within a shared PostHog instance.

| Event                  | When                              | Properties (all non-sensitive)                                    |
|------------------------|-----------------------------------|-------------------------------------------------------------------|
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

- **macOS**: full capabilities (system translation / system OCR / screenshot translation / copy key / auto update)
- **Windows**: input translation, copy-key translation, online translation engines and auto update work; system translation and screenshot translation are not supported (region capture is macOS only)
- Currently only macOS and Windows are supported; Linux is not supported

## 4. Repositories & Releases

### Repositories

| Platform | URL                           |
|----------|-------------------------------|
| CNB      | https://cnb.cool/dtapp/kai    |
| GitHub   | https://github.com/dtapps/kai |
| Gitea    | https://gitea.com/dtapps/kai  |
| GitLab   | https://gitlab.com/dtapps/kai |
| Gitee    | https://gitee.com/dtapps/kai  |
| GitCode  | https://gitcode.com/dtapp/kai |

> Gitea / GitCode are code mirrors only and do not publish releases.

### Releases

- **Stable release**: manually trigger the Release workflow with a version number; macOS / Windows (arm64 + amd64) are built and published as a GitHub Release
- **Pre-release**: check `prerelease` in the same workflow (or pass `pre_release=true` on the CNB side); there is no daily Nightly build anymore
- Build artifacts are mirrored from the GitHub Release to CNB / GitLab / Gitee

## 5. Documentation

Development and maintenance docs live in [docs/](docs/): project plan, CI image, Codecov / Coveralls, GitLab release sync, and more.

## 6. License

See the `LICENSE` file in the repository.

## 7. Known Issues

> The following are known limitations in the current version (Wails v3 beta.34), mostly caused by upstream framework or macOS platform behavior rather than application logic defects.

- **macOS: screenshot window may be covered on first hotkey invocation**: When invoking the screenshot translation window via the hotkey (`⌥+S`) for the first time while another app is in the foreground, it may occasionally be covered by that app's window. A subsequent invocation (window already in the event loop) works normally. Cause: Kai is an accessory app (no Dock icon), so on macOS (especially Tahoe / macOS 27) its ability to reclaim foreground focus from a background state is limited by the system; a pure Wails solution has a race on the first invocation. Tray-menu clicks are unaffected (native clicks naturally transfer foreground focus). **Workaround**: if covered, click the Kai tray icon once or trigger the hotkey again.

## 8. Acknowledgements

<a href="https://wails.io">
  <img src="https://wails.io/img/wails-button-light.svg" width="90" height="24" alt="Built with Wails">
</a>

Design inspired by [Bob](https://github.com/ripperhe/Bob) and [Easydict](https://github.com/tisfeng/Easydict).
