package wails_updater_providers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// TestGitHubStableNeedsUpdate GitHub 源稳定版「需要更新」场景（走公开入口 NewMirrorProvider + NewUpdaterAssetMatcher）：
// 本地 1.0.0，/releases/latest 返回 1.2.0，Check 应返回稳定版更新并命中 updater 包、解析校验和。
func TestGitHubStableNeedsUpdate(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+testRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.2.0",
			Name:        "release 1.2.0",
			Prerelease:  false,
			PublishedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
			Assets: []githubAsset{
				{Name: "example-darwin-arm64.app.zip", Size: 12345},
				{Name: "updater-darwin-arm64.zip.sig", Size: 256},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
			},
		})
	})

	mux.HandleFunc("/"+testRepo+"/releases/download/v1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:   testRepo,
		GithubToken:  "test-token",
		BuildTime:    time.Now().Add(-72 * time.Hour),
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
		t.Fatalf("期望选中 v1.2.0，实际 %s", rel.Version)
	}
	if rel.Artifact.Filename != "updater-darwin-arm64.zip" {
		t.Fatalf("期望选中 updater-darwin-arm64.zip，实际 %s", rel.Artifact.Filename)
	}
	if rel.Verification == nil || string(rel.Verification.Digest) != "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Fatalf("校验和未正确解析: %+v", rel.Verification)
	}
}

// TestGitHubStableNoUpdate GitHub 源稳定版「不需要更新」场景（走公开入口 NewMirrorProvider + NewUpdaterAssetMatcher）：
// 本地已是线上最新 1.2.0，Check 应返回 error（无可用更新）。
func TestGitHubStableNoUpdate(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+testRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.2.0",
			Name:        "release 1.2.0",
			Prerelease:  false,
			PublishedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
			Assets: []githubAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
			},
		})
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:   testRepo,
		GithubToken:  "test-token",
		BuildTime:    time.Now().Add(-72 * time.Hour),
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
