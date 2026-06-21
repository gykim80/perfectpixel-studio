package main

import (
	"testing"

	"perfectpixel/internal/update"
)

func TestNormalizeVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"0.0.1", "v0.0.1", true},
		{"v1.2.3", "v1.2.3", true},
		{"dev", "", false},
		{"", "", false},
		{"not-a-version", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeVersion(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("normalizeVersion(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestEvaluate(t *testing.T) {
	key := update.CurrentPlatform()
	withAsset := &update.Manifest{
		Version:   "0.0.2",
		Platforms: map[string]update.Asset{key: {URL: "https://x/app", Sig: "https://x/app.minisig", Size: 10}},
	}

	// 더 새 버전 + 서명된 자산 => available.
	if info := evaluate("0.0.1", withAsset); !info.Available || info.Err != "" {
		t.Fatalf("expected available update, got %+v", info)
	}
	// 동일 버전 => not available.
	if info := evaluate("0.0.2", withAsset); info.Available {
		t.Fatalf("same version should not be available: %+v", info)
	}
	// dev 빌드는 절대 프롬프트하지 않는다.
	if info := evaluate("dev", withAsset); info.Available {
		t.Fatalf("dev build should never prompt: %+v", info)
	}
	// 서명 없는 자산은 자가 업데이트 가능 플랫폼에서 거부된다.
	noSig := &update.Manifest{Version: "0.0.2", Platforms: map[string]update.Asset{key: {URL: "https://x/app", Size: 10}}}
	if info := evaluate("0.0.1", noSig); canSelfUpdate() && (info.Available || info.Err == "") {
		t.Fatalf("unsigned asset must be rejected on self-update platforms: %+v", info)
	}
}
