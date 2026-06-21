// Package update는 데스크톱 자동 업데이트의 공유 타입과 서명 검증을 정의합니다.
// latest.json 매니페스트 포맷, 플랫폼별 자산 조회, minisign 검증의 단일 진실
// 원천(single source of truth)으로, CI 서명 도구(cmd/sign)와 실행 중인
// 업데이터(updater.go)가 모두 이 패키지를 import 하므로 서명 경로와 검증 경로가
// 절대 어긋나지 않습니다.
package update

import "runtime"

// Manifest는 데스크톱 릴리스와 함께 배포되는 latest.json 입니다. 업데이터가 이를
// 공개 릴리스 저장소에서 받아 Version 을 실행 중인 빌드와 비교하고, Asset 으로
// 현재 플랫폼의 산출물을 조회합니다.
type Manifest struct {
	Version      string           `json:"version"`       // 릴리스 버전, 예: "v1.1.0"
	Notes        string           `json:"notes"`         // 마크다운 릴리스 노트
	PubDate      string           `json:"pub_date"`      // RFC3339, 선택
	DownloadPage string           `json:"download_page"` // 사람이 보는 다운로드 페이지 (macOS 수동 업데이트 폴백)
	Platforms    map[string]Asset `json:"platforms"`     // PlatformKey 키, 예: "darwin-arm64"
}

// Asset은 한 플랫폼의 다운로드 산출물과 업데이터가 검증/표시에 필요한 메타데이터입니다.
type Asset struct {
	URL    string `json:"url"`    // 산출물 직접 다운로드 URL
	Sig    string `json:"sig"`    // detached minisign(.minisig) 서명 URL
	Size   int64  `json:"size"`   // 산출물 크기(바이트) — 다운로드 진행률 분모
	SHA256 string `json:"sha256"` // 소문자 hex 다이제스트 — verify 이후 2차 무결성 검사
}

// PlatformKey는 주어진 OS/arch 에 대한 Manifest.Platforms 의 맵 키입니다.
// 업데이터는 CurrentPlatform 을, 매니페스트 생성기는 동일한 방식으로 키를
// 만들므로 조회가 항상 일치합니다.
func PlatformKey(goos, goarch string) string { return goos + "-" + goarch }

// CurrentPlatform은 실행 중인 바이너리에 대한 PlatformKey 입니다.
func CurrentPlatform() string { return PlatformKey(runtime.GOOS, runtime.GOARCH) }

// Asset은 매니페스트가 현재 플랫폼의 산출물을 가지고 있으면 그것을 반환합니다.
func (m Manifest) Asset() (Asset, bool) {
	a, ok := m.Platforms[CurrentPlatform()]
	if ok {
		return a, true
	}
	return Asset{}, false
}
