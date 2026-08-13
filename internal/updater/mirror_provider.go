package updater

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// 更新源策略（检测/下载/校验按语言选源，CNB 与 GitHub 为两个平行源）：
//   - 英文用户：版本检测、下载、校验均走官方 GitHub（github provider 原始行为）。
//   - 中文用户：版本检测、下载、校验均走 CNB（需 cnbToken 鉴权）。
// CNB 与 GitHub 各自独立，不存在镜像/回退关系，各语言固定走对应源。

const (
	// cnbDownloadTpl：CNB 的 release 资源下载地址模板（仓库 dtapp/kai）。
	// 占位符：{tag} {file}
	cnbDownloadTpl = "https://cnb.cool/dtapp/kai/-/releases/download/{tag}/{file}"

	// cnbAPIBase：CNB OpenAPI 基地址（鉴权后按 tag 拉 release 元数据）。
	// 匿名访问返回 401，必须带 cnbToken（Authorization: Bearer）。仓库固定 dtapp/kai。
	cnbAPIBase = "https://api.cnb.cool"
)

// mirrorProvider 包装 github.Provider，仅重写下载地址。
type mirrorProvider struct {
	*github.Provider
	// checkProvider 用于版本检测：中文走 CNB，SHA256SUMS 也由 Check 从 CNB 拉取。
	checkProvider *github.Provider
	client        *http.Client

	// prerelease 控制是否提供 nightly 预发布更新（对应设置项 Prerelease）。
	prerelease bool
	// repo / baseURL / githubToken 用于直接按 tag 拉取 nightly Release，
	// 绕过 github provider 内置的 semver「IsNewer」门槛（nightly 非合法 semver）。
	repo        string
	baseURL     string
	githubToken string
	// cnbToken 为 CNB 只读 Token（构建期 ldflags 注入，与 githubToken 同源），
	// 预留给「检测层走 CNB 鉴权 API」使用——CNB 匿名 API 返回 401，
	// 需带 Token 才能按 tag 拉取 nightly 元数据，使中文用户断 GitHub 也能检测更新。
	cnbToken string
	// assetMatcher 复用 github provider 的平台匹配逻辑。
	assetMatcher github.AssetMatcher
	// nightlyTag 为固定预发布标签（默认 nightly）。
	nightlyTag string
	// installedBuildTime 为本机已安装二进制的构建时间（ldflags 注入）。
	// nightly 时间对比直接用它对比「远端 SHA256SUMS 的 build_time」：
	// 远端 build_time 不晚于本机 installedBuildTime 即视为已是最新（跳过）。
	// 为空时（本地 dev 构建 / 未注入 ldflags）无法比对，nightly 检测直接跳过，避免循环更新。
	installedBuildTime string
	// installedGitCommit 为本机已安装二进制的 Git 提交（ldflags 注入，短 sha）。
	// nightly 优先用它对比「远端 SHA256SUMS 的 git_commit」：两者相等即同一份代码，
	// 直接跳过更新（每日重打同一 commit 不再频繁提示）；不等再退回到 build_time 对比。
	// 为空时（本地 dev 构建 / 未注入 ldflags）跳过 commit 比对，退化回时间对比。
	installedGitCommit string
}

