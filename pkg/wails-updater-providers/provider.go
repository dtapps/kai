package wails_updater_providers

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// gitCommitRe 校验 GIT_COMMIT 文件内容：单行十六进制 commit hash（短 hash 或完整 40 位均可）。
// CI 与本地均使用 `git rev-parse --short HEAD`（默认 7 位短 hash），比较按字符串相等，
// 因此只校验"是合法的十六进制 hash"，不强制 40 位。
var gitCommitRe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// commitEqual 判断本机 git commit 与远端 git commit 是否指向同一提交。
// 两端可能一端是短 hash（如 CI 的 `git rev-parse --short HEAD` 默认 7 位）、
// 另一端是完整 40 位 hash，因此采用"前缀匹配"而非纯字符串相等：
// 较短者是对较长者的前缀（或两者完全相同）即视为同一提交，避免误判为"不同"而强制更新。
func commitEqual(local, remote string) bool {
	if local == "" || remote == "" {
		return false
	}
	if len(local) <= len(remote) {
		return strings.HasPrefix(remote, local)
	}
	return strings.HasPrefix(local, remote)
}

// sha256Re 校验 SHA256SUMS 侧车中挑出的哈希：64 位十六进制。
var sha256Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// 下载地址模板：{repo} 在运行时替换为实际仓库路径（如 example-org/example-repo），
// {tag} 用版本号（tag_name），{file} 为资源文件名。
// CNB 公开下载基址为 https://cnb.cool（与响应 browser_download_url 同源，无需鉴权）；
// GitHub 公开下载基址为 https://github.com。两者均用模板拼接。
const (
	cnbDownloadURL    = "https://cnb.cool/{repo}/-/releases/download/{tag}/{file}"
	ghDownloadURL     = "https://github.com/{repo}/releases/download/{tag}/{file}"
	cnbReleaseTagList = "https://api.cnb.cool/{repo}/-/releases?page=1&page_size=20"
	cnbReleaseTagURL  = "https://api.cnb.cool/{repo}/-/releases/tags/{tag}"
	ghReleaseLatest   = "https://api.github.com/repos/{repo}/releases/latest"
	ghReleasesList    = "https://api.github.com/repos/{repo}/releases?per_page=100"
)

// 下载中转服务：把源站原始 HTTPS 下载地址拼到中转域名之后（原始地址已以 https:// 开头，
// 故中间无需再加斜杠），用于下载计数/加速。GitHub -> dl-stats.dtapp.top，CNB -> dl-stats.dtapp.net。
// 仅作用于 release 文件下载地址；API 域名（api.github.com / api.cnb.cool）不会被匹配，仍直连。
const (
	proxyMirrorGithub = "https://dl-stats.dtapp.top/"
	proxyMirrorCNB    = "https://dl-stats.dtapp.net/"
)

// proxyDownloadURL 在原始下载地址前拼接对应的中转域名；未知源站原样返回（直连）。
// 形如 https://dl-stats.dtapp.top/https://github.com/owner/repo/releases/download/v1.0.0/file.zip
func proxyDownloadURL(raw string) string {
	switch {
	case strings.HasPrefix(raw, "https://github.com/"):
		return proxyMirrorGithub + raw
	case strings.HasPrefix(raw, "https://cnb.cool/"):
		return proxyMirrorCNB + raw
	default:
		return raw
	}
}

// isProxyURL 判断地址是否为经中转的地址（用于决定是否向中转服务泄露源站令牌）。
func isProxyURL(u string) bool {
	return strings.HasPrefix(u, proxyMirrorGithub) || strings.HasPrefix(u, proxyMirrorCNB)
}

// proxyThenDirect 返回“先中转、后直连”的地址顺序；无对应中转时仅直连。
func proxyThenDirect(raw string) []string {
	if p := proxyDownloadURL(raw); p != raw {
		return []string{p, raw}
	}
	return []string{raw}
}

// buildURL 用模板渲染下载地址：{tag} -> tag，{file} -> file。
func buildURL(tpl, tag, file string) string {
	u := strings.ReplaceAll(tpl, "{tag}", tag)
	u = strings.ReplaceAll(u, "{file}", file)
	return u
}

