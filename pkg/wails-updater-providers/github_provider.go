package wails_updater_providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// githubProvider 实现 GitHub 源的 updater.Provider（nightly + stable）。
type githubProvider struct {
	client        *http.Client        // HTTP 客户端（构造期从包全局 GetClient 注入）
	lg            *slog.Logger        // 日志器（构造期从包全局 GetLogger 注入）
	repo          string              // GitHub 仓库路径
	assetMatcher  github.AssetMatcher // 资源匹配器（官方类型）
	checksumFile  string              // 校验和文件名，用于下载后校验产物完整性
	gitCommitFile string              // 预发布 git commit 文件名
	buildTimeFile string              // 预发布 build time 文件名
	token         string              // GitHub 访问令牌（Bearer）
	buildTime     time.Time           // 本机构建时间
	gitCommit     string              // 本机 git commit
	prerelease    bool                // 是否订阅 nightly（预发布）渠道
}

// t 以当前包全局 locale 渲染 i18n 文案的便捷方法。
func (g *githubProvider) t(key string, data ...any) string { return T(key, data...) }

// apiRequest 请求 GitHub API 接口，带 Accept: application/json 与 Bearer 授权（token 非空时）。
func (g *githubProvider) apiRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	return req, nil
}

// fileRequest 下载 GitHub 文件，带 Accept: application/octet-stream 与 Bearer 授权（token 非空时）。
func (g *githubProvider) fileRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	return req, nil
}

// Name 实现 updater.Provider 接口，返回 "github"。
func (g *githubProvider) Name() string { return string(SourceGithub) }

// Check 实现 updater.Provider 接口。
// 开启预发布（prerelease=true）时：prerelease 与 stable 两个候选一起参与比较（取发布时间更晚者）；
// 关闭预发布时只查稳定版（checkStable 本就排除预发布）。
func (g *githubProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	g.lg.Debug(g.t("updater_start"))
	// 关闭预发布：只查稳定版（checkStable 本就排除预发布，只看稳定版）。
	if !g.prerelease {
		rel, err := g.checkStable(ctx, req)
		if err != nil {
			return nil, err
		}
		g.lg.Info(g.t("updater_check_done"))
		return rel, nil
	}

	// 开启预发布：取发布时间最新的一条作为候选，按它自身类型判定——
	// 是预发布则下载 gitCommit/buildTime 比较；是稳定版则按版本号 isNewer 比较。
	// 即"最新版本是什么类型，就用什么方式判定"，不再单独跑稳定版第二路、也不按发布时间择优。
	g.lg.Debug(g.t("updater_check_nightly_channel"))
	return g.checkPrerelease(ctx, req)
}

// Download 实现 updater.Provider 接口，复用共用下载逻辑。
func (g *githubProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return downloadRelease(ctx, g.lg, g.client, ghDownloadURL, g.repo, rel, dst, onProgress, "", g.fileRequest)
}

// fetchChecksum 拉取并解析本源的校验和侧车，复用共用逻辑。
func (g *githubProvider) fetchChecksum(ctx context.Context, downloadURLTpl, repo string, rel *updater.Release, sidecar string) ([]byte, bool) {
	return fetchReleaseChecksum(ctx, g.lg, g.client, downloadURLTpl, repo, rel, sidecar, "", g.fileRequest)
}