// NewMirrorProvider 基于 github.Config 创建按语言选源的 Provider。
// client 为全局 HTTP 客户端（含 UA 注入、代理、自定义 DNS，由 network.BuildHTTPClient 构建），
// 同时注入到 github provider 与校验文件拉取，避免自建裸 client 丢失全局注入。
// installedBuildTime 为本机已安装二进制的构建时间（ldflags 注入），nightly 时间对比用它对比
// 远端 SHA256SUMS 的 build_time，为空时 nightly 检测直接跳过（无法比对，避免循环更新）。
// cnbToken 为 CNB 只读 Token（ldflags 注入），预留给检测层走 CNB 鉴权 API（CNB 匿名 API 返回 401）。
// installedGitCommit 为本机已安装二进制的 Git 提交（ldflags 注入），nightly 优先用它对比
// 远端 SHA256SUMS 的 git_commit，相等即同一份代码直接跳过（避免每日重打同 commit 频繁提示）。
func NewMirrorProvider(cfg github.Config, client *http.Client, installedBuildTime string, installedGitCommit string, cnbToken string) (*mirrorProvider, error) {
	cfg.HTTPClient = client
	gh, err := github.New(cfg)
	if err != nil {
		return nil, err
	}
	// 版本检测用的 Provider 关闭原生 GitHub 校验文件抓取（中文的 SHA256SUMS 由 Check 从 CNB 拉取）。
	noChecksumCfg := cfg
	noChecksumCfg.ChecksumAsset = ""
	ghNoChecksum, err := github.New(noChecksumCfg)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	matcher := cfg.AssetMatcher
	if matcher == nil {
		matcher = github.DefaultAssetMatcher
	}
	return &mirrorProvider{
		Provider:           gh,
		checkProvider:      ghNoChecksum,
		client:             client,
		prerelease:         cfg.Prerelease,
		repo:               cfg.Repository,
		baseURL:            base,
		githubToken:        cfg.Token,
		cnbToken:           cnbToken,
		assetMatcher:       matcher,
		nightlyTag:         "nightly",
		installedBuildTime: installedBuildTime,
		installedGitCommit: installedGitCommit,
	}, nil
}

// Name 实现 updater.Provider。
func (m *mirrorProvider) Name() string { return "github-mirror" }

// Check 版本检测按语言选择源，与下载/校验策略一致：
//   - 英文用户：直接使用官方 GitHub（含 SHA256SUMS 校验，原始行为）。
//   - 中文用户：用 checkProvider 检测版本（不抓校验），再从 CNB 拉 SHA256SUMS 并填充 Verification。
//
// nightly 预发布检测见 checkNightly（同样按语言选源）。
func (m *mirrorProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	if m.prerelease {
		// 优先提供 nightly 预发布；失败或无 nightly 时回退稳定版逻辑。
		if rel, err := m.checkNightly(ctx, req); err != nil {
			slog.Warn(i18n.T("log.updater_nightly_failed", "Error", err))
		} else if rel != nil {
			return rel, nil
		}
		// 回退：继续走下方稳定版检测逻辑。
	}

	if i18n.GetLocale() == string(i18n.EN_US) {
		// 英文：官方 GitHub 原始行为，含 SHA256SUMS 校验。
		return m.Provider.Check(ctx, req)
	}

	rel, err := m.checkProvider.Check(ctx, req)
	if err != nil {
		return nil, err
	}
	// 中文：从 CNB 拉 SHA256SUMS。
	if digest, _, _, ok := m.fetchChecksumViaMirror(ctx, rel, "SHA256SUMS"); ok {
		rel.Verification = &updater.Verification{
			DigestAlgo: "sha256",
			Digest:     digest,
		}
	} else {
		slog.Warn(i18n.T("log.updater_checksum_fetch_failed"))
	}
	return rel, nil
}

// checkNightly 检测 nightly 预发布更新。检测源按界面语言**单一固定**，不混用、
// 不跨源回退（避免「用了 CNB 又用 GitHub」）：
//   - 英文用户：仅官方 GitHub；
//   - 中文用户：仅 CNB（需 cnbToken，匿名 401 / 网络不可达即视为「无更新」）。
//
// 仅在 Prerelease=true 时由 Check 调用。
//   - 返回 (rel, nil)：成功找到可用 nightly；
//   - 返回 (nil, nil)：无可用 nightly（该源未发布 / 草稿 / 比本机更旧 / 源不可用），
//     调用方据此回退稳定版；
//   - 返回 (nil, err)：响应解码等致命错误（非源不可用、非网络错误）。
func (m *mirrorProvider) checkNightly(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	enUS := i18n.GetLocale() == string(i18n.EN_US)

	// 按语言选单一检测源，不做跨源回退。
	if enUS {
		return m.checkNightlyGitHub(ctx, req)
	}
	return m.checkNightlyCNB(ctx, req)
}

