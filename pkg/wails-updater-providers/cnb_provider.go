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

// cnbProvider 实现 CNB 源的 updater.Provider（prerelease + stable）。
type cnbProvider struct {
	client        *http.Client        // HTTP 客户端（构造期从包全局 GetClient 注入）
	lg            *slog.Logger        // 日志器（构造期从包全局 GetLogger 注入）
	repo          string              // CNB 仓库路径
	assetMatcher  github.AssetMatcher // 资源匹配器（官方类型）
	checksumFile  string              // 校验和文件名，用于下载后校验产物完整性
	gitCommitFile string              // 预发布 git commit 文件名
	buildTimeFile string              // 预发布 build time 文件名
	token         string              // CNB 访问令牌（Bearer）
	buildTime     time.Time           // 本机构建时间
	gitCommit     string              // 本机 git commit
	prerelease    bool                // 是否订阅 nightly（预发布）渠道
}

// t 以当前包全局 locale 渲染 i18n 文案的便捷方法。
func (c *cnbProvider) t(key string, data ...any) string { return T(key, data...) }

// apiRequest 请求 CNB API 接口（列表/详情），带 Accept: application/json 与 Bearer 授权（token 非空时）。
func (c *cnbProvider) apiRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// fileRequest 下载 CNB 文件（/releases/download/...），带 Accept: application/octet-stream。
// 不带 Authorization：CNB 文件端点靠 302 重定向后的时效签名地址授权，带 Bearer 反而返回 400。
func (c *cnbProvider) fileRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	return req, nil
}

// Name 实现 updater.Provider 接口，返回 "cnb"。
func (c *cnbProvider) Name() string { return string(SourceCNB) }

// Check 实现 updater.Provider 接口。
// 关闭预发布（prerelease=false）：只查稳定版（checkStable 排除预发布，按版本号比较）。
// 开启预发布（prerelease=true）：取发布时间最新的一条作为候选，按它自身类型判定——
// 是预发布（tag 含 "-"）则下载 gitCommit/buildTime 比较；是稳定版则按版本号 isNewer 比较。
// 即"最新版本是什么类型，就用什么方式判定"，不再单独跑稳定版第二路、也不按发布时间择优。
func (c *cnbProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	c.lg.Debug(c.t("updater_start"))
	// 关闭预发布：只查稳定版（checkStable 本就排除预发布，只看稳定版）。
	if !c.prerelease {
		rel, err := c.checkStable(ctx, req)
		if err != nil {
			return nil, err
		}
		c.lg.Info(c.t("updater_check_done"))
		return rel, nil
	}

	// 开启预发布：取最新一条（不分预发布/稳定），按它自身类型判定。
	c.lg.Debug(c.t("updater_check_nightly_channel"))
	return c.checkPrerelease(ctx, req)
}

// Download 实现 updater.Provider 接口，复用共用下载逻辑。
func (c *cnbProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return downloadRelease(ctx, c.lg, c.client, cnbDownloadURL, c.repo, rel, dst, onProgress, "", c.fileRequest)
}

// fetchChecksum 拉取并解析本源的校验和侧车，复用共用逻辑。
func (c *cnbProvider) fetchChecksum(ctx context.Context, downloadURLTpl, repo string, rel *updater.Release, sidecar, directURL string) ([]byte, bool) {
	return fetchReleaseChecksum(ctx, c.lg, c.client, downloadURLTpl, repo, rel, sidecar, directURL, c.fileRequest)
}

