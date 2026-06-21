package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
	"golang.org/x/mod/semver"

	"perfectpixel/internal/update"
)

// updater.go는 데스크톱 자동 업데이터의 전송(transport) 비의존 코어입니다:
// 매니페스트 fetch, 버전 비교, 서명 검증 다운로드, 플랫폼별 적용/재실행. Wails
// 의존성이 없어 로직을 직접 단위 테스트할 수 있으며, updater_app.go 가 이를 App
// 메서드와 progress 이벤트로 연결하는 얇은 Wails 바인딩입니다.

const (
	// defaultManifestURL은 공개 데스크톱 릴리스의 latest.json 입니다. GitHub의
	// releases/latest/download/<asset> 안정 URL 을 사용하므로 새 릴리스가 나오면
	// 자동으로 최신 매니페스트를 가리킵니다. CI/로컬은 PERFECTPIXEL_UPDATE_MANIFEST_URL
	// 또는 _PATH 로 재정의할 수 있습니다.
	defaultManifestURL  = "https://github.com/gykim80/perfectpixel-studio/releases/latest/download/latest.json"
	defaultDownloadPage = "https://github.com/gykim80/perfectpixel-studio/releases/latest"
	httpTimeout         = 30 * time.Second
)

type manifestSource struct {
	kind string // "path" | "url"
	ref  string
}

// UpdateInfo는 프론트엔드의 업데이트 배너를 구동하는 CheckUpdate 결과입니다.
type UpdateInfo struct {
	Available     bool   `json:"available"`
	Current       string `json:"current"`
	Latest        string `json:"latest"`
	Notes         string `json:"notes"`
	CanSelfUpdate bool   `json:"canSelfUpdate"` // 이 플랫폼에서 인앱 적용이 가능하면 true
	DownloadURL   string `json:"downloadUrl"`   // 미지원 플랫폼 폴백용 릴리스 페이지
	AssetSize     int64  `json:"assetSize"`     // 현재 플랫폼 산출물 크기(진행률 바)
	Err           string `json:"err,omitempty"` // 체크 자체가 실패하면 설정
}

// updateProgress는 ApplyUpdate 전반에서 emit 되는 "updater:progress" 이벤트
// 페이로드입니다.
type updateProgress struct {
	Phase    string `json:"phase"` // downloading | verifying | applying | done | error
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Err      string `json:"err,omitempty"`
}

var updateHTTPClient = &http.Client{Timeout: httpTimeout}

func httpClient() (*http.Client, error) { return updateHTTPClient, nil }

func canSelfUpdate() bool { return canSelfUpdateOS(runtime.GOOS) }

func canSelfUpdateOS(goos string) bool {
	switch goos {
	case "darwin", "linux", "windows":
		return true
	default:
		return false
	}
}

// normalizeVersion은 버전을 semver "vX.Y.Z" 로 정규화합니다. 주입되지 않은 "dev"
// 빌드(및 유효하지 않은 semver)는 ok=false 를 반환하므로 dev 빌드는 절대 업데이트
// 프롬프트를 띄우지 않습니다.
func normalizeVersion(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" {
		return "", false
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", false
	}
	return semver.Canonical(v), true
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// manifestSources는 로컬 override(env)를 먼저, 그다음 disable 되지 않았다면 기본
// 원격 URL 을 반환합니다.
func manifestSources() []manifestSource {
	var sources []manifestSource
	if p := expandManifestPath(os.Getenv("PERFECTPIXEL_UPDATE_MANIFEST_PATH")); p != "" {
		sources = append(sources, manifestSource{kind: "path", ref: p})
	}
	if u := strings.TrimSpace(os.Getenv("PERFECTPIXEL_UPDATE_MANIFEST_URL")); u != "" {
		sources = append(sources, manifestSource{kind: "url", ref: u})
	}
	if !envBool("PERFECTPIXEL_UPDATE_DISABLE_REMOTE") {
		sources = append(sources, manifestSource{kind: "url", ref: defaultManifestURL})
	}
	return sources
}

func expandManifestPath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

// fetchManifest는 설정된 로컬 소스를 먼저, 그다음 원격을 시도해 latest.json 을
// 받아 디코딩합니다.
func fetchManifest(ctx context.Context, c *http.Client) (*update.Manifest, error) {
	var lastErr error
	sources := manifestSources()
	if len(sources) == 0 {
		return nil, fmt.Errorf("update: no manifest sources configured")
	}
	for _, source := range sources {
		b, err := fetchManifestSource(ctx, c, source)
		if err != nil {
			lastErr = fmt.Errorf("%s %s: %w", source.kind, source.ref, err)
			continue
		}
		var m update.Manifest
		if err := json.Unmarshal(b, &m); err != nil {
			lastErr = fmt.Errorf("%s %s: %w", source.kind, source.ref, err)
			continue
		}
		return &m, nil
	}
	return nil, fmt.Errorf("update: fetch manifest: %w", lastErr)
}