// checkNightlyGitHub 直接按 nightlyTag 拉取 GitHub 预发布 Release，跳过 github
// provider 内置的 semver「IsNewer」门槛（nightly 非合法 semver，会被恒判为「不更新」）。
// 仅在 Prerelease=true 时调用。无可用 nightly 返回 (nil, nil)；网络错误返回 err。
func (m *mirrorProvider) checkNightlyGitHub(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	tag := m.nightlyTag
	u := m.baseURL + "/repos/" + m.repo + "/releases/tags/" + tag

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if m.githubToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+m.githubToken)
	}

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// 继续解析
	case http.StatusNotFound:
		// 尚无 nightly 预发布，回退稳定版。
		return nil, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s", i18n.T("log.updater_github_api_error", "Status", resp.StatusCode, "Body", string(body)))
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("log.updater_github_decode_error"), err)
	}

	return m.buildNightlyRelease(ctx, req, tag, rel.Name, rel.Body, rel.HTMLURL, rel.Draft, rel.PublishedAt, asReleaseAssetsLocal(rel.Assets), nil)
}

// checkNightlyCNB 通过 CNB 鉴权 API 拉取 nightly 预发布 Release（CNB 匿名 API
// 返回 401，必须带 cnbToken）。CNB release 结构与 GitHub 基本一致，但 asset.id
// 为字符串且资产自带 hash_value（sha256），可直接用于校验，无需 SHA256SUMS 侧车。
// 仅在 GitHub 网络不可达时由 checkNightly 回退调用。
func (m *mirrorProvider) checkNightlyCNB(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	tag := m.nightlyTag
	if m.cnbToken == "" {
		// 未配置 cnbToken：中文用户检测层仅用 CNB，无 token 即视为「无更新」。
		slog.Debug(i18n.T("log.updater_nightly_cnb_no_token"))
		return nil, nil
	}
	u := cnbAPIBase + "/dtapp/kai/-/releases/tags/" + tag

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/vnd.cnb.api+json")
	httpReq.Header.Set("Authorization", "Bearer "+m.cnbToken)

	resp, err := m.client.Do(httpReq)
	if err != nil {
		// CNB 网络不可达：视为「无更新」，仅告警便于排查。
		slog.Warn(i18n.T("log.updater_nightly_cnb_unreachable", "Error", err))
		return nil, nil
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// 继续解析
	case http.StatusNotFound:
		// CNB 上尚无 nightly，视为无 nightly 更新。
		return nil, nil
	case http.StatusUnauthorized:
		// token 无效或无权限：告警后视为「无更新」。
		slog.Warn(i18n.T("log.updater_nightly_cnb_unauthorized"))
		return nil, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s", i18n.T("log.updater_cnb_api_error", "Status", resp.StatusCode, "Body", string(body)))
	}

	var rel cnbRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("log.updater_cnb_decode_error"), err)
	}

	// CNB 资产自带 hash_value（sha256），按索引对应传给 buildNightlyRelease。
	hashes := make([]string, len(rel.Assets))
	for i, a := range rel.Assets {
		hashes[i] = a.HashValue
	}
	htmlURL := "https://cnb.cool/dtapp/kai/-/releases/tag/" + tag
	return m.buildNightlyRelease(ctx, req, tag, rel.Name, rel.Body, htmlURL, rel.Draft, rel.PublishedAt, asReleaseAssetsCNB(rel.Assets), hashes)
}

