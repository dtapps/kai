package wails_updater_providers

import (
	"bytes"
	"text/template"
)

// 内建多语言表：locale -> key -> 模板（模板支持 {{.X}} 占位符）。
// 不依赖外部 JSON 或项目 i18n，第三方库自包含。
var i18nMessages = map[Locale]map[string]string{
	LocaleZhCN: { //nolint:gosec // 此处含 "token" 字样仅为 i18n 文案，非真实凭证
		"updater_check_done":                       "检查更新完成",
		"updater_check_nightly_channel":            "已订阅 nightly 渠道，预发布与稳定版一起参与比较",
		"updater_prerelease_no_update":             "预发布与稳定版检查均出错，按官方契约返回错误",
		"updater_prerelease_up_to_date":            "开启预发布：预发布与稳定版均已是最新",
		"updater_pick_newer":                       "开启预发布：预发布与稳定版均可用，选取发布时间更晚的更新候选",
		"updater_nightly_no_prerelease":            "未找到预发布版本",
		"updater_check_failed":                     "检查更新失败",
		"updater_check_panic":                      "检查更新时发生未捕获异常",
		"updater_checksum_download":                "正在下载校验和：{{.URL}}",
		"updater_checksum_fetch_failed":            "获取校验和文件失败",
		"updater_checksum_no_url":                  "{{.Tag}} 无校验和地址",
		"updater_checksum_parse_failed":            "解析校验和 {{.URL}} 失败（目标 {{.Target}}）",
		"updater_checksum_invalid":                 "校验和文件内容非法：{{.File}} 对应哈希非 64 位十六进制（{{.Hash}}）",
		"updater_checksum_source_failed":           "校验和源 {{.URL}} 失败：{{.Error}}",
		"updater_gitcommit_download":               "正在下载 git commit 文件：{{.URL}}",
		"updater_gitcommit_fetch_failed":           "获取 git commit 文件失败",
		"updater_gitcommit_no_url":                 "{{.Tag}} 无 git commit 文件地址",
		"updater_gitcommit_invalid":                "git commit 文件内容非法：应为 7~40 位十六进制 hash，实际 {{.Content}}",
		"updater_buildtime_download":               "正在下载构建时间文件：{{.URL}}",
		"updater_buildtime_fetch_failed":           "获取构建时间文件失败",
		"updater_buildtime_no_url":                 "{{.Tag}} 无构建时间文件地址",
		"updater_buildtime_parse_failed":           "构建时间文件解析失败：内容 {{.Content}}，错误 {{.Error}}",
		"updater_warn_api_error":                   "API 错误：状态码 {{.Status}}，响应 {{.Body}}",
		"updater_warn_unauthorized":                "token 未授权",
		"updater_warn_no_tags":                     "列举 releases 失败",
		"updater_warn_get_release_failed":          "获取 release {{.Tag}} 失败",
		"updater_download_all_failed":              "所有源均下载失败",
		"updater_download_size_mismatch":           "大小不一致：期望 {{.Expected}}，实际 {{.Actual}}",
		"updater_fetch_dockerhub":                  "正在获取 Docker Hub 镜像信息",
		"updater_fetch_failed":                     "获取 {{.URL}} 失败：{{.Err}}",
		"updater_fetch_mirrors":                    "已从远端获取 {{.MirrorCount}} 个镜像",
		"updater_fetch_npm_mirror":                 "正在获取 npm 镜像信息",
		"updater_http_status":                      "非预期 HTTP 状态码：{{.Status}}",
		"updater_init_failed":                      "初始化更新器失败",
		"updater_init_both_repo_empty":             "CNB 与 GitHub 仓库均不可为空，至少需配置一个",
		"updater_init_repo_empty":                  "已选择 {{.Source}} 源，但对应仓库为空",
		"updater_init_token_empty":                 "已选择 {{.Source}} 源，但对应 token 为空",
		"updater_init_unknown_source":              "未知的更新源：{{.Source}}",
		"updater_source_auto_selected":             "自动选择更新源：{{.Source}}",
		"updater_init_no_source":                   "未配置任何更新源",
		"updater_err_unauthorized":                 "token 未授权",
		"updater_err_api":                          "API 错误：状态码 {{.Status}}，响应 {{.Body}}",
		"updater_err_github_latest_not_prerelease": "最新版本不是预发布",
		"updater_err_nightly_no_published_at":      "Release 无 published_at",
		"updater_err_local_build_time_empty":       "本地构建无构建时间",
		"updater_err_nightly_not_newer":            "nightly 远端发布时间不新于本地",
		"updater_err_no_matching_nightly_asset":    "未找到匹配的 nightly 资源",
		"updater_err_nightly_checksum_unavailable": "nightly 校验和不可用",
		"updater_err_no_stable_release":            "未找到稳定版 release",
		"updater_err_no_matching_stable_asset":     "未找到匹配的稳定版资源",
		"updater_err_stable_checksum_unavailable":  "稳定版校验和不可用",
		"updater_err_no_token":                     "未配置 token",
		"updater_err_no_tags":                      "无可用 tag",
		"updater_err_get_release_failed":           "获取 release 失败：状态码 {{.Status}}",
		"updater_err_no_stable_tags":               "无可用稳定版 tag",
		"updater_err_no_stable_matched":            "未匹配到稳定版",
		"updater_err_artifact_filename_empty":      "升级产物文件名为空",
		"updater_err_request_create":               "创建 HTTP 请求失败：{{.Err}}",
		"updater_err_request_failed":               "请求远端失败：{{.Err}}",
		"updater_err_decode":                       "解析响应失败：{{.Err}}",
		"updater_err_build_release":                "构造发布信息失败：{{.Err}}",
		"updater_err_download_conn":                "连接下载地址失败：{{.Err}}",
		"updater_err_download_io":                  "下载读写失败：{{.Err}}",
		"updater_err_download_read_body":           "读取错误响应体失败：{{.Err}}",
		"updater_download_canceled":                "下载被取消",
		"updater_err_download_request":             "创建下载请求失败",
		"updater_err_download_failed":              "下载失败：状态码 {{.Status}}，响应 {{.Body}}",
		"updater_err_release_tag_empty":            "release tag 为空",
		"updater_err_nightly_tag_empty":            "nightly tag 为空",
		"updater_err_nightly_missing_published_at": "nightly release 缺少 published_at",
		"updater_err_nightly_same_commit":          "nightly 远端提交 {{.Commit}} 与本机一致，跳过更新",
		"updater_err_prerelease_meta_missing":      "预发布版本缺少 git commit / build time 附带文件",
		"updater_matcher_start":                    "资源匹配开始（平台：{{.Plat}}，架构：{{.Arch}}，候选数：{{.Count}}）",
		"updater_matcher_check":                    "检查候选 #{{.Index}}：{{.Name}}",
		"updater_matcher_hit":                      "命中升级资源 #{{.Index}}：{{.Name}}",
		"updater_matcher_none":                     "未匹配到任何升级资源",
		"updater_matcher_skip_not_updater":         "跳过：非 updater- 升级专用文件",
		"updater_matcher_skip_sig":                 "跳过：签名/校验文件（.sig/.asc/.zsync）",
		"updater_matcher_skip_format":              "跳过：非压缩归档（需 .zip/.tar.gz/.tgz）",
		"updater_matcher_skip_plat":                "跳过：平台不匹配（需 {{.Plat}}）",
		"updater_matcher_skip_arch":                "跳过：架构不匹配（需 {{.Arch}}）",
		"updater_nightly_no_token":                 "未配置 token，跳过 nightly",
		"updater_nightly_unauthorized":             "token 未授权",
		"updater_nightly_unreachable":              "API 不可达：{{.Error}}",
		"updater_nightly_failed":                   "nightly 更新失败：{{.Error}}",
		"updater_nightly_no_asset":                 "未找到匹配 {{.Tag}} 的资源（{{.Platform}}/{{.Arch}}）",
		"updater_nightly_ready":                    "nightly 更新就绪：{{.Tag}}（{{.Platform}}/{{.Arch}}）",
		"updater_nightly_no_build_time":            "Release 无 published_at，跳过",
		"updater_nightly_no_local_build_time":      "本地构建无构建时间，跳过",
		"updater_nightly_build_skipped":            "nightly 发布构造失败，跳过",
		"updater_nightly_same_commit":              "nightly 远端提交（{{.Commit}}）与本机一致，同一份代码，跳过更新",
		"updater_nightly_skipped":                  "nightly 跳过：发布时间 {{.PublishedAt}} <= 本地 {{.BuildTime}}",
		"updater_no_mirror":                        "远端未返回镜像，跳过本轮",
		"updater_ready":                            "更新已就绪，重启后生效",
		"updater_release_missing_metadata":         "Release 缺少必要元数据",
		"updater_run_err":                          "镜像更新器运行出错",
		"updater_save_failed":                      "镜像信息保存失败",
		"updater_save_mirror":                      "正在将镜像信息写入数据库",
		"updater_save_succeed":                     "镜像信息保存成功",
		"updater_slow_api":                         "接口响应过慢：{{.URL}} 耗时 {{.Seconds}} 秒",
		"updater_source_download":                  "正在从 {{.Source}} 下载：{{.URL}}",
		"updater_source_failed":                    "源 {{.Source}} 下载失败：{{.Error}}",
		"updater_start":                            "镜像更新器已启动",
		"updater_stable_no_asset":                  "未找到匹配稳定版 {{.Tag}} 的资源（{{.Platform}}/{{.Arch}}）",
		"updater_stable_not_newer":                 "{{.Tag}} 不比当前版本新，跳过",
		"updater_stable_no_matching_skip":          "稳定版候选 {{.Tag}} 无匹配资源，跳过",
		"updater_stable_build_skipped":             "稳定版 {{.Tag}} 构造失败，跳过",
		"updater_stable_req_failed":                "稳定版 {{.Tag}} 请求发布详情失败：{{.Error}}",
		"updater_stable_decode_failed":             "稳定版 {{.Tag}} 解析发布详情失败：{{.Error}}",
		"updater_stable_time_parse_failed":         "稳定版 {{.Tag}} 解析发布时间失败：{{.Error}}",
		"updater_stable_ready":                     "稳定版更新就绪：{{.Tag}} -> {{.Asset}}",
		"updater_ticker":                           "镜像更新器已定时",
		"updater_using_default_mirror":             "使用内置默认镜像列表",
		"window_title_check":                       "检查更新",
		"window_checking":                          "正在检查更新…",
		"window_downloading":                       "正在下载更新…",
		"window_installing":                        "正在安装更新…",
		"window_success":                           "更新成功",
		"window_restart":                           "重启应用",
		"window_close":                             "关闭",
		"window_error":                             "更新出错",
		"window_current":                           "当前版本",
		"window_new_version":                       "新版本",
		"window_release_notes":                     "更新说明",
		"window_notes_empty":                       "暂无更新说明",
		"window_contacting":                        "正在连接更新服务器…",
		"window_checking_title":                    "正在检查更新…",
		"window_update_available":                  "发现新版本",
		"window_up_to_date":                        "已是最新版本",
		"window_downloading_title":                 "正在下载更新",
		"window_download_starting":                 "开始下载…",
		"window_downloaded":                        "已下载",
		"window_verifying":                         "正在校验更新",
		"window_checking_signature":                "正在校验签名…",
		"window_installing_title":                  "正在安装更新",
		"window_unpacking":                         "正在解包并准备…",
		"window_update_ready":                      "更新已就绪",
		"window_update_failed":                     "更新失败",
		"window_error_default":                     "发生未知错误",
		"window_stage_prefix":                      "在",
		"window_stage_suffix":                      " 阶段",
		"window_skip_version":                      "跳过此版本",
		"window_remind_later":                      "稍后提醒我",
		"window_install_update":                    "安装更新",
		"window_try_again":                         "重试",
	},
	LocaleEnUS: { //nolint:gosec // 此处含 "token" 字样仅为 i18n 文案，非真实凭证
		"updater_check_done":                       "update check done",
		"updater_check_nightly_channel":            "subscribed to nightly channel, comparing prerelease and stable together",
		"updater_prerelease_no_update":             "both prerelease and stable checks failed, returning error per provider contract",
		"updater_prerelease_up_to_date":            "prerelease enabled: both prerelease and stable are up-to-date",
		"updater_pick_newer":                       "prerelease enabled: both prerelease and stable available, picking the candidate with later publishedAt",
		"updater_nightly_no_prerelease":            "no prerelease found",
		"updater_check_failed":                     "update check failed",
		"updater_check_panic":                      "uncaught panic during update check",
		"updater_checksum_download":                "Downloading checksum from {{.URL}}",
		"updater_checksum_fetch_failed":            "Failed to fetch checksum file",
		"updater_checksum_no_url":                  "No checksum URL for {{.Tag}}",
		"updater_checksum_parse_failed":            "Failed to parse checksum {{.URL}} (target {{.Target}})",
		"updater_checksum_invalid":                 "invalid checksum file: hash for {{.File}} is not a 64-char hex string ({{.Hash}})",
		"updater_checksum_source_failed":           "Checksum source {{.URL}} failed: {{.Error}}",
		"updater_gitcommit_download":               "Downloading git commit file from {{.URL}}",
		"updater_gitcommit_fetch_failed":           "Failed to fetch git commit file",
		"updater_gitcommit_no_url":                 "No git commit file URL for {{.Tag}}",
		"updater_gitcommit_invalid":                "invalid git commit file content: expected 7-40 char hex hash, got {{.Content}}",
		"updater_buildtime_download":               "Downloading build time file from {{.URL}}",
		"updater_buildtime_fetch_failed":           "Failed to fetch build time file",
		"updater_buildtime_no_url":                 "No build time file URL for {{.Tag}}",
		"updater_buildtime_parse_failed":           "failed to parse build time file: content {{.Content}}, error {{.Error}}",
		"updater_warn_api_error":                   "API error: status {{.Status}}, body {{.Body}}",
		"updater_warn_unauthorized":                "token unauthorized",
		"updater_warn_no_tags":                     "failed to list releases",
		"updater_warn_get_release_failed":          "failed to get release {{.Tag}}",
		"updater_download_all_failed":              "All sources failed to download",
		"updater_download_size_mismatch":           "Size mismatch: expected {{.Expected}}, got {{.Actual}}",
		"updater_fetch_dockerhub":                  "Fetching Docker Hub mirror info",
		"updater_fetch_failed":                     "Failed to fetch {{.URL}}: {{.Err}}",
		"updater_fetch_mirrors":                    "Fetched {{.MirrorCount}} mirrors from remote",
		"updater_fetch_npm_mirror":                 "Fetching npm mirror info",
		"updater_http_status":                      "Unexpected HTTP status: {{.Status}}",
		"updater_init_failed":                      "failed to init updater",
		"updater_init_both_repo_empty":             "CnbRepo and GithubRepo cannot both be empty, at least one must be set",
		"updater_init_repo_empty":                  "source {{.Source}} selected but its repo is empty",
		"updater_init_token_empty":                 "source {{.Source}} selected but its token is empty",
		"updater_init_unknown_source":              "unknown updater source: {{.Source}}",
		"updater_source_auto_selected":             "auto selected updater source: {{.Source}}",
		"updater_init_no_source":                   "no updater source configured",
		"updater_err_unauthorized":                 "token unauthorized",
		"updater_err_api":                          "API error: status {{.Status}}, body {{.Body}}",
		"updater_err_github_latest_not_prerelease": "latest release is not a prerelease",
		"updater_err_nightly_no_published_at":      "Release has no published_at",
		"updater_err_local_build_time_empty":       "local build has no build time",
		"updater_err_nightly_not_newer":            "nightly remote publish time is not newer than local",
		"updater_err_no_matching_nightly_asset":    "no matching nightly asset",
		"updater_err_nightly_checksum_unavailable": "nightly checksum unavailable",
		"updater_err_no_stable_release":            "no stable release found",
		"updater_err_no_matching_stable_asset":     "no matching stable asset",
		"updater_err_stable_checksum_unavailable":  "stable checksum unavailable",
		"updater_err_no_token":                     "no token configured",
		"updater_err_no_tags":                      "no available tags",
		"updater_err_get_release_failed":           "failed to get release: status {{.Status}}",
		"updater_err_no_stable_tags":               "no available stable tags",
		"updater_err_no_stable_matched":            "matched no stable release",
		"updater_err_artifact_filename_empty":      "release artifact filename is empty",
		"updater_err_request_create":               "failed to create HTTP request: {{.Err}}",
		"updater_err_request_failed":               "request to remote failed: {{.Err}}",
		"updater_err_decode":                       "failed to decode response: {{.Err}}",
		"updater_err_build_release":                "failed to build release info: {{.Err}}",
		"updater_err_download_conn":                "failed to connect download URL: {{.Err}}",
		"updater_err_download_io":                  "download read/write failed: {{.Err}}",
		"updater_err_download_read_body":           "failed to read error response body: {{.Err}}",
		"updater_download_canceled":                "download canceled",
		"updater_err_download_request":             "failed to create download request",
		"updater_err_download_failed":              "download failed: status {{.Status}}, body {{.Body}}",
		"updater_err_release_tag_empty":            "release tag is empty",
		"updater_err_nightly_tag_empty":            "nightly tag is empty",
		"updater_err_nightly_missing_published_at": "nightly release missing published_at",
		"updater_err_nightly_same_commit":          "nightly remote commit {{.Commit}} matches local, update skipped",
		"updater_err_prerelease_meta_missing":      "prerelease missing git commit / build time sidecar file",
		"updater_matcher_start":                    "asset matching start (platform: {{.Plat}}, arch: {{.Arch}}, count: {{.Count}})",
		"updater_matcher_check":                    "checking candidate #{{.Index}}: {{.Name}}",
		"updater_matcher_hit":                      "hit update asset #{{.Index}}: {{.Name}}",
		"updater_matcher_none":                     "no update asset matched",
		"updater_matcher_skip_not_updater":         "skip: not an updater- dedicated asset",
		"updater_matcher_skip_sig":                 "skip: signature/checksum file (.sig/.asc/.zsync)",
		"updater_matcher_skip_format":              "skip: not a compressed archive (.zip/.tar.gz/.tgz required)",
		"updater_matcher_skip_plat":                "skip: platform mismatch (need {{.Plat}})",
		"updater_matcher_skip_arch":                "skip: arch mismatch (need {{.Arch}})",
		"updater_nightly_no_token":                 "No token configured, skip nightly",
		"updater_nightly_unauthorized":             "token unauthorized",
		"updater_nightly_unreachable":              "API unreachable: {{.Error}}",
		"updater_nightly_failed":                   "Nightly update failed: {{.Error}}",
		"updater_nightly_no_asset":                 "No matching asset for {{.Tag}} ({{.Platform}}/{{.Arch}})",
		"updater_nightly_ready":                    "nightly update ready: {{.Tag}} ({{.Platform}}/{{.Arch}})",
		"updater_nightly_no_build_time":            "Release has no published_at, skip",
		"updater_nightly_no_local_build_time":      "Local build has no build time, skip",
		"updater_nightly_build_skipped":            "nightly release build failed, skip",
		"updater_nightly_same_commit":              "nightly remote commit ({{.Commit}}) matches local, same build, update skipped",
		"updater_nightly_skipped":                  "Nightly skipped: published {{.PublishedAt}} <= local {{.BuildTime}}",
		"updater_no_mirror":                        "No mirror returned from remote, skip this round",
		"updater_ready":                            "update ready, takes effect after restart",
		"updater_release_missing_metadata":         "Release missing required metadata",
		"updater_run_err":                          "Mirror updater run error",
		"updater_save_failed":                      "Failed to save mirror info",
		"updater_save_mirror":                      "Saving mirror info to database",
		"updater_save_succeed":                     "Mirror info saved successfully",
		"updater_slow_api":                         "API too slow: {{.URL}} took {{.Seconds}}s",
		"updater_source_download":                  "Downloading from {{.Source}}: {{.URL}}",
		"updater_source_failed":                    "Source {{.Source}} failed: {{.Error}}",
		"updater_start":                            "Mirror updater started",
		"updater_stable_no_asset":                  "No matching stable asset for {{.Tag}} ({{.Platform}}/{{.Arch}})",
		"updater_stable_not_newer":                 "{{.Tag}} is not newer than current, skip",
		"updater_stable_no_matching_skip":          "stable candidate {{.Tag}} has no matching asset, skip",
		"updater_stable_build_skipped":             "stable {{.Tag}} build failed, skip",
		"updater_stable_req_failed":                "stable {{.Tag}} request release detail failed: {{.Error}}",
		"updater_stable_decode_failed":             "stable {{.Tag}} decode release detail failed: {{.Error}}",
		"updater_stable_time_parse_failed":         "stable {{.Tag}} parse publish time failed: {{.Error}}",
		"updater_stable_ready":                     "Stable update ready: {{.Tag}} -> {{.Asset}}",
		"updater_ticker":                           "Mirror updater scheduled",
		"updater_using_default_mirror":             "Using default built-in mirror list",
		"window_title_check":                       "Check for Updates",
		"window_checking":                          "Checking for updates…",
		"window_downloading":                       "Downloading update…",
		"window_installing":                        "Installing update…",
		"window_success":                           "Update successful",
		"window_restart":                           "Restart",
		"window_close":                             "Close",
		"window_error":                             "Update failed",
		"window_current":                           "Current",
		"window_new_version":                       "New version",
		"window_release_notes":                     "Release notes",
		"window_notes_empty":                       "No release notes provided.",
		"window_contacting":                        "Contacting update server…",
		"window_checking_title":                    "Checking for Updates…",
		"window_update_available":                  "Update Available",
		"window_up_to_date":                        "You're Up to Date",
		"window_downloading_title":                 "Downloading Update",
		"window_download_starting":                 "Starting download…",
		"window_downloaded":                        "Downloaded",
		"window_verifying":                         "Verifying Update",
		"window_checking_signature":                "Checking signature…",
		"window_installing_title":                  "Installing Update",
		"window_unpacking":                         "Unpacking and staging…",
		"window_update_ready":                      "Update Ready",
		"window_update_failed":                     "Update Failed",
		"window_error_default":                     "An unexpected error occurred.",
		"window_stage_prefix":                      "During",
		"window_stage_suffix":                      "",
		"window_skip_version":                      "Skip This Version",
		"window_remind_later":                      "Remind Me Later",
		"window_install_update":                    "Install Update",
		"window_try_again":                         "Try Again",
	},
}

