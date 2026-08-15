package wails_updater_providers

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

// buildURL 用模板渲染下载地址：{tag} -> tag，{file} -> file。
func buildURL(tpl, tag, file string) string {
	u := strings.ReplaceAll(tpl, "{tag}", tag)
	u = strings.ReplaceAll(u, "{file}", file)
	return u
}

// downloadRelease 共用下载逻辑：下载 release 的升级产物到 dst，通过 onProgress 回报进度。
// 优先使用 directURL（资源真实下载地址，如 CNB 的 browser_download_url）；
// directURL 为空时回退到 downloadURLTpl + repo + version + filename 的模板拼接（GitHub 路径）。
// 请求由调用方通过 newReq 在「发起前」构造好（CNB 在此带上 Accept 等头，GitHub 用普通 GET），
// 本函数只负责执行请求、读 body 与回报进度，不直接 new 请求。
func downloadRelease(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, dst io.Writer, onProgress func(written, total int64), directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) error {
	filename := rel.Artifact.Filename
	if filename == "" {
		return fmt.Errorf("%s", T("updater_err_artifact_filename_empty"))
	}
	url := directURL
	if url == "" {
		url = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}

	req, err := newReq(ctx, url)
	if err != nil {
		return fmt.Errorf("%s: %w", T("updater_err_download_request"), err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", T("updater_err_download_conn"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if rerr != nil {
			lg.Warn(T("updater_err_download_read_body", "Err", rerr.Error()))
		}
		return fmt.Errorf("%s", T("updater_err_download_failed", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}

	total := resp.ContentLength
	var written int64
	reader := bufio.NewReader(resp.Body)
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			lg.Debug(T("updater_download_canceled"), "url", url)
			return ctx.Err()
		default:
		}
		n, rerr := reader.Read(buf)
		if n > 0 {
			wn, werr := dst.Write(buf[:n])
			written += int64(wn)
			if werr != nil {
				return fmt.Errorf("%s: %w", T("updater_err_download_io"), werr)
			}
			if onProgress != nil {
				onProgress(written, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("%s: %w", T("updater_err_download_io"), rerr)
		}
	}
	return nil
}

// fetchReleaseChecksum 共用校验和获取逻辑：下载 SHA256SUMS 侧车，解析出
// 目标文件的哈希。返回 (哈希字节, 是否找到)。directURL 非空时优先作为侧车真实地址，
// 否则回退模板拼接。
func fetchReleaseChecksum(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, sidecar, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) ([]byte, bool) {
	checksumURL := directURL
	if checksumURL == "" {
		checksumURL = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, sidecar)
	}
	lg.Debug(T("updater_checksum_download", "URL", checksumURL))

	req, err := newReq(ctx, checksumURL)
	if err != nil {
		lg.Warn(T("updater_checksum_fetch_failed"), "error", err)
		return nil, false
	}
	resp, err := client.Do(req)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", checksumURL, "Error", err.Error()))
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		lg.Warn(T("updater_checksum_no_url", "Tag", rel.Version))
		return nil, false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		lg.Warn(T("updater_checksum_fetch_failed"), "error", err)
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
				lg.Warn(T("updater_checksum_invalid", "URL", checksumURL, "File", target, "Hash", hash))
				return nil, false
			}
			raw, err := hex.DecodeString(hash)
			if err != nil {
				lg.Warn(T("updater_checksum_invalid", "URL", checksumURL, "File", target, "Hash", hash))
				return nil, false
			}
			return raw, true
		}
	}

	lg.Warn(T("updater_checksum_parse_failed", "URL", checksumURL, "Target", target))
	return nil, false
}

// fetchGitCommitFile 下载预发布附带的 git commit 文件（GIT_COMMIT），
// 校验为单行 40 位十六进制 hash 后返回。文件不存在、下载失败或内容非法时返回 ("", false)。
func fetchGitCommitFile(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, filename, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) (string, bool) {
	url := directURL
	if url == "" {
		url = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}
	lg.Debug(T("updater_gitcommit_download", "URL", url))

	req, err := newReq(ctx, url)
	if err != nil {
		lg.Warn(T("updater_gitcommit_fetch_failed"), "error", err)
		return "", false
	}
	resp, err := client.Do(req)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", url, "Error", err.Error()))
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		lg.Warn(T("updater_gitcommit_no_url", "Tag", rel.Version, "File", filename))
		return "", false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		lg.Warn(T("updater_gitcommit_fetch_failed"), "error", err)
		return "", false
	}
	commit := strings.TrimSpace(string(body))
	if !gitCommitRe.MatchString(commit) {
		lg.Warn(T("updater_gitcommit_invalid", "URL", url, "Content", commit))
		return "", false
	}
	return commit, true
}

// fetchBuildTimeFile 下载预发布附带的构建时间文件（BUILD_TIME），
// 解析为 RFC3339 时间后返回。文件不存在、下载失败或解析失败时返回 (time.Time{}, false)。
func fetchBuildTimeFile(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, filename, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) (time.Time, bool) {
	url := directURL
	if url == "" {
		url = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}
	lg.Debug(T("updater_buildtime_download", "URL", url))

	req, err := newReq(ctx, url)
	if err != nil {
		lg.Warn(T("updater_buildtime_fetch_failed"), "error", err)
		return time.Time{}, false
	}
	resp, err := client.Do(req)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", url, "Error", err.Error()))
		return time.Time{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		lg.Warn(T("updater_buildtime_no_url", "Tag", rel.Version, "File", filename))
		return time.Time{}, false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		lg.Warn(T("updater_buildtime_fetch_failed"), "error", err)
		return time.Time{}, false
	}
	raw := strings.TrimSpace(string(body))
	parsed, perr := time.Parse(time.RFC3339, raw)
	if perr != nil {
		lg.Warn(T("updater_buildtime_parse_failed", "URL", url, "Content", raw, "Error", perr.Error()))
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