// downloadToTemp 把单个地址完整下载到独立临时文件，返回临时文件路径（所在临时目录由
// 调用方用 filepath.Dir 清理）与文件大小；任何失败返回错误。stripAuth 为 true 时不向
// 中转服务发送源站 Authorization 令牌，避免泄露给第三方中转。
func downloadToTemp(ctx context.Context, lg *slog.Logger, client *http.Client, url string, newReq func(ctx context.Context, url string) (*http.Request, error), stripAuth bool) (string, int64, error) {
	req, err := newReq(ctx, url)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", T("updater_err_download_request"), err)
	}
	if stripAuth {
		req.Header.Del("Authorization")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", T("updater_err_download_conn"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", 0, fmt.Errorf("%s", T("updater_err_download_failed", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	dir, err := os.MkdirTemp("", "kai-updater-proxy-*")
	if err != nil {
		return "", 0, fmt.Errorf("%s", T("updater_err_download_tmpdir"))
	}
	tmp := filepath.Join(dir, "artifact")
	f, err := os.Create(tmp)
	if err != nil {
		os.RemoveAll(dir)
		return "", 0, fmt.Errorf("%s", T("updater_err_download_tmpfile"))
	}
	size, err := io.Copy(f, resp.Body)
	if err != nil {
		f.Close()
		os.RemoveAll(dir)
		return "", 0, fmt.Errorf("%s: %w", T("updater_err_download_io"), err)
	}
	if err := f.Close(); err != nil {
		os.RemoveAll(dir)
		return "", 0, fmt.Errorf("%s: %w", T("updater_err_download_io"), err)
	}
	return tmp, size, nil
}

// downloadBytes 把单个地址完整下载到内存并返回内容；任何失败返回错误。
// stripAuth 语义同 downloadToTemp。
func downloadBytes(ctx context.Context, lg *slog.Logger, client *http.Client, url string, newReq func(ctx context.Context, url string) (*http.Request, error), stripAuth bool) ([]byte, error) {
	req, err := newReq(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", T("updater_err_download_request"), err)
	}
	if stripAuth {
		req.Header.Del("Authorization")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", T("updater_err_download_conn"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%s", T("updater_err_download_failed", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	return io.ReadAll(resp.Body)
}

// downloadRelease 共用下载逻辑：下载 release 的升级产物到 dst，通过 onProgress 回报进度。
// 优先使用 directURL（资源真实下载地址，如 CNB 的 browser_download_url）；
// directURL 为空时回退到 downloadURLTpl + repo + version + filename 的模板拼接（GitHub 路径）。
// 下载优先走中转（dl-stats.dtapp.top / dl-stats.dtapp.net，计数/加速），任一候选成功即采用，
// 全部失败才返回错误。为避免“中转失败回退直连”时污染 wails 的 sha256 校验写入链，每个候选
// 先完整下载到独立临时文件，成功后再拷入 dst（驱动校验与进度）；一旦开始写入 dst 便不再回退。
func downloadRelease(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, dst io.Writer, onProgress func(written, total int64), directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) error {
	filename := rel.Artifact.Filename
	if filename == "" {
		return fmt.Errorf("%s", T("updater_err_artifact_filename_empty"))
	}
	rawURL := directURL
	if rawURL == "" {
		rawURL = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}

	var lastErr error
	for _, u := range proxyThenDirect(rawURL) {
		tmp, size, err := downloadToTemp(ctx, lg, client, u, newReq, isProxyURL(u))
		if err != nil {
			lg.Warn(T("updater_download_proxy_fallback", "URL", u, "Error", err.Error()))
			lastErr = err
			continue // 仅网络/中转失败：dst 尚未写入，安全尝试下一候选
		}
		// 成功下载：把临时文件拷入 wails 的 dst（驱动 sha256 校验与进度），随后清理临时目录。
		if onProgress != nil {
			onProgress(0, size)
		}
		f, oerr := os.Open(tmp)
		if oerr != nil {
			os.RemoveAll(filepath.Dir(tmp))
			return fmt.Errorf("%s: %w", T("updater_err_download_io"), oerr)
		}
		_, cerr := io.Copy(dst, f)
		f.Close()
		os.RemoveAll(filepath.Dir(tmp))
		if cerr != nil {
			lg.Warn(T("updater_download_proxy_fallback", "URL", u, "Error", cerr.Error()))
			return cerr // dst 已写入部分字节且 hasher 不可重置，无法安全回退，直接报错
		}
		lg.Debug(T("updater_download_done", "URL", u, "Size", size))
		return nil
	}
	return lastErr
}

// fetchReleaseChecksum 共用校验和获取逻辑：直连下载 SHA256SUMS 侧车（不走下载中转），
// 解析出目标文件的哈希。返回 (哈希字节, 是否找到)。directURL 非空时优先作为侧车真实地址，
// 否则回退模板拼接。注意：校验和等元信息文件不参与中转计数，仅升级产物二进制走中转。
func fetchReleaseChecksum(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, sidecar, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) ([]byte, bool) {
	rawURL := directURL
	if rawURL == "" {
		rawURL = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, sidecar)
	}
	body, err := downloadBytes(ctx, lg, client, rawURL, newReq, false)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", rawURL, "Error", err.Error()))
		lg.Warn(T("updater_checksum_fetch_failed"))
		return nil, false
	}
	target := rel.Artifact.Filename
	for line := range strings.SplitSeq(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := fields[0]
		name := strings.TrimSpace(strings.TrimPrefix(line, hash))
		if strings.Contains(name, " ") {
			name = strings.TrimSpace(strings.SplitN(name, " ", 2)[1])
		}
		if name == target {
			if !sha256Re.MatchString(hash) {
				lg.Warn(T("updater_checksum_invalid", "URL", rawURL, "File", target, "Hash", hash))
				return nil, false
			}
			raw, err := hex.DecodeString(hash)
			if err != nil {
				lg.Warn(T("updater_checksum_invalid", "URL", rawURL, "File", target, "Hash", hash))
				return nil, false
			}
			return raw, true
		}
	}
	lg.Warn(T("updater_checksum_parse_failed", "URL", rawURL, "Target", target))
	return nil, false
}

// fetchGitCommitFile 直连下载预发布附带的 git commit 文件（GIT_COMMIT，不走下载中转），
// 校验为单行十六进制 hash 后返回。文件不存在、下载失败或内容非法时返回 ("", false)。
func fetchGitCommitFile(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, filename, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) (string, bool) {
	rawURL := directURL
	if rawURL == "" {
		rawURL = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}
	body, err := downloadBytes(ctx, lg, client, rawURL, newReq, false)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", rawURL, "Error", err.Error()))
		return "", false
	}
	commit := strings.TrimSpace(string(body))
	if !gitCommitRe.MatchString(commit) {
		lg.Warn(T("updater_gitcommit_invalid", "URL", rawURL, "Content", commit))
		return "", false
	}
	return commit, true
}

// fetchBuildTimeFile 直连下载预发布附带的构建时间文件（BUILD_TIME，不走下载中转），
// 解析为 RFC3339 时间后返回。文件不存在、下载失败或解析失败时返回 (time.Time{}, false)。
func fetchBuildTimeFile(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, filename, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) (time.Time, bool) {
	rawURL := directURL
	if rawURL == "" {
		rawURL = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}
	body, err := downloadBytes(ctx, lg, client, rawURL, newReq, false)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", rawURL, "Error", err.Error()))
		return time.Time{}, false
	}
	raw := strings.TrimSpace(string(body))
	parsed, perr := time.Parse(time.RFC3339, raw)
	if perr != nil {
		lg.Warn(T("updater_buildtime_parse_failed", "URL", rawURL, "Content", raw, "Error", perr.Error()))
		return time.Time{}, false
	}
	return parsed, true
}

// isNewer 基于版本字符串比较：remote 与 current 不同则视为有更新。
func isNewer(remote, current string) bool {
	if remote == "" {
		return false
	}
	if current == "" {
		return true
	}
	return remote != current
}