func fetchManifestSource(ctx context.Context, c *http.Client, source manifestSource) ([]byte, error) {
	switch source.kind {
	case "path":
		return os.ReadFile(source.ref)
	case "url":
		return fetchBytes(ctx, c, source.ref)
	default:
		return nil, fmt.Errorf("unknown manifest source kind %q", source.kind)
	}
}

// evaluate는 실행 중인 버전을 매니페스트와 비교해 프론트엔드용 결과를 만듭니다.
// I/O 가 없어(pure) 비교 로직을 단위 테스트합니다.
func evaluate(current string, m *update.Manifest) UpdateInfo {
	page := m.DownloadPage
	if page == "" {
		page = defaultDownloadPage
	}
	info := UpdateInfo{
		Current:       current,
		Latest:        m.Version,
		Notes:         m.Notes,
		CanSelfUpdate: canSelfUpdate(),
		DownloadURL:   page,
	}
	cur, okCur := normalizeVersion(current)
	latest, okLatest := normalizeVersion(m.Version)
	if !okLatest {
		info.Err = "manifest has no valid version"
		return info
	}
	asset, hasAsset := m.Asset()
	if hasAsset {
		info.AssetSize = asset.Size
	}
	// dev/유효하지 않은 실행 버전은 자동 프롬프트하지 않음. 더 새 매니페스트는 이
	// 플랫폼용 서명된 산출물이 있을 때만 적용 가능.
	if okCur && semver.Compare(latest, cur) > 0 {
		if !hasAsset {
			info.Err = fmt.Sprintf("manifest has no artifact for %s", update.CurrentPlatform())
			return info
		}
		if info.CanSelfUpdate && strings.TrimSpace(asset.Sig) == "" {
			info.Err = "manifest has no signature for update artifact"
			return info
		}
		info.Available = true
	}
	return info
}

// fetchBytes는 URL 을 메모리로 전부 GET 합니다. file:// URL 과 절대 경로도
// 지원하므로 로컬에서 생성한 매니페스트가 로컬 산출물을 가리킬 수 있습니다.
func fetchBytes(ctx context.Context, c *http.Client, rawURL string) ([]byte, error) {
	if b, ok, err := readLocalBytes(rawURL); ok || err != nil {
		return b, err
	}
	if c == nil {
		c = updateHTTPClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func readLocalBytes(ref string) ([]byte, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, true, fmt.Errorf("empty URL")
	}
	if filepath.IsAbs(ref) {
		b, err := os.ReadFile(ref)
		return b, true, err
	}
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "file" {
		return nil, false, nil
	}
	if u.Host != "" && u.Host != "localhost" {
		return nil, true, fmt.Errorf("unsupported file URL host %q", u.Host)
	}
	path, err := url.PathUnescape(u.Path)
	if err != nil {
		return nil, true, err
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") && len(path) >= 3 && path[2] == ':' {
		path = strings.TrimPrefix(path, "/")
	}
	b, err := os.ReadFile(path)
	return b, true, err
}

// download는 url 을 메모리로 받으며 바이트가 도착할 때 onProgress 를 호출합니다.
// total 은 진행률 분모 기대 크기입니다(Content-Length 로 override).
func download(ctx context.Context, c *http.Client, url string, total int64, onProgress func(received, total int64)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	var buf bytes.Buffer
	pr := &progressReader{r: resp.Body, total: total, onProgress: onProgress}
	if _, err := io.Copy(&buf, pr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// progressReader는 누적 읽은 바이트를 보고하되, 이벤트 채널이 넘치지 않게
// throttle 합니다.
type progressReader struct {
	r          io.Reader
	received   int64
	total      int64
	lastEmit   int64
	onProgress func(received, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.received += int64(n)
	if p.onProgress != nil && (p.received-p.lastEmit >= 256<<10 || err == io.EOF) {
		p.lastEmit = p.received
		p.onProgress(p.received, p.total)
	}
	return n, err
}

// checkSHA256은 data 의 다이제스트가 소문자 hex want 와 일치하는지 검증합니다.
func checkSHA256(data []byte, want string) error {
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, want) {
		return fmt.Errorf("update: sha256 mismatch: got %s want %s", got, want)
	}
	return nil
}

// extractBinary는 .tar.gz blob 에서 지정한 이름의 단일 정규 파일을 꺼냅니다.
func extractBinary(targz []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(targz))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && (h.Name == name || strings.HasSuffix(h.Name, "/"+name)) {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("update: %q not found in archive", name)
}

// applyLinux는 다운로드한 tar.gz 안의 바이너리로 실행 중인 바이너리를 교체합니다.
// 호출자가 이후 재실행합니다.
func applyLinux(targz []byte) error {
	bin, err := extractBinary(targz, "perfectpixel")
	if err != nil {
		return err
	}
	return selfupdate.Apply(bytes.NewReader(bin), selfupdate.Options{})
}

// applyWindows는 다운로드한 NSIS 인스톨러를 임시 파일에 쓰고 실행합니다. per-user
// 인스톨러는 관리자 권한이 필요 없고 finish 페이지에서 앱을 재실행하며, 이후
// 호출자가 종료해 인스톨러가 실행 중인 exe 를 교체할 수 있게 합니다.
func applyWindows(installer []byte) error {
	f, err := os.CreateTemp("", "perfectpixel-update-*.exe")
	if err != nil {
		return err
	}
	name := f.Name()
	if _, err := f.Write(installer); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return exec.Command(name).Start()
}

// applyDarwin은 서명된 .app 번들을 staging 한 뒤 현재 프로세스 종료 후 helper 가
// 번들을 교체하게 합니다. 실행 중인 앱이 자기 자신을 삭제하지 않게 하는 구조입니다.
func applyDarwin(zipData []byte) error {
	currentApp, err := currentDarwinAppBundle()
	if err != nil {
		return err
	}
	tempRoot, newApp, err := extractDarwinApp(zipData)
	if err != nil {
		return err
	}
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.RemoveAll(tempRoot)
		}
	}()
	if filepath.Base(newApp) != filepath.Base(currentApp) {
		return fmt.Errorf("update: app bundle name mismatch: got %s want %s", filepath.Base(newApp), filepath.Base(currentApp))
	}
	if err := startDarwinUpdateHelper(currentApp, newApp, tempRoot, os.Getpid()); err != nil {
		return err
	}
	keepTemp = true
	return nil
}

func currentDarwinAppBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	contentsDir := filepath.Dir(filepath.Dir(exe))
	if filepath.Base(contentsDir) != "Contents" {
		return "", fmt.Errorf("update: running executable is not inside a macOS app bundle: %s", exe)
	}
	app := filepath.Dir(contentsDir)
	if !strings.HasSuffix(app, ".app") {
		return "", fmt.Errorf("update: running bundle is not a .app: %s", app)
	}
	return app, nil
}

