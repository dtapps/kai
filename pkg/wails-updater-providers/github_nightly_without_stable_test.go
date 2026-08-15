package wails_updater_providers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// TestGitHubNightlyWithoutStableNeedsUpdate GitHub 源「需要更新」场景（走公开入口）：
// 已订阅 nightly 渠道（prerelease=true）且线上只有 nightly、没有稳定版时，Check 返回
// nightly 更新（nightly-x1b2c3）。
// 注意：GitHub 的 /releases/latest 只返回稳定版（不含预发布），nightly 由 checkNightly
// 走 /releases 列表筛选 Prerelease==true 获取。
func TestGitHubNightlyWithoutStableNeedsUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	// 发布列表：只有预发布 nightly，无稳定版（订阅 nightly 时由 checkPrerelease 筛选预发布）。
	mux.HandleFunc("/repos/"+testRepo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]githubRelease{
			{
				TagName:     "nightly-x1b2c3",
				Name:        "nightly build",
				Prerelease:  true,
				PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
				Assets: []githubAsset{
					{Name: "example-darwin-arm64.app.zip", Size: 12345},
					{Name: "updater-darwin-arm64.zip.sig", Size: 256},
					{Name: "SHA256SUMS", Size: 512},
					{Name: "GIT_COMMIT", Size: 41},
					{Name: "BUILD_TIME", Size: 28},
					{Name: "updater-darwin-arm64.zip", Size: 9988776},
				},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Format(time.RFC3339) + "\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:    testRepo,
		GithubToken:   "test-token",
		BuildTime:     now.Add(-72 * time.Hour),
		Prerelease:    true, // 订阅 nightly 渠道
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("NewMirrorProvider 构造失败: %v", err)
	}
	// 走 Check 编排（公开 Provider 接口）：已订阅 nightly 渠道、且线上无稳定版时，应返回 nightly 更新。
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[GitHub] 当前版本(currentVersion=%q, buildTime=%s), 需要更新=%v, 候选版本=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("已订阅 nightly 且线上无稳定版时 Check 应返回 nightly 更新，却失败: %v", err)
	}
	if rel.Version != "nightly-x1b2c3" {
		t.Fatalf("期望 nightly 版本 nightly-x1b2c3，实际 %s", rel.Version)
	}
	if rel.Artifact.Filename != "updater-darwin-arm64.zip" {
		t.Fatalf("期望选中 updater-darwin-arm64.zip，实际 %s", rel.Artifact.Filename)
	}
}

// TestGitHubNightlyWithoutStableNoUpdate GitHub 源「不需要更新」场景（走公开入口）：
// 已订阅 nightly 渠道，但本地 buildTime 晚于 nightly 发布时间（已是最新），Check 应返回 error。
func TestGitHubNightlyWithoutStableNoUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+testRepo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]githubRelease{
			{
				TagName:     "nightly-x1b2c3",
				Name:        "nightly build",
				Prerelease:  true,
				PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
				Assets: []githubAsset{
					{Name: "updater-darwin-arm64.zip", Size: 9988776},
					{Name: "SHA256SUMS", Size: 512},
					{Name: "GIT_COMMIT", Size: 41},
					{Name: "BUILD_TIME", Size: 28},
				},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Add(-1*time.Hour).Format(time.RFC3339) + "\n"))
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:    testRepo,
		GithubToken:   "test-token",
		BuildTime:     now.Add(1 * time.Hour), // 比 nightly 新，已是最新
		Prerelease:    true,
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("NewMirrorProvider 构造失败: %v", err)
	}
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[GitHub] 当前版本(currentVersion=%q, buildTime=%s), 需要更新=%v, 候选版本=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("nightly 已是最新时 Check 应返回 nil,nil 表示 up-to-date，却报错: %v", err)
	}
	if rel != nil {
		t.Fatalf("已是最新时不应返回 release，却返回 %s", rel.Version)
	}
}
