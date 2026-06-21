package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"perfectpixel/internal/update"
)

// updater_app.go는 자동 업데이터의 바인딩 표면 — 프론트엔드가 호출하는 App
// 메서드들입니다. 전송 비의존 로직은 updater.go 에 있고, 이 파일은 Wails glue 로
// progress 이벤트를 stream 하고 install 요청을 플랫폼별 적용 경로로 보냅니다.

// version은 빌드 시 -ldflags 로 주입됩니다(main.go 참고). 미주입 시 "dev".
var version = "dev"

// Version은 프론트엔드가 표시하고 CheckUpdate 가 비교하는 실행 버전을 반환합니다.
func (a *App) Version() string { return resolvedVersion() }

func resolvedVersion() string {
	if v := strings.TrimSpace(version); v != "" && v != "dev" {
		return v
	}
	if v := localVersionFile(); v != "" {
		return v
	}
	if v := bundleShortVersion(); v != "" {
		return v
	}
	return version
}

func localVersionFile() string {
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "VERSION"), filepath.Join(cwd, "..", "VERSION"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for i := 0; i < 7; i++ {
			candidates = append(candidates, filepath.Join(dir, "VERSION"))
			dir = filepath.Dir(dir)
		}
	}
	for _, path := range candidates {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(b))
		if v != "" && v != "dev" {
			return v
		}
	}
	return ""
}

func bundleShortVersion() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	contentsDir := filepath.Dir(filepath.Dir(exe))
	if filepath.Base(contentsDir) != "Contents" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(contentsDir, "Info.plist"))
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?s)<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`)
	m := re.FindSubmatch(b)
	if len(m) != 2 {
		return ""
	}
	v := strings.TrimSpace(string(m[1]))
	if v == "" || v == "dev" {
		return ""
	}
	return v
}

// CheckUpdate는 설정된 매니페스트를 받아 이 플랫폼에 더 새 빌드가 있는지 보고합니다.
// 시작 시 호출해도 안전합니다: 매니페스트/네트워크 에러는 실패 대신 UpdateInfo.Err
// 로 노출되므로 UI 가 조용히 있을 수 있습니다.
func (a *App) CheckUpdate() (*UpdateInfo, error) {
	current := resolvedVersion()
	m, err := fetchManifest(a.reqCtx(), nil)
	if err != nil {
		return &UpdateInfo{
			Current:       current,
			CanSelfUpdate: canSelfUpdate(),
			DownloadURL:   defaultDownloadPage,
			Err:           err.Error(),
		}, nil
	}
	info := evaluate(current, m)
	return &info, nil
}

// OpenDownloadPage는 폴백 링크로 릴리스 페이지를 브라우저에서 엽니다.
func (a *App) OpenDownloadPage() {
	page := defaultDownloadPage
	if m, err := fetchManifest(a.reqCtx(), nil); err == nil && m.DownloadPage != "" {
		page = m.DownloadPage
	}
	if a.ctx != nil {
		wruntime.BrowserOpenURL(a.ctx, page)
	}
}

// ApplyUpdate는 최신 빌드를 다운로드·검증·설치한 뒤 재실행합니다. 진행 상황은
// "updater:progress" 이벤트로 stream 되고 성공 시 프로세스가 종료됩니다.
func (a *App) ApplyUpdate() error {
	if !canSelfUpdate() {
		a.OpenDownloadPage()
		return nil
	}
	m, err := fetchManifest(a.reqCtx(), nil)
	if err != nil {
		return a.failUpdate(err)
	}
	asset, ok := m.Asset()
	if !ok {
		return a.failUpdate(fmt.Errorf("no update artifact for %s", update.CurrentPlatform()))
	}

	data, err := a.downloadVerify(asset)
	if err != nil {
		return a.failUpdate(err)
	}

	a.emitProgress("applying", asset.Size, asset.Size, "")
	switch runtime.GOOS {
	case "darwin":
		err = applyDarwin(data)
	case "windows":
		err = applyWindows(data)
	case "linux":
		err = applyLinux(data)
	default:
		err = fmt.Errorf("self-update unsupported on %s", runtime.GOOS)
	}
	if err != nil {
		return a.failUpdate(err)
	}

	a.emitProgress("done", asset.Size, asset.Size, "")

	// 핸드오프 전에 진행 중 작업을 정리한다. Linux 는 교체된 바이너리를 재실행하고,
	// macOS 는 helper 가 종료 후 번들을 교체하며, Windows 는 인스톨러가 이어받는다.
	a.CancelGeneration()
	if runtime.GOOS == "linux" {
		_ = relaunch()
	}
	os.Exit(0)
	return nil
}

// downloadVerify는 asset 을 다운로드하면서 progress 를 emit 하고, detached minisign
// 서명을 검증한 뒤 bytes 를 반환합니다.
func (a *App) downloadVerify(asset update.Asset) ([]byte, error) {
	c, err := httpClient()
	if err != nil {
		return nil, err
	}
	data, err := download(a.reqCtx(), c, asset.URL, asset.Size, func(rcv, total int64) {
		a.emitProgress("downloading", rcv, total, "")
	})
	if err != nil {
		return nil, err
	}
	a.emitProgress("verifying", asset.Size, asset.Size, "")

	if strings.TrimSpace(asset.Sig) == "" {
		return nil, fmt.Errorf("update: no signature for update artifact")
	}
	sig, err := fetchBytes(a.reqCtx(), c, asset.Sig)
	if err != nil {
		return nil, err
	}
	if err := update.Verify(data, sig); err != nil {
		return nil, err
	}

	if asset.SHA256 != "" {
		if err := checkSHA256(data, asset.SHA256); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// reqCtx는 업데이터 HTTP 호출용 컨텍스트입니다 — startup 후엔 Wails 컨텍스트,
// 그 전이면 Background.
func (a *App) reqCtx() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) emitProgress(phase string, received, total int64, errMsg string) {
	a.emit("updater:progress", updateProgress{
		Phase: phase, Received: received, Total: total, Err: errMsg,
	})
}

// failUpdate는 에러 progress 이벤트를 emit 하고 에러를 호출자에게 반환합니다.
func (a *App) failUpdate(err error) error {
	a.emitProgress("error", 0, 0, err.Error())
	return err
}