func extractDarwinApp(zipData []byte) (string, string, error) {
	tempRoot, err := os.MkdirTemp("", "perfectpixel-update-*")
	if err != nil {
		return "", "", err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempRoot)
		}
	}()

	extractDir := filepath.Join(tempRoot, "extracted")
	if err := os.Mkdir(extractDir, 0o700); err != nil {
		return "", "", err
	}
	zipPath := filepath.Join(tempRoot, "update.app.zip")
	if err := os.WriteFile(zipPath, zipData, 0o600); err != nil {
		return "", "", err
	}
	if out, err := exec.Command("/usr/bin/ditto", "-x", "-k", zipPath, extractDir).CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("update: extract app zip: %w: %s", err, strings.TrimSpace(string(out)))
	}
	app, err := findExtractedDarwinApp(extractDir)
	if err != nil {
		return "", "", err
	}
	cleanup = false
	return tempRoot, app, nil
}

func findExtractedDarwinApp(root string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || !d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".app") && isDarwinAppBundle(path) {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("update: no .app bundle found in archive")
	}
	return found, nil
}

func isDarwinAppBundle(path string) bool {
	st, err := os.Stat(filepath.Join(path, "Contents", "Info.plist"))
	return err == nil && !st.IsDir()
}

func startDarwinUpdateHelper(targetApp, newApp, tempRoot string, parentPID int) error {
	helper := filepath.Join(tempRoot, "apply-update.sh")
	script := `#!/bin/sh
set -eu

target_app="$1"
new_app="$2"
temp_root="$3"
parent_pid="$4"
backup_app="${target_app}.previous-update"

i=0
while [ "$i" -lt 150 ] && kill -0 "$parent_pid" 2>/dev/null; do
  i=$((i + 1))
  sleep 0.2
done

rm -rf "$backup_app"
if [ -d "$target_app" ]; then
  mv "$target_app" "$backup_app"
fi

if ! /usr/bin/ditto "$new_app" "$target_app"; then
  rm -rf "$target_app"
  if [ -d "$backup_app" ]; then
    mv "$backup_app" "$target_app"
  fi
  exit 1
fi

/usr/bin/xattr -dr com.apple.quarantine "$target_app" 2>/dev/null || true
/usr/bin/open "$target_app" >/dev/null 2>&1 || true
rm -rf "$backup_app"
rm -rf "$temp_root"
`
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", helper, targetApp, newApp, tempRoot, fmt.Sprint(parentPID))
	cmd.Stdout, cmd.Stderr, cmd.Stdin = nil, nil, nil
	return cmd.Start()
}

// relaunch는 (방금 교체된) 실행 파일의 새 복사본을 시작합니다.
func relaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Start()
}
