# wails-updater-providers

Wails v3 自更新器的多源 Provider 实现包。封装 CNB / GitHub 双源，对外只暴露官方
`github.com/wailsapp/wails/v3/pkg/updater` 的类型，调用方 `import` 后直接
`updater.Config{Providers: []updater.Provider{...}}` 即可使用，无需引入 GitHub 私有类型。

## 特性

- 双源支持：`cnb` / `github` / `auto`（按可达性自动选择）。
- 注入项与全局配置分离：`Options` 只承载调用方注入项（`Logger`、`Client`、
  `CnbRepo`、`GithubRepo`、`Token`、`BuildTime`、`GitCommit`、`Prerelease`、
  `AssetMatcher`、`ChecksumFile` 等）；**语言/主题/主源为包级全局**，由 `SetLocale` /
  `SetTheme` / `SetSource` 设置，库内部（`matcher` / `provider` / 更新窗口）直接读取，
  运行时调用 `SetXxx` 即可实时跟随，无需重新构造 provider。
- 内建 i18n：日志语言由包全局 `GetLocale()` 驱动（`LocaleZhCN` / `LocaleEnUS`），默认
  `LocaleZhCN`（中文），不写死。
- 自定义资源匹配器：默认回退官方 `github.DefaultAssetMatcher`，亦可启用库内置的
  `NewUpdaterAssetMatcher`（仅匹配 `updater-` 前缀的升级专用压缩包）。
- 双判定通道：稳定版按版本号比较；预发布（nightly）按发布源附带的 `GIT_COMMIT` /
  `BUILD_TIME` 独立文件判定（优先 commit 相等跳过，回退 build time 对比）。

## 安装

```go
import kupdater "cnb.cool/dtapp/kai/pkg/wails-updater-providers"
```

## 快速使用

```go
import (
    "log/slog"

    "github.com/wailsapp/wails/v3/pkg/updater"

    kupdater "cnb.cool/dtapp/kai/pkg/wails-updater-providers"
)

func main() {
    // 1. 设置包全局语言/主源（一次设置，运行时亦可经 SetXxx 动态切换）。
    kupdater.SetLocale(kupdater.LocaleZhCN) // 或 kupdater.LocaleEnUS
    kupdater.SetSource(kupdater.SourceAuto) // cnb / github / auto
    // 2. 构造 Options（只注入调用方侧数据；语言/主题/源由包全局承载）。
    opts := kupdater.Options{
        CnbRepo:     "your-org/your-repo",
        GithubRepo:  "your-org/your-repo",
        // Logger / Client / Token / BuildTime / GitCommit / Prerelease 等按需填写，
        // 不填则走默认值（Logger=slog.Default()、Client=http.DefaultClient 等）。
    }
    // 3. 如需自定义资源匹配（仅匹配 updater- 前缀升级文件），启用库内置 matcher。
    //    matcher 内部每次匹配直接读包全局 GetLocale()，无需传 Options。
    opts.AssetMatcher = kupdater.NewUpdaterAssetMatcher()

    // 3. 构造 provider 并交给官方 updater 使用。
    provider, err := kupdater.NewMirrorProvider(&opts)
    if err != nil {
        slog.Error("init updater provider failed", "error", err)
        return
    }

    if err := app.Updater.Init(updater.Config{
        CurrentVersion: "1.0.0",
        Providers:      []updater.Provider{provider},
        Window:         kupdater.OpenUpdaterWindow(app),
    }); err != nil {
        slog.Error("init updater failed", "error", err)
        return
    }

    // 监听更新就绪事件。
    app.Event.On(updater.EventUpdateReady, func(e *application.CustomEvent) {
        slog.Info("update ready")
    })
}
```

### 自带更新窗口（BYO 模式，推荐）

内置的 `provider.Window()` 走官方 Builtin 模式：框架负责创建/显示/隐藏窗口，
尺寸为写死常量、不随 release notes 内容自适应，且 HTML 侧无法直接调 `Window.SetSize`。
本包额外提供 **BYO（Bring-Your-Own）窗口**，由库自建并管理原生窗口，解决上述限制：