// buildNightlyRelease 将解析出的 nightly 元数据组装为 updater.Release，挑选匹配
// 当前平台的资产。assetHashes 与 assets 平行（按索引对应），为各资产自带的
// sha256 摘要（CNB 提供；GitHub 路径传 nil），非空时优先用于校验，避免依赖
// SHA256SUMS 侧车文件。
func (m *mirrorProvider) buildNightlyRelease(ctx context.Context, req updater.CheckRequest, tag, name, body, htmlURL string, draft bool, publishedAt time.Time, assets []github.ReleaseAsset, assetHashes []string) (*updater.Release, error) {
	if draft {
		return nil, nil
	}

	idx := m.assetMatcher(req, assets)
	if idx < 0 || idx >= len(assets) {
		return nil, fmt.Errorf("%s", i18n.T("log.updater_nightly_no_asset", "Tag", tag, "Platform", req.Platform, "Arch", req.Arch))
	}
	picked := assets[idx]

	out := &updater.Release{
		// nightly 预发布版本号就是裸 "nightly"（窗口会自动拼 v 显示为 vnightly），
		// 不掺杂本机版本号，保持 nightly 语义独立。
		Version:     tag,
		Channel:     "prerelease",
		Name:        name,
		Notes:       body,
		PublishedAt: publishedAt,
		Artifact: updater.Artifact{
			Filename: picked.Name,
			Filetype: ftypeOfLocal(picked.Name),
			Size:     picked.Size,
			Platform: req.Platform,
			Arch:     req.Arch,
		},
		Metadata: map[string]any{
			"github.asset.contentType": picked.ContentType,
			"github.release.tag":       tag,
			"github.release.htmlURL":   htmlURL,
			"github.asset.url":         picked.URL,
		},
	}
	// 校验摘要：优先使用选中资产自带的 sha256 摘要（CNB 提供）；
	// 否则从 CNB 拉 SHA256SUMS 侧车文件，全失败则降级跳过校验。
	var inlineHash string
	if idx >= 0 && idx < len(assetHashes) {
		inlineHash = assetHashes[idx]
	}
	if inlineHash != "" {
		if digest, err := hex.DecodeString(inlineHash); err == nil {
			out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: digest}
		}
	}
	// 从 SHA256SUMS 注释行读取 build_time 与 git_commit：发布时写入的、带校验保障的基准，
	// 作为 nightly 对比的权威依据（与 releases API 的 published_at 无关）。
	// 旧版 SHA256SUMS 无这些行 → 对应零值 → commit 跳过比对、build_time 兜底。
	_, bt, remoteCommit, ok := m.fetchChecksumViaMirror(ctx, out, "SHA256SUMS")
	if !ok && out.Verification == nil {
		slog.Warn(i18n.T("log.updater_checksum_fetch_failed"))
	}

	// 优先比对 Git 提交：仅当远端 commit 与本机已装 commit 都存在且相等时，
	// 判定为同一份代码直接跳过（每日重打同一 commit 不再频繁提示）。
	// 其余情况（commit 不等 / commit 缺失）一律退化到 build_time 对比，
	// 不在此处因 commit 缺失而短路跳过——避免 dev 构建或旧格式 SHA256SUMS 误判。
	localCommit := strings.TrimSpace(m.installedGitCommit)
	if remoteCommit != "" && localCommit != "" && remoteCommit == localCommit {
		slog.Debug(i18n.T("log.updater_nightly_same_commit", "Commit", localCommit))
		return nil, nil
	}

	// 本机已安装二进制的构建时间（ldflags 注入）。为空（dev 构建 / 未注入）则无法比对，
	// 直接跳过，避免「每次都当新版本」的循环更新。
	localBuildTime := m.parseInstalled()
	if localBuildTime.IsZero() {
		slog.Debug(i18n.T("log.updater_nightly_no_local_build_time"))
		return nil, nil
	}
	// 远端 nightly 的 build_time 不晚于本机已装 build_time → 已是最新，跳过。
	if bt.IsZero() {
		// 旧格式 / 解析失败：无法获得可靠 build_time，nightly 不提示更新，避免误判或循环。
		slog.Debug(i18n.T("log.updater_nightly_no_build_time"))
		return nil, nil
	}
	if !bt.After(localBuildTime) {
		slog.Debug(i18n.T("log.updater_nightly_skipped", "PublishedAt", bt.Format(time.RFC3339), "BuildTime", localBuildTime.Format(time.RFC3339)))
		return nil, nil
	}

	// 以 SHA256SUMS 的 build_time 作为发布时间（前端展示 / 日志统一语义）。
	out.PublishedAt = bt
	return out, nil
}

