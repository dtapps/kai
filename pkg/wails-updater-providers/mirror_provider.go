package wails_updater_providers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// Options 控制更新器行为，全部由调用方注入。
// 注意：语言/Locale、主题/Theme、主源/Source、更新窗口名/尺寸/事件名
// 已提升为「库全局配置」（见下方 var 块与 Set/Get 包函数），不再出现在本结构体；
// 库内部直接读包级全局，调用方无需逐层传参，运行时切换调用 SetXxx 即可。
type Options struct {
	// CnbRepo CNB 仓库路径（如 your-org/your-repo）。Source 为 SourceCNB/SourceAuto 且
	// 选中 CNB 时不可为空，否则 NewMirrorProvider 返回错误。
	CnbRepo string
	// GithubRepo GitHub 仓库路径（如 your-org/your-repo）。Source 为 SourceGithub/SourceAuto
	// 且选中 GitHub 时不可为空，否则 NewMirrorProvider 返回错误。
	GithubRepo string
	// GithubToken GitHub 访问令牌（private 仓库或提频所需）。
	GithubToken string
	// CnbToken CNB 访问令牌（private 仓库或提频所需）。
	CnbToken string
	// BuildTime 本机构建时间，用于 nightly 版本时间比较（远端发布时间更新才升级）。
	// 示例：time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	// 或 time.Parse(time.RFC3339, "2026-08-15T12:00:00Z")。
	BuildTime time.Time
	// GitCommit 本机构建 git commit，用于 nightly 版本相同 commit 跳过。
	GitCommit string
	// Prerelease 是否允许预发布版本。
	Prerelease bool
	// AssetMatcher 自定义资源匹配器（可选）。类型为官方 github.AssetMatcher：
	// func(req updater.CheckRequest, assets []github.ReleaseAsset) int，返回
	// 命中 asset 的下标，或 -1 表示无匹配。为 nil 时自动使用官方
	// github.DefaultAssetMatcher（按平台/架构匹配、跳过签名与校验和附带文件）。
	// 直接复用官方类型，调用方若曾使用官方 github provider 的 matcher 可零改动搬用。
	// 库在 matcher.go 提供了 NewUpdaterAssetMatcher（仅匹配 updater- 前缀升级文件），
	// 语言自动取自包全局 GetLocale()，如需启用可显式传入 AssetMatcher: kupdater.NewUpdaterAssetMatcher。
	// 注意：NewUpdaterAssetMatcher 内部每次匹配都读包全局 GetLocale()，运行时调用
	// SetLocale 即可动态切换 matcher 日志语言（如宿主语言切换时 main 调用 kupdater.SetLocale）。
	AssetMatcher github.AssetMatcher
	// ChecksumFile 校验和文件名（用于下载后校验产物完整性）。默认 "SHA256SUMS"。
	// 内容公式：每行 "<sha256十六进制哈希>  <资产文件名>"，例如：
	//   a1b2c3d4...  kai-darwin-amd64.tar.gz
	//   9f8e7d6c...  kai-windows-amd64.zip
	ChecksumFile string
	// GitCommitFile 预发布（Pre-release）版本附带的 git commit 文件名。
	// 从原来的 SHA256SUMS 中拆出，独立成文件，便于校验预发布版本对应的 commit。
	// 为空时回退 "GIT_COMMIT"。
	// 内容公式：单行纯 commit hash（40 位十六进制），例如：
	//   9f3c1a2b7e4d5c6a8b9c0d1e2f3a4b5c6d7e8f90
	GitCommitFile string
	// BuildTimeFile 预发布（Pre-release）版本附带的 build time 文件名。
	// 从原来的 SHA256SUMS 中拆出，独立成文件，便于校验预发布版本的构建时间。
	// 为空时回退 "BUILD_TIME"。
	// 内容公式：单行 RFC3339 时间（UTC），例如：
	//   2026-08-15T12:00:00Z
	BuildTimeFile string
}

// AssetMatcherOrDefault 返回调用方自定义 AssetMatcher；为 nil 时回退官方
// github.DefaultAssetMatcher（按平台/架构匹配、跳过签名与校验和附带文件）。
func (o Options) AssetMatcherOrDefault() github.AssetMatcher {
	if o.AssetMatcher != nil {
		return o.AssetMatcher
	}
	return github.DefaultAssetMatcher
}