- 窗口尺寸由 JS 驱动的 `ResizeObserver` → `wails:updater:resize` 事件 → `win.SetSize`
  实现内容自适应（notes 变长时窗口自动加高）。
- 语言/主题实时跟随：`SetUpdaterLocaleTheme(app)` 在原窗口上原地刷新 HTML。
- 启动不弹窗（默认 `Hidden`）；菜单点击检查更新时调 `kupdater.ShowUpdaterWindow(app)`
  强制重建可见窗口（含双 `Show()` + `Focus()` 兜底 Wails 已知首次 Show 不显示的 bug）。
- 当前版本号由 Go 模板在渲染时注入（`app.Updater.CurrentVersion()`），不依赖
  `EventMeta` 的时序，避免 BYO 模式下收不到 snapshot 而显示「—」。

```go
// 1. Init 时挂载 BYO 窗口选项（而非 provider.Window()）。
winOpt := kupdater.OpenUpdaterWindow(app)
app.Updater.Init(updater.Config{
    CurrentVersion: buildinfo.Version, // 注入当前版本，HTML 模板渲染时用
    Providers:      []updater.Provider{provider},
    Window:         winOpt,
})

// 2. 菜单「检查更新」点击：先确保窗口存活/重建并显示，再触发检查。
kupdater.ShowUpdaterWindow(app)
_ = app.Updater.CheckAndInstall(context.Background())

// 3. 语言/主题切换时同步刷新窗口文案（原地重建 HTML，不销毁窗口）。
kupdater.SetUpdaterLocaleTheme(app)
```

#### BYO 窗口用户事件契约

BYO 模式下框架**不**在用户关闭窗口时 show/hide 我们的窗口，而是靠本包监听以下
自定义事件自行处理显隐（见 `updater_window.go` 的 `registerCloseHandler`）：

| 事件名 | 触发时机 | 库行为 |
| --- | --- | --- |
| `wails:updater:user:cancel` | 用户点「取消」 | 隐藏更新窗口 |
| `wails:updater:user:skip`   | 用户点「跳过此版本」 | 隐藏更新窗口 |
| `wails:updater:user:remind` | 用户点「稍后提醒我」 | 隐藏更新窗口 |

窗口名/尺寸等可通过 `SetWindowName` / `SetWindowSize` 覆盖（包级全局，默认
`kai-updater-window`、520×660）。

> 注意：框架在 `WindowClosing` 时有一个内部监听器会**无条件销毁**窗口并从注册表
> 移除（不读 `event.Cancelled`），因此 `Close()` 被实现为 no-op——窗口显隐完全由
> 本包控制（显示走 `ShowUpdaterWindow`、隐藏走用户关闭事件或 `WindowClosing →
> Hide`），避免「重开 session 误藏窗口」导致一闪而过。
    // 监听更新就绪事件。
    app.Event.On(updater.EventUpdateReady, func(e *application.CustomEvent) {
        slog.Info("update ready")
    })
}
```

## Options 字段说明

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Logger` | `*slog.Logger` | 自定义日志器，默认 `slog.Default()`。 |
| `Client` | `*http.Client` | 自定义 HTTP 客户端，默认 `http.DefaultClient`。 |
| `CnbRepo` | `string` | CNB 仓库路径（如 `your-org/your-repo`）。`Source` 选中 CNB 时必填。 |
| `GithubRepo` | `string` | GitHub 仓库路径（如 `your-org/your-repo`）。`Source` 选中 GitHub 时必填。 |
| `CnbToken` | `string` | CNB 访问令牌（私有仓库或提频所需）。 |
| `GithubToken` | `string` | GitHub 访问令牌（私有仓库或提频所需）。 |
| `BuildTime` | `time.Time` | 本机构建时间，用于 nightly 版本时间比较。 |
| `GitCommit` | `string` | 本机构建 git commit，用于 nightly 相同 commit 跳过。 |
| `Prerelease` | `bool` | 是否允许预发布（nightly）版本。 |
| `AssetMatcher` | `github.AssetMatcher` | 自定义资源匹配器，为 nil 时回退官方 `github.DefaultAssetMatcher`。 |
| `ChecksumFile` | `string` | 校验和侧车文件名，默认 `SHA256SUMS`。 |
| `GitCommitFile` | `string` | 预发布版本附带的 git commit 文件名，默认 `GIT_COMMIT`。供 nightly 优先比对「远端 commit == 本机 commit」以跳过同一份代码的更新。 |
| `BuildTimeFile` | `string` | 预发布版本附带的 build time 文件名，默认 `BUILD_TIME`。供 nightly 在 commit 不同回退比对构建时间。 |