// cnbRelease / cnbReleaseAsset 是 CNB releases API 响应的最小子集。
// 与 GitHub 结构基本一致，但 asset.id 为字符串、且资产自带 hash_value（sha256）。
type cnbRelease struct {
	TagName     string            `json:"tag_name"`
	Name        string            `json:"name"`
	Body        string            `json:"body"`
	Prerelease  bool              `json:"prerelease"`
	Draft       bool              `json:"draft"`
	PublishedAt time.Time         `json:"published_at"`
	HTMLURL     string            `json:"html_url"`
	Assets      []cnbReleaseAsset `json:"assets"`
}

type cnbReleaseAsset struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	ContentType        string `json:"content_type"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	HashAlgo           string `json:"hash_algo"`
	HashValue          string `json:"hash_value"`
}

// asReleaseAssetsCNB 将 CNB 资产列表转为 github.ReleaseAsset（字段兼容）。
func asReleaseAssetsCNB(assets []cnbReleaseAsset) []github.ReleaseAsset {
	out := make([]github.ReleaseAsset, len(assets))
	for i, a := range assets {
		out[i] = github.ReleaseAsset{
			Name:        a.Name,
			ContentType: a.ContentType,
			Size:        a.Size,
			URL:         a.BrowserDownloadURL,
		}
	}
	return out
}

// parseInstalled 解析本机构建时间（ldflags 注入的 ISO 8601 字符串）。
func (m *mirrorProvider) parseInstalled() time.Time {
	s := strings.TrimSpace(m.installedBuildTime)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ghRelease / ghReleaseAsset 是 GitHub releases API 响应的最小子集，
// 用于直接按 tag 拉取 nightly 预发布（provider 内部结构体未导出，此处自建模）。
type ghRelease struct {
	TagName     string           `json:"tag_name"`
	Name        string           `json:"name"`
	Body        string           `json:"body"`
	Prerelease  bool             `json:"prerelease"`
	Draft       bool             `json:"draft"`
	PublishedAt time.Time        `json:"published_at"`
	HTMLURL     string           `json:"html_url"`
	Assets      []ghReleaseAsset `json:"assets"`
}

type ghReleaseAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	ContentType        string `json:"content_type"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func asReleaseAssetsLocal(assets []ghReleaseAsset) []github.ReleaseAsset {
	out := make([]github.ReleaseAsset, len(assets))
	for i, a := range assets {
		out[i] = github.ReleaseAsset{
			Name:        a.Name,
			ContentType: a.ContentType,
			Size:        a.Size,
			URL:         a.BrowserDownloadURL,
		}
	}
	return out
}

func ftypeOfLocal(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return strings.ToLower(name[i+1:])
	}
	return ""
}

// Download 实现 updater.Provider：中文走 CNB，英文走 GitHub。
func (m *mirrorProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	if rel == nil || rel.Metadata == nil {
		return fmt.Errorf("%s", i18n.T("log.updater_release_missing_metadata"))
	}
	// 英文用户：直接使用官方 GitHub 下载（原始行为）。
	if i18n.GetLocale() == string(i18n.EN_US) {
		return m.Provider.Download(ctx, rel, dst, onProgress)
	}

	tag, _ := rel.Metadata["github.release.tag"].(string)
	file := rel.Artifact.Filename

	// 中文：走 CNB 下载。
	type candidate struct {
		source string
		url    string
	}
	var candidates []candidate
	if u := m.buildURL(cnbDownloadTpl, tag, file); u != "" {
		candidates = append(candidates, candidate{"cnb", u})
	}

	for _, c := range candidates {
		resetDst(dst)
		slog.Debug(i18n.T("log.updater_source_download", "Source", c.source, "URL", c.url))
		if err := m.downloadFrom(ctx, c.url, rel, dst, onProgress); err != nil {
			slog.Warn(i18n.T("log.updater_source_failed", "Source", c.source, "Error", err))
			continue
		}
		return nil
	}
	// 所有下载源均失败。
	return fmt.Errorf("%s", i18n.T("log.updater_download_all_failed"))
}