// ChecksumFileOrDefault 返回校验和文件名；为空时回退 "SHA256SUMS"。
func (o *Options) ChecksumFileOrDefault() string {
	if o.ChecksumFile == "" {
		return "SHA256SUMS"
	}
	return o.ChecksumFile
}

// GitCommitFileOrDefault 返回调用方自定义 GitCommitFile；为空时回退 "GIT_COMMIT"。
func (o Options) GitCommitFileOrDefault() string {
	if o.GitCommitFile == "" {
		return "GIT_COMMIT"
	}
	return o.GitCommitFile
}

// BuildTimeFileOrDefault 返回调用方自定义 BuildTimeFile；为空时回退 "BUILD_TIME"。
func (o Options) BuildTimeFileOrDefault() string {
	if o.BuildTimeFile == "" {
		return "BUILD_TIME"
	}
	return o.BuildTimeFile
}

// ===================== 库全局配置（包级变量） =====================
// 语言/Locale、主题/Theme、主源/Source、更新窗口名/尺寸/事件名均为「库全局」，
// 对外通过导出的 Set/Get 包函数读写；库内部（matcher、provider、窗口）直接
// 读取，无需调用方把这些信息塞进 Options 再逐层传参。运行时切换（跟随系统
// 语言/外观、用户改源）调用 SetXxx 即可，无需重建 provider。

var (
	globalLocale      Locale = LocaleZhCN             // 当前库全局语言，未设置时回退默认（zh-CN）
	globalTheme       Theme  = ThemeDark              // 当前库全局主题，未设置时回退默认（dark）
	globalSource      Source = SourceAuto             // 当前库全局主源偏好（auto 按语言选 CNB/GitHub）
	globalWindowName         = "updater-window"       // 内置更新窗口名
	globalWindowW            = 520                    // 内置更新窗口宽（像素，与 JS 侧 resizeToContent 的 WINDOW_WIDTH 一致，避免展开时在 348↔520 间跳变）
	globalWindowH            = 660                    // 内置更新窗口固定高（像素；Wails 内联 shim 的 Events.Emit 丢弃 payload，无法回传高度数据，故固定高度 + CSS 填满背景）
	globalResizeEvent        = "wails:updater:resize" // HTML ↔ 窗口 resize 通信事件名
	globalLogger             = slog.Default()         // 当前库全局日志器，默认 slog.Default()
	globalClient             = http.DefaultClient     // 当前库全局 HTTP 客户端，默认 http.DefaultClient
)

// SetLocale 设置库全局语言；空值归一化为默认（en-US）。
// 运行时语言切换（如跟随系统/用户设置）调用此函数即可，无需重建 provider。
func SetLocale(locale Locale) { globalLocale = normalizeLocale(string(locale)) }

// GetLocale 返回当前库全局语言（未设置时回退默认 en-US）。
func GetLocale() Locale { return normalizeLocale(string(globalLocale)) }

// SetTheme 设置库全局主题；空值归一化为默认（light）。
func SetTheme(theme Theme) { globalTheme = normalizeTheme(theme) }

// GetTheme 返回当前库全局主题（未设置时回退默认 light）。
func GetTheme() Theme { return normalizeTheme(globalTheme) }

// SetSource 设置库全局主源偏好（auto/cnb/github）。
func SetSource(v Source) { globalSource = normalizeSource(v) }

// GetSource 返回当前库全局主源偏好。
func GetSource() Source { return globalSource }

// SetWindowName 设置内置更新窗口名（一般保持默认，特殊场景覆盖）。
func SetWindowName(name string) {
	if name != "" {
		globalWindowName = name
	}
}

// GetWindowName 返回内置更新窗口名。
func GetWindowName() string { return globalWindowName }

// SetWindowSize 设置内置更新窗口尺寸（宽/高，像素，<=0 忽略）。
func SetWindowSize(w, h int) {
	if w > 0 {
		globalWindowW = w
	}
	if h > 0 {
		globalWindowH = h
	}
}

// GetWindowSize 返回内置更新窗口尺寸（宽、高）。
func GetWindowSize() (int, int) { return globalWindowW, globalWindowH }