> 语言 / 主题 / 主源 **不在** `Options` 中，而是包级全局：用 `kupdater.SetLocale` /
> `kupdater.SetTheme` / `kupdater.SetSource` 设置，`GetLocale` / `GetTheme` / `GetSource`
> 读取。默认 `LocaleZhCN`（中文）、`ThemeDark`（深色）、`SourceAuto`（自动）。

## 资源匹配器

### 默认行为

`AssetMatcher` 为 nil 时自动使用官方 `github.DefaultAssetMatcher`（按平台/架构匹配、跳过
签名与校验和附带文件）。

### 自定义：仅匹配升级专用文件

库内置 `NewUpdaterAssetMatcher`，仅匹配「升级专用文件」（文件名以 `updater-` 开头）：

- 打包/安装文件（`Windows -install.exe`、`macOS .app.zip`、`Linux AppImage/deb/rpm/pkg.tar.zst`）
  只用于首次安装，不用于自更新；
- 升级文件统一压缩（`Windows`/`macOS` 为 `.zip`，`Linux` 为 `.tar.gz`），内含单一二进制，
  由 updater 下载、校验（`SHA256SUMS`）后替换自身。

语言取自包全局 `GetLocale()`，不写死：

```go
updOpts.AssetMatcher = kupdater.NewUpdaterAssetMatcher()
```

## 校验和

下载产物默认用 `SHA256SUMS` 校验。若你的发布使用其他文件名（如 `checksums.txt`），
可通过 `Options.ChecksumFile` 覆盖。

## 更新判定逻辑

稳定版与预发布（nightly）走不同的判定通道，由 `Options.Prerelease` 控制。

### 关闭预发布（`Prerelease=false`）

只查稳定版，唯一判定依据是 `updater.CheckRequest.CurrentVersion` 走 `isNewer`
比较版本号。无候选即视为已是最新（`nil,nil`）。

### 开启预发布（`Prerelease=true`）

取发布时间**最新的一条**作为唯一候选，按它自身类型判定（单通道，不双路择优、不回退到第二路）：

- **最新一条是预发布**：下载 `GitCommitFile` / `BuildTimeFile` 两个独立文件：
  - 远端 commit == 本机 commit → 同一份代码，跳过更新（`nil,nil`）；
  - commit 不同但本机 buildTime ≥ 远端 buildTime → 不更新（`nil,nil`）；
  - commit 不同且本机 buildTime < 远端 buildTime → 返回该 nightly 候选。
- **最新一条是稳定版**：直接 `isNewer(tag, CurrentVersion)` 比版本号，需要更新才返回。
- 资产不匹配本机平台/架构：该候选不适用，视为已是最新（`nil,nil`）。

> `GitCommitFile` / `BuildTimeFile` 由 CI 在发布预览版（nightly）时作为独立文件随
> release 资产一起发布（文件名默认 `GIT_COMMIT` / `BUILD_TIME`）。稳定版 release
> 不发布这两个文件，也不依赖此判定。

### 当前版本来源

nightly 不把版本号纳入新旧判定，但会如实记录并打印 `updater.CheckRequest.CurrentVersion`；
稳定版的 `CheckRequest.CurrentVersion` 是真正参与比较的依据。两者统一从
`updater.CheckRequest.CurrentVersion` 取得，不另设 Options 字段。