// checkNightly 检查 CNB 的 nightly（预发布）更新：取最新 tag 比对时间与 commit。
func (c *cnbProvider) checkPrerelease(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	if c.token == "" {
		c.lg.Debug(c.t("updater_nightly_no_token"))
		return nil, fmt.Errorf("%s", c.t("updater_err_no_token"))
	}

	tagsURL := strings.ReplaceAll(cnbReleaseTagList, "{repo}", c.repo)
	httpReq, err := c.apiRequest(ctx, tagsURL)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		c.lg.Warn(c.t("updater_nightly_unreachable", "Error", err.Error()))
		return nil, fmt.Errorf("%s", c.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		c.lg.Warn(c.t("updater_nightly_unauthorized"))
		return nil, fmt.Errorf("%s", c.t("updater_err_unauthorized"))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		c.lg.Warn(c.t("updater_warn_api_error", "Status", resp.StatusCode, "Body", string(body)))
		return nil, fmt.Errorf("%s", c.t("updater_err_api", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	var tagList []cnbReleaseListItem
	if err := json.NewDecoder(resp.Body).Decode(&tagList); err != nil {
		c.lg.Warn(c.t("updater_err_decode", "Err", err.Error()))
		return nil, fmt.Errorf("%s", c.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}
	if len(tagList) == 0 {
		c.lg.Warn(c.t("updater_warn_no_tags"))
		return nil, fmt.Errorf("%s", c.t("updater_err_no_tags"))
	}
	// 开启预发布：取发布时间最新的一条（不分预发布/稳定），按它自身类型判定。
	sortReleasesByPublishedAt(tagList)
	newest := tagList[0]
	tag := strings.TrimPrefix(newest.Name, "v")
	if tag == "" {
		tag = newest.Name
	}
	// CNB 直接提供 prerelease 布尔字段，以它区分预发布。
	isPre := newest.Prerelease

	releaseURL := strings.ReplaceAll(strings.ReplaceAll(cnbReleaseTagURL, "{repo}", c.repo), "{tag}", newest.TagName)
	relReq, err := c.apiRequest(ctx, releaseURL)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	relResp, err := c.client.Do(relReq)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer relResp.Body.Close()
	if relResp.StatusCode != http.StatusOK {
		c.lg.Warn(c.t("updater_warn_get_release_failed", "Tag", tag))
		return nil, fmt.Errorf("%s", c.t("updater_err_get_release_failed", map[string]any{"Status": relResp.StatusCode}))
	}
	var tagDetail cnbReleaseTagDetail
	if err := json.NewDecoder(relResp.Body).Decode(&tagDetail); err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}

	publishedAt, err := time.Parse(time.RFC3339, newest.PublishedAt)
	if err != nil {
		c.lg.Debug(c.t("updater_nightly_no_build_time"))
		return nil, fmt.Errorf("%s", c.t("updater_err_nightly_no_published_at"))
	}

	// 取资源（先按 matcher 找到升级产物文件名，供后续下载/校验使用）。
	assets := cnbReleaseAssetsToReleaseAssets(tagDetail.Assets)
	idx := c.assetMatcher(req, assets)
	if idx < 0 || idx >= len(assets) {
		// 最新一条的升级产物不匹配本机平台/架构：该候选不适用，视为 up-to-date（而非 provider 失败）。
		c.lg.Debug(c.t("updater_nightly_no_asset", "Tag", tag, "Platform", req.Platform, "Arch", req.Arch))
		return nil, nil
	}
	filename := tagDetail.Assets[idx].Name

	// 稳定版（最新一条不是预发布）：直接比较版本号，不需要更新则 up-to-date。
	if !isPre {
		if req.CurrentVersion != "" && !isNewer(tag, req.CurrentVersion) {
			c.lg.Debug(c.t("updater_stable_not_newer", "Tag", tag))
			return nil, nil
		}
		out, berr := buildStableRelease(&updater.Release{}, newest.TagName, tag, tagDetail.Body, cnbReleasePageURL(c.repo, tagDetail.TagName), publishedAt, filename, 0)
		if berr != nil {
			c.lg.Debug(c.t("updater_stable_build_skipped", "Tag", tag, "Err", berr.Error()))
			return nil, fmt.Errorf("%s", c.t("updater_err_build_release", map[string]any{"Err": berr.Error()}))
		}
		hash, ok := c.fetchChecksum(ctx, cnbDownloadURL, c.repo, out, c.checksumFile, "")
		if !ok {
			c.lg.Warn(c.t("updater_checksum_fetch_failed"))
			return nil, fmt.Errorf("%s", c.t("updater_err_nightly_checksum_unavailable"))
		}
		out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
		c.lg.Info(c.t("updater_stable_ready", "Tag", tag, "Asset", filename))
		return out, nil
	}

	// 预发布（最新一条是预发布）：下载 gitCommit / buildTime，按内容判定是否需要更新。
	out := &updater.Release{}
	out, err = buildNightlyRelease(c.buildTime, c.gitCommit, out, newest.TagName, tag, tagDetail.Body, cnbReleasePageURL(c.repo, tagDetail.TagName), publishedAt, filename, 0, "")
	if err != nil {
		c.lg.Debug(c.t("updater_nightly_build_skipped"), "reason", err)
		return nil, fmt.Errorf("%s", c.t("updater_err_build_release", map[string]any{"Err": err.Error()}))
	}

	// Pre-release 额外下载 gitCommit / buildTime 两个文件，按内容判定是否需要更新。
	remoteCommit, okCommit := fetchGitCommitFile(ctx, c.lg, c.client, cnbDownloadURL, c.repo, out, c.gitCommitFile, "", c.fileRequest)
	remoteBuildTime, okTime := fetchBuildTimeFile(ctx, c.lg, c.client, cnbDownloadURL, c.repo, out, c.buildTimeFile, "", c.fileRequest)
	if !okCommit || !okTime {
		c.lg.Warn(c.t("updater_err_prerelease_meta_missing"), "Tag", tag, "Commit", okCommit, "BuildTime", okTime)
		return nil, fmt.Errorf("%s", c.t("updater_err_prerelease_meta_missing"))
	}
	// 优先比较 gitCommit：相同（兼容短 hash / 完整 hash 前缀匹配）代表不需要更新（nil,nil = up-to-date）。
	if commitEqual(c.gitCommit, remoteCommit) {
		c.lg.Debug(c.t("updater_err_nightly_same_commit", "Commit", remoteCommit))
		return nil, nil
	}
	// gitCommit 不同：比较 buildTime，本机 < 远程才可更新。
	if c.buildTime.IsZero() {
		c.lg.Debug(c.t("updater_nightly_no_local_build_time"))
		return nil, fmt.Errorf("%s", c.t("updater_err_local_build_time_empty"))
	}
	if !c.buildTime.Before(remoteBuildTime) {
		c.lg.Debug(c.t("updater_nightly_skipped", "PublishedAt", c.buildTime.Format(time.RFC3339), "BuildTime", remoteBuildTime.Format(time.RFC3339)))
		c.lg.Debug(c.t("updater_err_nightly_not_newer"))
		return nil, nil
	}

	hash, ok := c.fetchChecksum(ctx, cnbDownloadURL, c.repo, out, c.checksumFile, "")
	if !ok {
		c.lg.Warn(c.t("updater_checksum_fetch_failed"))
		return nil, fmt.Errorf("%s", c.t("updater_err_nightly_checksum_unavailable"))
	}
	out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
	c.lg.Info(c.t("updater_nightly_ready", "Tag", tag, "Platform", req.Platform, "Arch", req.Arch))
	return out, nil
}

// checkStable 检查 CNB 的稳定版更新：遍历 tag 找比当前更新的非预发布版本。
func (c *cnbProvider) checkStable(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	if c.token == "" {
		c.lg.Debug(c.t("updater_nightly_no_token"))
		return nil, fmt.Errorf("%s", c.t("updater_err_no_token"))
	}
	tagsURL := strings.ReplaceAll(cnbReleaseTagList, "{repo}", c.repo)
	httpReq, err := c.apiRequest(ctx, tagsURL)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		c.lg.Warn(c.t("updater_warn_api_error", "Status", resp.StatusCode, "Body", string(body)))
		return nil, fmt.Errorf("%s", c.t("updater_err_api", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	var tagList []cnbReleaseListItem
	if err := json.NewDecoder(resp.Body).Decode(&tagList); err != nil {
		c.lg.Warn(c.t("updater_err_decode", "Err", err.Error()))
		return nil, fmt.Errorf("%s", c.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}
	if len(tagList) == 0 {
		c.lg.Warn(c.t("updater_warn_no_tags"))
		return nil, fmt.Errorf("%s", c.t("updater_err_no_stable_tags"))
	}
	sortReleasesByPublishedAt(tagList)

	for _, item := range tagList {
		tag := strings.TrimPrefix(item.Name, "v") // 判断/显示沿用 HEAD 原逻辑（基于发布标题去 v）
		if tag == "" {
			tag = item.Name
		}
		// 跳过预发布与草稿，只保留稳定版
		if item.Prerelease || item.Draft {
			continue
		}
		if req.CurrentVersion != "" && !isNewer(tag, req.CurrentVersion) {
			c.lg.Debug(c.t("updater_stable_not_newer", "Tag", tag))
			continue
		}
		releaseURL := strings.ReplaceAll(strings.ReplaceAll(cnbReleaseTagURL, "{repo}", c.repo), "{tag}", item.TagName)
		relReq, err := c.apiRequest(ctx, releaseURL)
		if err != nil {
			c.lg.Debug(c.t("updater_stable_req_failed", "Tag", tag, "Err", err.Error()))
			continue
		}
		relResp, err := c.client.Do(relReq)
		if err != nil {
			c.lg.Debug(c.t("updater_stable_req_failed", "Tag", tag, "Err", err.Error()))
			continue
		}
		var tagDetail cnbReleaseTagDetail
		jerr := json.NewDecoder(relResp.Body).Decode(&tagDetail)
		relResp.Body.Close()
		if jerr != nil {
			c.lg.Debug(c.t("updater_stable_decode_failed", "Tag", tag, "Err", jerr.Error()))
			continue
		}
		publishedAt, perr := time.Parse(time.RFC3339, item.PublishedAt)
		if perr != nil {
			c.lg.Debug(c.t("updater_stable_time_parse_failed", "Tag", tag, "Err", perr.Error()))
			publishedAt = time.Time{}
		}
		assets := cnbReleaseAssetsToReleaseAssets(tagDetail.Assets)
		idx := c.assetMatcher(req, assets)
		if idx < 0 || idx >= len(assets) {
			c.lg.Debug(c.t("updater_stable_no_matching_skip", "Tag", tag))
			continue
		}
		filename := tagDetail.Assets[idx].Name
		out, berr := buildStableRelease(&updater.Release{}, item.TagName, tag, tagDetail.Body, cnbReleasePageURL(c.repo, tagDetail.TagName), publishedAt, filename, 0)
		if berr != nil {
			c.lg.Debug(c.t("updater_stable_build_skipped", "Tag", tag, "Err", berr.Error()))
			continue
		}
		hash, ok := c.fetchChecksum(ctx, cnbDownloadURL, c.repo, out, c.checksumFile, "")
		if !ok {
			c.lg.Warn(c.t("updater_checksum_fetch_failed"), "tag", tag)
			continue
		}
		out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
		c.lg.Info(c.t("updater_stable_ready", "Tag", tag, "Asset", filename))
		return out, nil
	}
	c.lg.Debug(c.t("updater_stable_no_asset", "Tag", "", "Platform", req.Platform, "Arch", req.Arch))
	// 遍历完无更高稳定版 = 已是最新（nil,nil = up-to-date）。开启预发布时本函数作为另一路候选，
	// 是否采用由 Check 层与 prerelease 候选一起仲裁决定。
	c.lg.Debug(c.t("updater_err_no_stable_matched"))
	return nil, nil
}

// cnbReleaseAssetsToReleaseAssets 把 CNB release 资源归一化为官方 github.ReleaseAsset，
// 以便直接套用官方 AssetMatcher。CNB 的 Size 已为 int64，无需解析。
// 注意：CNB 下载地址统一用模板拼接（基址 https://cnb.cool，见 cnbDownloadURL），
// 不走响应里的 browser_download_url，故此处只映射 Name/Size。
func cnbReleaseAssetsToReleaseAssets(atts []cnbReleaseAsset) []github.ReleaseAsset {
	out := make([]github.ReleaseAsset, 0, len(atts))
	for _, a := range atts {
		out = append(out, github.ReleaseAsset{
			Name: a.Name,
			Size: a.Size,
		})
	}
	return out
}

// cnbReleasePageURL 拼接 CNB release 的发布页地址（用于 Metadata 展示/跳转）。
// 格式：https://cnb.cool/{repo}/-/releases/tags/{tag}
func cnbReleasePageURL(repo, tag string) string {
	if repo == "" || tag == "" {
		return ""
	}
	return "https://cnb.cool/" + repo + "/-/releases/tags/" + tag
}
