// Package buildinfo 暴露可通过 -ldflags -X 覆盖的构建期变量，并负责运行期目录规则与模式判断。
//
// 所有构建期注入变量（版本号 / 构建时间 / 开发标志 / 升级 token）集中在本包，
// 由 CI 通过 -ldflags -X cnb.cool/dtapp/kai/internal/buildinfo.xxx 注入，避免分散到 main 包。
// 应用数据目录（数据根 / 数据库子目录 / 日志子目录）的拼接规则也集中在本包，
// 由 DataDir / DBDir / LogDir 提供，main 及其余代码只调用、不自行拼接。
package buildinfo

import "path/filepath"

// 以下变量由打包时注入：
//
//	-X cnb.cool/dtapp/kai/internal/buildinfo.Version=1.0.0
//	-X cnb.cool/dtapp/kai/internal/buildinfo.BuildTime=2026-08-06T12:00:00Z
//	-X cnb.cool/dtapp/kai/internal/buildinfo.Dev=false
//	-X cnb.cool/dtapp/kai/internal/buildinfo.GithubToken=xxx
//	-X cnb.cool/dtapp/kai/internal/buildinfo.CnbToken=xxx
var (
	Version   = "dev"
	BuildTime = "unknown"
	// Dev 标志："true" 走 ~/.kai.dev/，"false" 走 ~/.kai/。
	Dev = "true"
	// 升级相关 token（GitHub 源 / CNB 镜像源）。本地 dev 未注入则为空，不影响运行。
	GithubToken = ""
	CnbToken    = ""
	// GitCommit 构建时注入的提交哈希（CI 通过 -ldflags 注入，本地为空）。
	GitCommit = ""
)

// IsDev 是否为开发模式
func IsDev() bool { return Dev == "true" || Dev == "1" }

// DataHome 返回数据根目录名（按模式切换，不含 home 前缀）
func DataHome() string {
	if IsDev() {
		return ".kai.dev"
	}
	return ".kai"
}

// DataDir 返回应用数据根目录：~/{.kai|.kai.dev}
func DataDir(homeDir string) string {
	return filepath.Join(homeDir, DataHome())
}

// DBDir 返回数据库存放目录：DataDir 下的 data/ 子目录（config.db / history.db / httplog.db）
func DBDir(homeDir string) string {
	return filepath.Join(DataDir(homeDir), "data")
}

// LogDir 返回日志存放目录：DataDir 下的 logs/ 子目录（kai.log / frontend.log / kai-bridge.log）
func LogDir(homeDir string) string {
	return filepath.Join(DataDir(homeDir), "logs")
}
