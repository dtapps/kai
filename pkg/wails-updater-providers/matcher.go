package wails_updater_providers

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// 自定义资源匹配：仅匹配「升级专用文件」（文件名以 updater- 开头）。
// 打包/安装文件（Windows -install.exe、macOS .app.zip、Linux
// AppImage/deb/rpm/pkg.tar.zst）只用于首次安装，不用于自更新；
// 升级文件统一压缩（Windows/macOS 为 .zip，Linux 为 .tar.gz），内
// 含单一二进制，由 updater 下载、校验（SHA256SUMS）后替换自身。
// NewUpdaterAssetMatcher 构造「升级专用文件」匹配器。
// 注意：本函数直接读包全局语言（GetLocale），运行时调用方 SetLocale 即可
// 动态切换 matcher 日志语言，无需持有任何 Options 指针。
// 即「选择使用」——不传入 AssetMatcher 时 AssetMatcherOrDefault 回退官方
// github.DefaultAssetMatcher；注入本 matcher 后才跟随包全局语言。
func NewUpdaterAssetMatcher() github.AssetMatcher {
	lg := GetLogger()
	return func(req updater.CheckRequest, assets []github.ReleaseAsset) int {
		// 每次匹配读包全局语言当前值（T 内部直接读 GetLocale），实现动态语言监听。
		plat := strings.ToLower(req.Platform)
		arch := strings.ToLower(req.Arch)
		lg.Debug(T("updater_matcher_start", "Plat", plat, "Arch", arch, "Count", len(assets)))
		for i, a := range assets {
			name := strings.ToLower(a.Name)
			lg.Debug(T("updater_matcher_check", "Index", i, "Name", a.Name))
			// 仅升级专用文件（updater- 前缀）参与自更新
			if !strings.HasPrefix(name, "updater-") {
				lg.Debug(T("updater_matcher_skip_not_updater"))
				continue
			}
			if strings.HasSuffix(name, ".sig") || strings.HasSuffix(name, ".asc") || strings.HasSuffix(name, ".zsync") {
				lg.Debug(T("updater_matcher_skip_sig"))
				continue
			}
			// 升级文件必须是压缩归档（.zip / .tar.gz / .tgz）
			if !strings.HasSuffix(name, ".zip") &&
				!strings.HasSuffix(name, ".tar.gz") &&
				!strings.HasSuffix(name, ".tgz") {
				lg.Debug(T("updater_matcher_skip_format"))
				continue
			}
			if plat != "" && !strings.Contains(name, plat) {
				lg.Debug(T("updater_matcher_skip_plat", "Plat", plat))
				continue
			}
			if arch != "" && !strings.Contains(name, arch) {
				lg.Debug(T("updater_matcher_skip_arch", "Arch", arch))
				continue
			}
			lg.Debug(T("updater_matcher_hit", "Index", i, "Name", a.Name))
			return i
		}
		lg.Debug(T("updater_matcher_none"))
		return -1
	}
}
