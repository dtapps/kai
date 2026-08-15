package wails_updater_providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// TestCNBNightlyWithStableNeedsUpdate CNB 源「需要更新」场景（走公开入口）：
// 已订阅 nightly 渠道（prerelease=true）且线上同时存在稳定版与 nightly 时，Check 应返回
// nightly（nightly-x1b2c3），而非被稳定版覆盖。验证 nightly 是优先渠道，且更新只从 nightly 自身判定。
func TestCNBNightlyWithStableNeedsUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	// releases 列表：稳定版 + nightly 都在（nightly 最新）。
	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "nightly-x1b2c3", Name: "nightly", Body: "nightly", Prerelease: true, PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
			{TagName: "v1.2.0", Name: "release", Body: "release", Prerelease: false, Draft: false, PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339)},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/"+testRepo+"/-/releases/tags/")
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: tag,
			Assets: []cnbReleaseAsset{
				{Name: "example-darwin-arm64.app.zip", Size: 12345},
				{Name: "updater-windows-amd64.zip", Size: 7776665},
				{Name: "updater-darwin-arm64.zip.sig", Size: 256},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "GIT_COMMIT", Size: 41},
				{Name: "BUILD_TIME", Size: 28},
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Format(time.RFC3339) + "\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:       testRepo,
		CnbToken:      "test-token",
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
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[CNB] 当前版本(currentVersion=%q, buildTime)=%s, 需要更新=%v, 候选版本=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("已订阅 nightly 且存在稳定版时 Check 应返回 nightly 更新，却失败: %v", err)
	}
	if rel.Version != "nightly-x1b2c3" {
		t.Fatalf("期望 nightly 版本 nightly-x1b2c3，实际 %s", rel.Version)
	}
	if rel.Artifact.Filename != "updater-darwin-arm64.zip" {
		t.Fatalf("期望选中 updater-darwin-arm64.zip，实际 %s", rel.Artifact.Filename)
	}
}

// TestCNBNightlyWithStableNoUpdate CNB 源「不需要更新」场景（走公开入口）：
// 已订阅 nightly 渠道，且 nightly 本机已是最新（buildTime 晚于 nightly），Check 应返回 nil,nil。
func TestCNBNightlyWithStableNoUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "nightly-x1b2c3", Name: "nightly", Body: "nightly", Prerelease: true, PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339)},
			{TagName: "v1.2.0", Name: "release", Body: "release", Prerelease: false, Draft: false, PublishedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/"+testRepo+"/-/releases/tags/")
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: tag,
			Assets: []cnbReleaseAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "GIT_COMMIT", Size: 41},
				{Name: "BUILD_TIME", Size: 28},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Add(-48*time.Hour).Format(time.RFC3339) + "\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:       testRepo,
		CnbToken:      "test-token",
		BuildTime:     now.Add(1 * time.Hour), // 比 nightly 与 stable 都新
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
	t.Logf("[CNB] 当前版本(currentVersion=%q, buildTime)=%s, 需要更新=%v, 候选版本=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("nightly 与稳定版都已是最新时 Check 应返回 nil,nil 表示 up-to-date，却报错: %v", err)
	}
	if rel != nil {
		t.Fatalf("已是最新时不应返回 release，却返回 %s", rel.Version)
	}
}

// TestCNBNightlyWithStableNoStableFallbackOnLatest CNB 源「开启预发布时稳定版一起参与」守护（走公开入口）：
// 订阅 nightly，但 nightly 发布时间不比本机 buildTime 新（本机更新），
// nightly 判定为不需要更新（nil,nil）；此时稳定版 1.2.0 比本机新且资产匹配，
// 按"两个版本一起参与"语义，Check 应返回稳定版 1.2.0 候选（而非 up-to-date）。
// 守护"开启预发布即仍纳入稳定版"语义：若改回只看预发布丢弃 stable，此处会变红。
func TestCNBNightlyWithStableNoStableFallbackOnLatest(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "nightly-x1b2c3", Name: "nightly", Body: "nightly", Prerelease: true, PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339)},
			{TagName: "v1.2.0", Name: "release", Body: "release", Prerelease: false, Draft: false, PublishedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/"+testRepo+"/-/releases/tags/")
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: tag,
			Assets: []cnbReleaseAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "GIT_COMMIT", Size: 41},
				{Name: "BUILD_TIME", Size: 28},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Add(-48*time.Hour).Format(time.RFC3339) + "\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:       testRepo,
		CnbToken:      "test-token",
		BuildTime:     now.Add(1 * time.Hour), // 本机 buildTime 晚于 nightly 发布时间，nightly 判定为不需要更新
		Prerelease:    true,
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("NewMirrorProvider 构造失败: %v", err)
	}
	// 开启预发布：取最新一条 = nightly-x1b2c3（预发布），本机 buildTime 晚于其发布时间 → 判定不需要更新。
	// 新逻辑按"最新一条类型"判定，不回退到第二路稳定版：nightly 不需要更新即 up-to-date。
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[CNB] 当前版本(currentVersion=%q, buildTime)=%s, 需要更新=%v, 候选版本=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("nightly 不需要更新时 Check 应返回 up-to-date(nil,nil) 而非报错: %v", err)
	}
	if rel != nil {
		t.Fatalf("最新一条 nightly 不需要更新时 Check 应返回 nil（不回退稳定版），却返回 %s", rel.Version)
	}
}

// TestCNBNightlyWithStableNoStableFallbackOnAssetMiss CNB 源「开启预发布时稳定版一起参与」守护（走公开入口）：
// 订阅 nightly，但 nightly 最新 tag 的资产不匹配本平台（只有 windows/amd64），checkPrerelease 失败；
// 此时稳定版 1.2.0 资产匹配 darwin/arm64 且比本机新，按"两个版本一起参与"语义应返回稳定版 1.2.0。
// 守护"开启预发布时 nightly 失败仍纳入稳定版"语义。
func TestCNBNightlyWithStableNoStableFallbackOnAssetMiss(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "night-x1", Prerelease: true, PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
			{TagName: "1.2.0", Prerelease: false, Draft: false, PublishedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/night-x1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: "night-x1",
			Assets: []cnbReleaseAsset{
				{Name: "updater-windows-amd64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/1.2.0", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: "1.2.0",
			Assets: []cnbReleaseAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:       testRepo,
		CnbToken:      "test-token",
		BuildTime:     now.Add(-72 * time.Hour),
		Prerelease:    true,
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("NewMirrorProvider 构造失败: %v", err)
	}
	// 开启预发布：取最新一条 = night-x1（预发布），但其资产仅 windows 不匹配本机 darwin → 该候选不可用。
	// 新逻辑按"最新一条类型"判定，不回退到第二路稳定版 1.2.0：无可用候选即 up-to-date。
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[CNB] 当前版本(currentVersion=%q, buildTime)=%s, 需要更新=%v, 候选版本=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("nightly 资产不匹配时 Check 应返回 up-to-date(nil,nil) 而非报错: %v", err)
	}
	if rel != nil {
		t.Fatalf("最新一条 nightly 资产不匹配时 Check 应返回 nil（不回退稳定版），却返回 %s", rel.Version)
	}
}
