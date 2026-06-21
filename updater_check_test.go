package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"perfectpixel/internal/update"
)

// TestCheckUpdateLocalManifest는 로컬 매니페스트 override 로 CheckUpdate 의
// fetch→evaluate 경로 전체를 네트워크 없이 검증합니다. VERSION 파일이 0.0.1 이므로
// 0.0.2 매니페스트(현재 플랫폼 서명 자산 포함)는 Available=true 로 평가돼야 합니다.
func TestCheckUpdateLocalManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "latest.json")
	m := update.Manifest{
		Version:      "0.0.2",
		Notes:        "test build",
		DownloadPage: "https://github.com/gykim80/perfectpixel-studio/releases/latest",
		Platforms: map[string]update.Asset{
			update.CurrentPlatform(): {
				URL:  "file://" + filepath.Join(dir, "artifact.bin"),
				Sig:  "file://" + filepath.Join(dir, "artifact.bin.minisig"),
				Size: 1234,
			},
		},
	}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(manifestPath, b, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PERFECTPIXEL_UPDATE_MANIFEST_PATH", manifestPath)
	t.Setenv("PERFECTPIXEL_UPDATE_DISABLE_REMOTE", "1")

	app := &App{}
	info, err := app.CheckUpdate()
	if err != nil {
		t.Fatalf("CheckUpdate error: %v", err)
	}
	if info.Err != "" {
		t.Fatalf("unexpected info.Err: %s", info.Err)
	}
	if info.Latest != "0.0.2" {
		t.Fatalf("latest = %q, want 0.0.2", info.Latest)
	}
	if !info.Available {
		t.Fatalf("expected update available, got %+v", info)
	}
	if info.AssetSize != 1234 {
		t.Fatalf("assetSize = %d, want 1234", info.AssetSize)
	}
}

// TestCheckUpdateNoSourcesGraceful은 매니페스트를 못 받아도 CheckUpdate 가
// 실패하지 않고 Err 를 채워 UI 가 조용히 버전만 보여줄 수 있는지 확인합니다.
func TestCheckUpdateNoSourcesGraceful(t *testing.T) {
	t.Setenv("PERFECTPIXEL_UPDATE_MANIFEST_PATH", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("PERFECTPIXEL_UPDATE_DISABLE_REMOTE", "1")

	app := &App{}
	info, err := app.CheckUpdate()
	if err != nil {
		t.Fatalf("CheckUpdate should not return error: %v", err)
	}
	if info.Available {
		t.Fatalf("should not be available when manifest is unreachable")
	}
	if info.Err == "" {
		t.Fatalf("expected info.Err to be set on fetch failure")
	}
}