// checkPrerelease 检查 GitHub 的 Pre-release（预发布）更新：遍历发布列表筛选预发布，
// 按发布时间取最新，再用 gitCommit/buildTime 文件内容判定是否需要更新。
func (g *githubProvider) checkPrerelease(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	// GitHub 的 /releases/latest 只返回稳定版（不含预发布），Pre-release 必须遍历
	// 发布列表、筛选 Prerelease==true 的所有条目，按发布时间倒序取最新一条作为候选。
	// Pre-release 不限于名为 nightly，只要标记为预发布都参与比较。
	listURL := strings.ReplaceAll(ghReleasesList, "{repo}", g.repo)
	httpReq, err := g.apiRequest(ctx, listURL)
	if err != nil {
		return nil, fmt.Errorf("%s", g.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s", g.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		g.lg.Warn(g.t("updater_warn_unauthorized"))
		return nil, fmt.Errorf("%s", g.t("updater_err_unauthorized"))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		g.lg.Warn(g.t("updater_warn_api_error", "Status", resp.StatusCode, "Body", string(body)))
		return nil, fmt.Errorf("%s", g.t("updater_err_api", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		g.lg.Warn(g.t("updater_err_decode", "Err", err.Error()))
		return nil, fmt.Errorf("%s", g.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}
	// 开启预发布：取发布时间最新的一条（不分预发布/稳定），按它自身类型判定。
	sortReleasesByPublishedAt(releases)
	rel := releases[0]
	publishedAt, err := time.Parse(time.RFC3339, rel.PublishedAt)
	if err != nil {
		g.lg.Debug(g.t("updater_nightly_no_build_time"))
		return nil, fmt.Errorf("%s", g.t("updater_err_nightly_no_published_at"))
	}

	assets := githubAssetsToReleaseAssets(rel.Assets)
	idx := g.assetMatcher(req, assets)
	if idx < 0 || idx >= len(assets) {
		// 最新一条的升级产物不匹配本机平台/架构：该候选不适用，视为 up-to-date（而非 provider 失败）。
		g.lg.Debug(g.t("updater_nightly_no_asset", "Tag", rel.TagName, "Platform", req.Platform, "Arch", req.Arch))
		return nil, nil
	}
	filename := rel.Assets[idx].Name
	sizeOf := rel.Assets[idx].Size

	// 稳定版（最新一条不是预发布）：直接比较版本号，不需要更新则 up-to-date。
	if !rel.Prerelease {
		tag := strings.TrimPrefix(rel.TagName, "v")
		if !isNewer(tag, req.CurrentVersion) {
			g.lg.Debug(g.t("updater_stable_not_newer", "Tag", tag))
			return nil, nil
		}
		out, berr := buildStableRelease(&updater.Release{}, rel.TagName, rel.Name, rel.Body, rel.HTMLURL, publishedAt, filename, sizeOf)
		if berr != nil {
			g.lg.Debug(g.t("updater_stable_build_skipped", "Tag", rel.TagName, "Err", berr.Error()))
			return nil, fmt.Errorf("%s", g.t("updater_err_build_release", map[string]any{"Err": berr.Error()}))
		}
		hash, ok := g.fetchChecksum(ctx, ghDownloadURL, g.repo, out, g.checksumFile)
		if !ok {
			g.lg.Warn(g.t("updater_checksum_fetch_failed"), "tag", rel.TagName)
			return nil, fmt.Errorf("%s", g.t("updater_err_stable_checksum_unavailable"))
		}
		out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
		g.lg.Info(g.t("updater_stable_ready", "Tag", rel.TagName, "Asset", filename))
		return out, nil
	}

	out := &updater.Release{}
	out, err = buildNightlyRelease(g.buildTime, g.gitCommit, out, rel.TagName, rel.Name, rel.Body, rel.HTMLURL, publishedAt, filename, sizeOf, rel.TargetCommitish)
	if err != nil {
		g.lg.Debug(g.t("updater_nightly_build_skipped"), "reason", err)
		return nil, fmt.Errorf("%s", g.t("updater_err_build_release", map[string]any{"Err": err.Error()}))
	}

	// Pre-release 额外下载 gitCommit / buildTime 两个文件，按内容判定是否需要更新。
	remoteCommit, okCommit := fetchGitCommitFile(ctx, g.lg, g.client, ghDownloadURL, g.repo, out, g.gitCommitFile, "", g.fileRequest)
	remoteBuildTime, okTime := fetchBuildTimeFile(ctx, g.lg, g.client, ghDownloadURL, g.repo, out, g.buildTimeFile, "", g.fileRequest)
	if !okCommit || !okTime {
		g.lg.Warn(g.t("updater_err_prerelease_meta_missing"), "Tag", rel.TagName, "Commit", okCommit, "BuildTime", okTime)
		return nil, fmt.Errorf("%s", g.t("updater_err_prerelease_meta_missing"))
	}
	// 优先比较 gitCommit：相同（兼容短 hash / 完整 hash 前缀匹配）代表不需要更新（nil,nil = up-to-date）。
	if commitEqual(g.gitCommit, remoteCommit) {
		g.lg.Debug(g.t("updater_err_nightly_same_commit", "Commit", remoteCommit))
		return nil, nil
	}
	// gitCommit 不同：比较 buildTime，本机 < 远程才可更新。
	if g.buildTime.IsZero() {
		g.lg.Debug(g.t("updater_nightly_no_local_build_time"))
		return nil, fmt.Errorf("%s", g.t("updater_err_local_build_time_empty"))
	}
	if !g.buildTime.Before(remoteBuildTime) {
		g.lg.Debug(g.t("updater_nightly_skipped", "PublishedAt", g.buildTime.Format(time.RFC3339), "BuildTime", remoteBuildTime.Format(time.RFC3339)))
		g.lg.Debug(g.t("updater_err_nightly_not_newer"))
		return nil, nil
	}

	hash, ok := g.fetchChecksum(ctx, ghDownloadURL, g.repo, out, g.checksumFile)
	if !ok {
		g.lg.Warn(g.t("updater_checksum_fetch_failed"))
		return nil, fmt.Errorf("%s", g.t("updater_err_nightly_checksum_unavailable"))
	}
	out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
	g.lg.Info(g.t("updater_nightly_ready", "Tag", rel.TagName, "Platform", req.Platform, "Arch", req.Arch))
	return out, nil
}

// checkStable 检查 GitHub 的稳定版更新：取 latest release 比对版本号。
func (g *githubProvider) checkStable(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	releaseURL := strings.ReplaceAll(ghReleaseLatest, "{repo}", g.repo)
	httpReq, err := g.apiRequest(ctx, releaseURL)
	if err != nil {
		return nil, fmt.Errorf("%s", g.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s", g.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		g.lg.Warn(g.t("updater_warn_unauthorized"))
		return nil, fmt.Errorf("%s", g.t("updater_err_unauthorized"))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		g.lg.Warn(g.t("updater_warn_api_error", "Status", resp.StatusCode, "Body", string(body)))
		return nil, fmt.Errorf("%s", g.t("updater_err_api", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		g.lg.Warn(g.t("updater_err_decode", "Err", err.Error()))
		return nil, fmt.Errorf("%s", g.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}
	if rel.Draft {
		g.lg.Warn(g.t("updater_stable_no_asset", "Tag", rel.TagName, "Platform", req.Platform, "Arch", req.Arch))
		return nil, fmt.Errorf("%s", g.t("updater_err_no_stable_release"))
	}
	tag := strings.TrimPrefix(rel.TagName, "v")
	if req.CurrentVersion != "" && !isNewer(tag, req.CurrentVersion) {
		g.lg.Debug(g.t("updater_stable_not_newer", "Tag", tag))
		// latest 不比当前新 = 已是最新（nil,nil = up-to-date，而非 provider 失败）。
		return nil, nil
	}
	publishedAt, perr := time.Parse(time.RFC3339, rel.PublishedAt)
	if perr != nil {
		g.lg.Debug(g.t("updater_stable_time_parse_failed", "Tag", rel.TagName, "Err", perr.Error()))
		publishedAt = time.Time{}
	}
	assets := githubAssetsToReleaseAssets(rel.Assets)
	idx := g.assetMatcher(req, assets)
	if idx < 0 || idx >= len(assets) {
		g.lg.Warn(g.t("updater_stable_no_asset", "Tag", rel.TagName, "Platform", req.Platform, "Arch", req.Arch))
		return nil, fmt.Errorf("%s", g.t("updater_err_no_matching_stable_asset"))
	}
	filename := rel.Assets[idx].Name
	sizeOf := rel.Assets[idx].Size
	out, berr := buildStableRelease(&updater.Release{}, rel.TagName, rel.Name, rel.Body, rel.HTMLURL, publishedAt, filename, sizeOf)
	if berr != nil {
		g.lg.Debug(g.t("updater_stable_build_skipped", "Tag", rel.TagName, "Err", berr.Error()))
		return nil, fmt.Errorf("%s", g.t("updater_err_build_release", map[string]any{"Err": berr.Error()}))
	}
	hash, ok := g.fetchChecksum(ctx, ghDownloadURL, g.repo, out, g.checksumFile)
	if !ok {
		g.lg.Warn(g.t("updater_checksum_fetch_failed"), "tag", rel.TagName)
		return nil, fmt.Errorf("%s", g.t("updater_err_stable_checksum_unavailable"))
	}
	out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
	g.lg.Info(g.t("updater_stable_ready", "Tag", rel.TagName, "Asset", filename))
	return out, nil
}

// githubAssetsToReleaseAssets 把 GitHub 发布资源归一化为官方 github.ReleaseAsset，
// 以便直接套用官方 AssetMatcher。
func githubAssetsToReleaseAssets(assets []githubAsset) []github.ReleaseAsset {
	out := make([]github.ReleaseAsset, 0, len(assets))
	for _, a := range assets {
		out = append(out, github.ReleaseAsset{
			Name: a.Name,
			Size: a.Size,
		})
	}
	return out
}
