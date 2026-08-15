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

// TestCNBStableNeedsUpdate CNB 源稳定版「需要更新」场景（走公开入口 NewMirrorProvider + NewUpdaterAssetMatcher）：
// 本地 1.0.0，线上有 1.2.0（非预发布、非草稿），Check 应返回稳定版更新并命中 updater 包、解析校验和。
func TestCNBStableNeedsUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

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
				{Name: "updater-darwin-arm64.zip.sig", Size: 256},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
			},
		})
	})

	mux.HandleFunc("/"+testRepo+"/-/releases/download/v1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:      testRepo,
		CnbToken:     "test-token",
		BuildTime:    now.Add(-72 * time.Hour),
		AssetMatcher: NewUpdaterAssetMatcher(),
		ChecksumFile: "SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("NewMirrorProvider 构造失败: %v", err)
	}

	rel, err := mp.Check(context.Background(), updater.CheckRequest{
		Platform:       "darwin",
		Arch:           "arm64",
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("需要更新时 Check 应返回更新，却失败: %v", err)
	}
	if rel.Version != "v1.2.0" {
		t.Fatalf("期望跳过 nightly 后选中 v1.2.0，实际 %s", rel.Version)
	}
	if rel.Artifact.Filename != "updater-darwin-arm64.zip" {
		t.Fatalf("期望选中 updater-darwin-arm64.zip，实际 %s", rel.Artifact.Filename)
	}
	if rel.Verification == nil || string(rel.Verification.Digest) != "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Fatalf("校验和未正确解析: %+v", rel.Verification)
	}
}

// TestCNBStableNoUpdate CNB 源稳定版「不需要更新」场景（走公开入口 NewMirrorProvider + NewUpdaterAssetMatcher）：
// 本地已是线上最新 1.2.0，Check 应返回 nil,nil（无可用更新）。
func TestCNBStableNoUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

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
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
			},
		})
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:      testRepo,
		CnbToken:     "test-token",
		BuildTime:    now.Add(-72 * time.Hour),
		AssetMatcher: NewUpdaterAssetMatcher(),
		ChecksumFile: "SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("NewMirrorProvider 构造失败: %v", err)
	}

	rel, err := mp.Check(context.Background(), updater.CheckRequest{
		Platform:       "darwin",
		Arch:           "arm64",
		CurrentVersion: "1.2.0",
	})
	if err != nil {
		t.Fatalf("当前已是最新（1.2.0）时 Check 应返回 nil,nil 表示 up-to-date，却报错: %v", err)
	}
	if rel != nil {
		t.Fatalf("已是最新时不应返回 release，却返回 %s", rel.Version)
	}
}