// SetResizeEvent 设置内置更新窗口与 HTML 间通信的 resize 事件名。
func SetResizeEvent(name string) {
	if name != "" {
		globalResizeEvent = name
	}
}

// GetResizeEvent 返回内置更新窗口 resize 事件名。
func GetResizeEvent() string { return globalResizeEvent }

// SetLogger 设置库全局日志器；为 nil 时回退 slog.Default()。
func SetLogger(lg *slog.Logger) {
	if lg != nil {
		globalLogger = lg
	} else {
		globalLogger = slog.Default()
	}
}

// GetLogger 返回当前库全局日志器（默认 slog.Default()）。
func GetLogger() *slog.Logger { return globalLogger }

// SetClient 设置库全局 HTTP 客户端；为 nil 时回退 http.DefaultClient。
func SetClient(c *http.Client) {
	if c != nil {
		globalClient = c
	} else {
		globalClient = http.DefaultClient
	}
}

// GetClient 返回当前库全局 HTTP 客户端（默认 http.DefaultClient）。
func GetClient() *http.Client { return globalClient }

// MirrorProvider 合并 CNB/GitHub 双源，按全局主源偏好选择主源。
// 语言/主题/主源均读包级全局（GetLocale/GetTheme/GetSource），不持有这些字段，
// 因此运行时调用 SetXxx 即可让后续 Check/Download/Window 实时跟随。
type MirrorProvider struct {
	opts    *Options // 调用方注入的配置指针
	cnbRepo string   // CNB 仓库路径
	ghRepo  string   // GitHub 仓库路径

	cnbProvider    *cnbProvider    // CNB 子源（source=CNB/auto 选中 CNB 时生效）
	githubProvider *githubProvider // GitHub 子源（source=Github/auto 选中 GitHub 时生效）

	buildTime time.Time // 本机构建时间，用于 nightly 时间比较
	gitCommit string    // 本机 git commit，用于 nightly 同 commit 跳过

	cnbToken      string              // CNB 访问令牌
	githubToken   string              // GitHub 访问令牌
	prerelease    bool                // 是否允许预发布（nightly）
	assetMatcher  github.AssetMatcher // 资源匹配器（官方类型）
	checksumFile  string              // 校验和文件名，用于下载后校验产物完整性，默认 SHA256SUMS
	gitCommitFile string              // 预发布版本附带的 git commit 文件名，默认 GIT_COMMIT
	buildTimeFile string              // 预发布版本附带的构建时间文件名，默认 BUILD_TIME
}

// NewMirrorProvider 根据 Options 构造双源更新器。
// 仓库/令牌/构建信息/matcher 等从 opts 读取；日志器与 HTTP 客户端读包级全局
// GetLogger/GetClient（调用方构造前以 SetLogger/SetClient 注入）。
func NewMirrorProvider(opts *Options) (*MirrorProvider, error) {
	// 构造期仅校验 source 合法性（不预先定死主源），实际主源在 Check/Download
	// 时按包全局 GetSource/GetLocale 现算，从而跟随运行时 SetXxx 的修改。
	if _, err := decideSource(GetLogger()); err != nil {
		return nil, err
	}

	m := &MirrorProvider{
		opts:          opts,
		cnbRepo:       opts.CnbRepo,
		ghRepo:        opts.GithubRepo,
		cnbToken:      opts.CnbToken,
		githubToken:   opts.GithubToken,
		prerelease:    opts.Prerelease,
		assetMatcher:  opts.AssetMatcherOrDefault(),
		checksumFile:  opts.ChecksumFileOrDefault(),
		gitCommitFile: opts.GitCommitFileOrDefault(),
		buildTimeFile: opts.BuildTimeFileOrDefault(),
		buildTime:     opts.BuildTime,
		gitCommit:     opts.GitCommit,
	}

	// source 仅用于构造期选择需要初始化的子源（auto 模式按当前语言选一个初始化，
	// 另一子源在 Check 时若被选中会按需惰性初始化）。
	source, _ := decideSource(GetLogger())
	switch source {
	case SourceCNB:
		if opts.CnbRepo == "" {
			return nil, fmt.Errorf("%s", T("updater_init_repo_empty", map[string]any{"Source": string(SourceCNB)}))
		}
		// token 空值检查推迟到 Check 时再做，构造期不拦截（NewMirrorProvider 始终能建出可用 provider）。
		m.cnbProvider = &cnbProvider{
			client:        GetClient(),
			lg:            GetLogger(),
			repo:          opts.CnbRepo,
			assetMatcher:  opts.AssetMatcherOrDefault(),
			checksumFile:  opts.ChecksumFileOrDefault(),
			gitCommitFile: opts.GitCommitFileOrDefault(),
			buildTimeFile: opts.BuildTimeFileOrDefault(),
			token:         opts.CnbToken,
			buildTime:     opts.BuildTime,
			gitCommit:     opts.GitCommit,
			prerelease:    opts.Prerelease,
		}
	case SourceGithub:
		if opts.GithubRepo == "" {
			return nil, fmt.Errorf("%s", T("updater_init_repo_empty", map[string]any{"Source": string(SourceGithub)}))
		}
		m.githubProvider = &githubProvider{
			client:        GetClient(),
			lg:            GetLogger(),
			repo:          opts.GithubRepo,
			assetMatcher:  opts.AssetMatcherOrDefault(),
			checksumFile:  opts.ChecksumFileOrDefault(),
			gitCommitFile: opts.GitCommitFileOrDefault(),
			buildTimeFile: opts.BuildTimeFileOrDefault(),
			token:         opts.GithubToken,
			buildTime:     opts.BuildTime,
			gitCommit:     opts.GitCommit,
			prerelease:    opts.Prerelease,
		}
	}
	return m, nil
}