// buildURL 将模板中的占位符替换为实际值。
func (m *mirrorProvider) buildURL(tpl, tag, file string) string {
	if tag == "" || file == "" {
		return ""
	}
	u := strings.ReplaceAll(tpl, "{tag}", tag)
	u = strings.ReplaceAll(u, "{file}", file)
	return u
}

// downloadFrom 从指定 URL 流式下载到 dst，并做基础防护。
func (m *mirrorProvider) downloadFrom(ctx context.Context, urlStr string, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s", i18n.T("log.updater_http_status", "Status", resp.StatusCode))
	}

	expected := rel.Artifact.Size
	total := expected
	if total == 0 && resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	written := int64(0)
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if onProgress != nil {
				onProgress(written, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	// 大小校验：期望大小已知且不符（多半是错误页/重定向页），回退下一源。
	if expected > 0 && written != expected {
		return fmt.Errorf("%s", i18n.T("log.updater_download_size_mismatch", "Expected", expected, "Actual", written))
	}
	return nil
}

// resetDst 在多次下载尝试间清空 dst，避免上一源的部分数据污染下一源。
func resetDst(dst io.Writer) {
	type seekTruncater interface {
		Seek(offset int64, whence int) (int64, error)
		Truncate(size int64) error
	}
	if st, ok := dst.(seekTruncater); ok {
		_, _ = st.Seek(0, io.SeekStart)
		_ = st.Truncate(0)
	}
}

// fetchChecksumViaMirror 按语言单一源拉取校验侧车文件（如 SHA256SUMS）：
//   - 中文：仅 CNB；
//   - 非中文：仅官方 GitHub（资产 URL 目录替换 / 标准仓库地址）。
//
// 解析出目标资产（rel.Artifact.Filename）的 sha256 摘要，并同时读取 SHA256SUMS 注释行中
// 发布的 build_time 与 git_commit（均由 CI 写入，作为 nightly 对比的权威基准）。
// 返回 (摘要, build_time, git_commit, 是否成功)；build_time / git_commit 解析失败时为
// 零值（time.Time{} / 空串），调用方忽略。
func (m *mirrorProvider) fetchChecksumViaMirror(ctx context.Context, rel *updater.Release, sidecar string) ([]byte, time.Time, string, bool) {
	// 兜底：rel 缺失不应 panic（否则会拖垮主进程）。
	// 注意：不能因 rel.Metadata == nil 直接失败——github Provider 返回的 Release 不一定填 Metadata
	// （nightly 路径的 out 是手工构造的，所以正常；稳定版 checkProvider.Check 的 rel.Metadata 可能为 nil）。
	// 这里统一用 Metadata["github.release.tag"]，取不到时再回退 rel.Version（稳定版 tag 即发布版本号）。
	if rel == nil {
		slog.Warn(i18n.T("log.updater_checksum_fetch_failed"))
		return nil, time.Time{}, "", false
	}
	tag, _ := rel.Metadata["github.release.tag"].(string)
	if tag == "" {
		tag = rel.Version
	}
	target := rel.Artifact.Filename

	var urls []string
	if i18n.GetLocale() == string(i18n.EN_US) {
		// 非中文：仅官方 GitHub（资产 URL 目录替换 / 标准仓库地址）。
		if ghURL, _ := rel.Metadata["github.asset.url"].(string); ghURL != "" {
			if idx := strings.LastIndex(ghURL, "/"); idx >= 0 {
				urls = append(urls, ghURL[:idx+1]+sidecar)
			}
		}
		if tag != "" && m.repo != "" {
			urls = append(urls, fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", m.repo, tag, sidecar))
		}
	} else {
		// 中文：仅 CNB。
		if tag != "" {
			if u := m.buildURL(cnbDownloadTpl, tag, sidecar); u != "" {
				urls = append(urls, u)
			}
		}
	}
	if len(urls) == 0 {
		// 既无 tag 也无资产 URL，无法拼出任何 CNB 地址，明确记录原因而非静默失败。
		slog.Warn(i18n.T("log.updater_checksum_no_url", "Tag", tag))
		return nil, time.Time{}, "", false
	}

	for _, u := range urls {
		slog.Debug(i18n.T("log.updater_checksum_download", "URL", u))
		body, err := m.fetchText(ctx, u)
		if err != nil {
			slog.Warn(i18n.T("log.updater_checksum_source_failed", "URL", u, "Error", err))
			continue
		}
		digest, ok := parseChecksumLine(string(body), target)
		if !ok {
			slog.Warn(i18n.T("log.updater_checksum_parse_failed", "URL", u, "Target", target))
			continue
		}
		// 同时从 SHA256SUMS 的注释行读取发布时写入的 build_time 与 git_commit，
		// 作为 nightly 对比的权威基准（带校验、可靠，不依赖 releases API）。
		bt := parseChecksumBuildTime(string(body))
		commit := parseChecksumGitCommit(string(body))
		return digest, bt, commit, true
	}
	return nil, time.Time{}, "", false
}

// fetchText 从指定 URL 拉取文本内容（用于校验侧车文件）。
func (m *mirrorProvider) fetchText(ctx context.Context, urlStr string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", i18n.T("log.updater_http_status", "Status", resp.StatusCode))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// parseChecksumLine 从 sha256sum 风格的清单中提取 target 的摘要。
// 每行格式为 "<hex-digest>  <filename>"，文件名按 base name 比较，
// 容忍 sha256sum --binary 在文件名前加的 "*" 标记。
func parseChecksumLine(body, target string) ([]byte, bool) {
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(name) == target || name == target {
			if digest, err := hex.DecodeString(fields[0]); err == nil {
				return digest, true
			}
		}
	}
	return nil, false
}

// parseChecksumBuildTime 从 sha256sum 风格的清单注释行中读取发布时写入的 build_time。
// CI 生成 SHA256SUMS 时需追加一行：# build_time=2006-01-02T15:04:05Z（UTC，RFC3339）。
// 该时间作为 nightly 版本时间对比的权威基准（带校验保障，不依赖 releases API 的 published_at）。
// 未找到或解析失败返回 time.Time{}。
func parseChecksumBuildTime(body string) time.Time {
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		// 容忍 "# build_time=xxx" 或 "#build_time=xxx"（= 前后可有空格）。
		if !strings.HasPrefix(kv, "build_time") {
			continue
		}
		rest := strings.TrimPrefix(kv, "build_time")
		rest = strings.TrimPrefix(rest, "=")
		rest = strings.TrimSpace(rest)
		if t, err := time.Parse(time.RFC3339, rest); err == nil {
			return t
		}
		// 容忍带空格/换行的其它写法，再试一次去引号。
		if t, err := time.Parse(time.RFC3339, strings.Trim(rest, `"`)); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseChecksumGitCommit 从 sha256sum 风格的清单注释行中读取发布时写入的 git_commit（短 sha）。
// CI 生成 SHA256SUMS 时需追加一行：# git_commit=abc1234。
// 该提交作为 nightly 版本对比的优先判据（与 build_time 相比，能精确区分「代码是否变化」，
// 避免每日重打同一 commit 时因时间单调递增而被反复提示更新）。
// 未找到或解析失败返回空串（调用方退化回 build_time 对比）。
func parseChecksumGitCommit(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		// 容忍 "# git_commit=xxx" 或 "#git_commit=xxx"（= 前后可有空格）。
		if !strings.HasPrefix(kv, "git_commit") {
			continue
		}
		rest := strings.TrimPrefix(kv, "git_commit")
		rest = strings.TrimPrefix(rest, "=")
		rest = strings.TrimSpace(rest)
		// 只保留合法的十六进制提交串（短 sha，≤40 位），其它写法视为无效。
		rest = strings.Trim(rest, `"`)
		if rest != "" && len(rest) <= 40 && isHex(rest) {
			return rest
		}
	}
	return ""
}

// isHex 判断字符串是否全为十六进制字符。
func isHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