// localeAliases 归一化常见别名。
var localeAliases = map[string]string{
	"zh":      "zh-CN",
	"zh_cn":   "zh-CN",
	"zh-hans": "zh-CN",
	"en":      "en-US",
	"en_us":   "en-US",
	"english": "en-US",
	"chinese": "zh-CN",
}

// T 返回当前库全局 locale 下 key 的渲染文本。data 支持两种写法：
//  1. 单个 map[string]any / 结构体作为模板上下文（如 T(key, map[string]any{"Plat": p})）；
//  2. 平铺键值对（如 T(key, "Plat", p, "Arch", a)），自动组装为 map。
//
// locale 已全局化：T 内部直接读 GetLocale()，调用方无需再传。
// 找不到 key / locale 时回退到 en-US，再回退到原始 key，保证调用方永远拿到非空串。
func T(key string, data ...any) string {
	loc := GetLocale()
	table, ok := i18nMessages[loc]
	if !ok {
		table = i18nMessages[(LocaleEnUS)]
	}
	tmplStr, ok := table[key]
	if !ok {
		if t2, ok2 := i18nMessages[(LocaleEnUS)][key]; ok2 {
			tmplStr = t2
		} else {
			return key
		}
	}
	if len(data) == 0 {
		return tmplStr
	}
	ctx := dataToContext(data)
	tmpl, err := template.New(key).Parse(tmplStr)
	if err != nil {
		return tmplStr
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return tmplStr
	}
	return buf.String()
}

// dataToContext 把 T 的 data 参数规整为模板上下文：
//   - 单个 map[string]any / 结构体 → 原样作为上下文；
//   - 偶数个且全部为 string 的平铺键值对 → 组装为 map[string]any；
//   - 其他（如单个非 map 值，或单个 map 但类型不符）→ 原样透传（交由 text/template 决定）。
func dataToContext(data []any) any {
	if len(data) == 1 {
		if m, ok := data[0].(map[string]any); ok {
			return m
		}
		// 单个结构体或标量也直接作为上下文
		return data[0]
	}
	// 平铺键值对：键必须是 string，值可以是任意类型（int/string/error 等）。
	// 不再要求所有值为 string——否则一旦混入 int（如 len(x)）就会退化成只取
	// 首个元素，导致整个模板上下文错乱、占位符渲染失败回退到原始模板串。
	if len(data)%2 == 0 {
		m := make(map[string]any, len(data)/2)
		ok := true
		for i := 0; i < len(data); i += 2 {
			key, isStr := data[i].(string)
			if !isStr {
				ok = false
				break
			}
			m[key] = data[i+1]
		}
		if ok {
			return m
		}
	}
	// 退化：取第一个元素当上下文（兼容旧调用方）
	return data[0]
}