// Name 实现 updater.Provider 接口。
func (m *MirrorProvider) Name() string { return "mirror" }

// decideSource 按当前包全局 GetSource/GetLocale 解析实际主源。
// source=auto/空时按语言选择：中文走 CNB，其他走 GitHub。
func decideSource(lg *slog.Logger) (Source, error) {
	cfgSource := GetSource()
	locale := GetLocale()
	switch cfgSource {
	case "", SourceAuto:
		// 自动模式下按语言选择源：中文走 CNB，其他走 GitHub。
		if locale == LocaleZhCN {
			lg.Debug(T("updater_source_auto_selected", "Source", string(SourceCNB)))
			return SourceCNB, nil
		}
		lg.Debug(T("updater_source_auto_selected", "Source", string(SourceGithub)))
		return SourceGithub, nil
	case SourceCNB:
		return SourceCNB, nil
	case SourceGithub:
		return SourceGithub, nil
	default:
		return "", fmt.Errorf("%s", T("updater_init_unknown_source", map[string]any{"Source": string(cfgSource)}))
	}
}

// resolveSource 运行时按当前包全局 GetSource/GetLocale 解析实际主源，
// 并惰性初始化对应子源（auto 模式可能切换语言后选中构造期未初始化的子源）。
// 返回主源与对应子源；src 为 "" 表示解析失败。
func (m *MirrorProvider) resolveSource() (Source, error) {
	src, err := decideSource(GetLogger())
	if err != nil {
		return "", err
	}
	switch src {
	case SourceCNB:
		if m.cnbProvider == nil {
			m.cnbProvider = &cnbProvider{
				repo:          m.cnbRepo,
				assetMatcher:  m.assetMatcher,
				checksumFile:  m.checksumFile,
				gitCommitFile: m.gitCommitFile,
				buildTimeFile: m.buildTimeFile,
				token:         m.cnbToken,
				buildTime:     m.buildTime,
				gitCommit:     m.gitCommit,
				prerelease:    m.prerelease,
				client:        GetClient(),
				lg:            GetLogger(),
			}
		}
		return SourceCNB, nil
	case SourceGithub:
		if m.githubProvider == nil {
			m.githubProvider = &githubProvider{
				client:        GetClient(),
				lg:            GetLogger(),
				repo:          m.ghRepo,
				assetMatcher:  m.assetMatcher,
				checksumFile:  m.checksumFile,
				gitCommitFile: m.gitCommitFile,
				buildTimeFile: m.buildTimeFile,
				token:         m.githubToken,
				buildTime:     m.buildTime,
				gitCommit:     m.gitCommit,
				prerelease:    m.prerelease,
			}
		}
		return SourceGithub, nil
	default:
		return "", fmt.Errorf("%s", T("updater_init_no_source"))
	}
}

// Check 实现 updater.Provider 接口，按主源分发到对应子源。
// 主源每次现算（读取包级 GetLocale/GetSource），语言切换后无需重建即可跟随。
func (m *MirrorProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	src, err := m.resolveSource()
	if err != nil {
		return nil, err
	}
	switch src {
	case SourceCNB:
		return m.cnbProvider.Check(ctx, req)
	case SourceGithub:
		return m.githubProvider.Check(ctx, req)
	default:
		return nil, fmt.Errorf("%s", T("updater_init_no_source"))
	}
}

// Download 实现 updater.Provider 接口，按主源分发到对应子源。
// 主源每次现算（读取包级 GetLocale/GetSource），语言切换后无需重建即可跟随。
func (m *MirrorProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	src, err := m.resolveSource()
	if err != nil {
		return err
	}
	switch src {
	case SourceCNB:
		return m.cnbProvider.Download(ctx, rel, dst, onProgress)
	case SourceGithub:
		return m.githubProvider.Download(ctx, rel, dst, onProgress)
	default:
		return fmt.Errorf("%s", T("updater_init_no_source"))
	}
}

// buildStableRelease 构造稳定版 release（源无关）。
func buildStableRelease(rel *updater.Release, tag, name, notes, htmlURL string, publishedAt time.Time, filename string, size int64) (*updater.Release, error) {
	if tag == "" {
		return nil, fmt.Errorf("%s", T("updater_err_release_tag_empty"))
	}
	if rel == nil {
		rel = &updater.Release{}
	}
	rel.Version = tag
	rel.Name = name
	rel.Notes = notes
	rel.PublishedAt = publishedAt
	rel.Metadata = map[string]any{
		MetadataReleaseHTMLURL: htmlURL,
	}
	rel.Artifact = updater.Artifact{
		Filename: filename,
		Size:     size,
	}
	return rel, nil
}

// buildNightlyRelease 构造 nightly release（源无关）。
// installedBuildTime / installedGitCommit 用于判断是否需要更新（比 buildStableRelease 多一层时间/commit 校验）。
func buildNightlyRelease(installedBuildTime time.Time, installedGitCommit string, rel *updater.Release, tag, name, notes, htmlURL string, publishedAt time.Time, filename string, size int64, remoteCommit string) (*updater.Release, error) {
	if tag == "" {
		return nil, fmt.Errorf("%s", T("updater_err_nightly_tag_empty"))
	}
	if rel == nil {
		rel = &updater.Release{}
	}
	// 注意：是否需要更新的判定（gitCommit 一致 / buildTime 不更新）不在本函数做，
	// 而是由调用方 checkPrerelease 基于下载的 gitCommitFile / buildTimeFile 判定后返回 nil,nil（up-to-date）。
	// 本函数只负责构造 Release，仅在缺少必要字段（tag/发布时间）时返回真正的错误。
	if publishedAt.IsZero() {
		return nil, fmt.Errorf("%s", T("updater_err_nightly_missing_published_at"))
	}
	rel.Version = tag
	rel.Name = name
	rel.Notes = notes
	rel.PublishedAt = publishedAt
	rel.Metadata = map[string]any{
		MetadataReleaseHTMLURL: htmlURL,
	}
	rel.Artifact = updater.Artifact{
		Filename: filename,
		Size:     size,
	}
	return rel, nil
}

// publishedAtGetter 排序泛型约束：只要类型能给出 PublishedAt 字符串即可参与排序。
type publishedAtGetter interface {
	GetPublishedAt() string
}

// sortReleasesByPublishedAt 按发布时间降序排序（最新在前），CNB 与 GitHub 共用。
func sortReleasesByPublishedAt[T publishedAtGetter](list []T) {
	sort.Slice(list, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, list[i].GetPublishedAt())
		tj, ej := time.Parse(time.RFC3339, list[j].GetPublishedAt())
		if ei != nil || ej != nil {
			return list[i].GetPublishedAt() > list[j].GetPublishedAt()
		}
		return ti.After(tj)
	})
}
