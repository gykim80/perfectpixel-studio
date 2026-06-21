package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/sprite"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App은 Wails 애플리케이션 본체입니다.
type App struct {
	ctx context.Context

	genMu      sync.Mutex
	genCancels map[int]context.CancelFunc // 진행 중인 생성 작업들의 취소 함수 (병렬 배치 지원)
	genSeq     int
}

// NewApp은 새 App 인스턴스를 생성합니다.
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// provider는 현재 활성 프로바이더 클라이언트를 반환합니다.
func (a *App) provider() (gen.Provider, error) {
	s := config.Load()
	cfg := s.Cfg(s.Provider)
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%s API 키가 설정되지 않았습니다. 설정에서 입력해 주세요", gen.ProviderLabel(s.Provider))
	}
	return gen.New(s.Provider, cfg.APIKey, cfg.Model)
}

func (a *App) emit(event string, data any) {
	runtime.EventsEmit(a.ctx, event, data)
}

// genContext는 취소 가능한 생성용 컨텍스트를 만듭니다.
// 병렬 배치 생성에서 여러 컨텍스트가 동시에 살아 있을 수 있으므로 각각을 추적하고,
// 호출자가 완료 시 해제할 수 있도록 release 함수를 함께 반환합니다.
func (a *App) genContext() (context.Context, func()) {
	a.genMu.Lock()
	defer a.genMu.Unlock()
	ctx, cancel := context.WithCancel(a.ctx)
	if a.genCancels == nil {
		a.genCancels = make(map[int]context.CancelFunc)
	}
	id := a.genSeq
	a.genSeq++
	a.genCancels[id] = cancel
	release := func() {
		a.genMu.Lock()
		defer a.genMu.Unlock()
		if c, ok := a.genCancels[id]; ok {
			c()
			delete(a.genCancels, id)
		}
	}
	return ctx, release
}

// CancelGeneration은 진행 중인 모든 생성 작업을 취소합니다.
func (a *App) CancelGeneration() {
	a.genMu.Lock()
	defer a.genMu.Unlock()
	for id, cancel := range a.genCancels {
		cancel()
		delete(a.genCancels, id)
	}
}

// friendlyErr는 취소 오류를 사용자 친화적 메시지로 바꿉니다.
func friendlyErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return errors.New("생성이 취소되었습니다")
	}
	return err
}

// ---------- 설정 ----------

// ProviderInfo는 프로바이더별 설정 상태입니다.
type ProviderInfo struct {
	HasKey     bool     `json:"hasKey"`
	KeyPreview string   `json:"keyPreview"`
	Model      string   `json:"model"`
	Models     []string `json:"models"` // 선택 가능한 모델 목록 (최신 모델이 맨 앞)
}

// SettingsInfo는 프론트엔드에 노출되는 설정 상태입니다.
type SettingsInfo struct {
	Provider  string                  `json:"provider"`
	Providers map[string]ProviderInfo `json:"providers"`
}

func keyPreview(key string) string {
	if len(key) > 8 {
		return key[:4] + "····" + key[len(key)-4:]
	}
	return ""
}

// GetSettings는 현재 설정 상태를 반환합니다 (키 원문은 노출하지 않음).
func (a *App) GetSettings() SettingsInfo {
	s := config.Load()
	info := SettingsInfo{Provider: s.Provider, Providers: map[string]ProviderInfo{}}
	for _, p := range gen.SupportedProviders {
		cfg := s.Cfg(p)
		model := cfg.Model
		if model == "" {
			model = gen.DefaultModelFor(p)
		}
		info.Providers[p] = ProviderInfo{
			HasKey:     cfg.APIKey != "",
			KeyPreview: keyPreview(cfg.APIKey),
			Model:      model,
			Models:     gen.ModelsFor(p),
		}
	}
	return info
}

// SaveProviderKey는 특정 프로바이더의 API 키를 검증 후 저장합니다.
func (a *App) SaveProviderKey(provider, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("API 키를 입력해 주세요")
	}
	c, err := gen.New(provider, key, "")
	if err != nil {
		return err
	}
	if err := c.ValidateKey(a.ctx); err != nil {
		return err
	}
	s := config.Load()
	s.Cfg(provider).APIKey = key
	return config.Save(s)
}

// SaveProviderModel은 프로바이더의 이미지 모델을 변경합니다 (빈 값이면 기본 모델로 복원).
func (a *App) SaveProviderModel(provider, model string) error {
	switch provider {
	case gen.ProviderGemini, gen.ProviderOpenAI, gen.ProviderOpenRouter, gen.ProviderFal, gen.ProviderBytePlus:
	default:
		return fmt.Errorf("지원하지 않는 프로바이더입니다: %s", provider)
	}
	model = strings.TrimSpace(model)
	if model == gen.DefaultModelFor(provider) {
		model = "" // 기본 모델은 빈 값으로 저장해 향후 기본값 변경을 따라가게 함
	}
	s := config.Load()
	s.Cfg(provider).Model = model
	return config.Save(s)
}

// SetProvider는 활성 프로바이더를 변경합니다.
func (a *App) SetProvider(provider string) error {
	switch provider {
	case gen.ProviderGemini, gen.ProviderOpenAI, gen.ProviderOpenRouter, gen.ProviderFal, gen.ProviderBytePlus:
	default:
		return fmt.Errorf("지원하지 않는 프로바이더입니다: %s", provider)
	}
	s := config.Load()
	s.Provider = provider
	return config.Save(s)
}

// ---------- 세션 저장/복원 ----------

// SaveSession은 현재 작업 상태(JSON)를 디스크에 저장합니다 (앱 재시작 시 복원용).
func (a *App) SaveSession(data string) error {
	path, err := config.SessionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// 임시 파일에 쓴 뒤 교체해 손상 방지
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadSession은 저장된 작업 세션 JSON을 반환합니다 (없으면 빈 문자열).
func (a *App) LoadSession() string {
	path, err := config.SessionPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// ClearSession은 저장된 작업 세션을 삭제합니다.
func (a *App) ClearSession() error {
	path, err := config.SessionPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ---------- 이미지 입출력 헬퍼 ----------

var dataURLRe = regexp.MustCompile(`^data:image/[a-zA-Z+.-]+;base64,`)

func decodeDataURL(dataURL string) ([]byte, error) {
	m := dataURLRe.FindString(dataURL)
	if m == "" {
		return nil, errors.New("올바른 이미지 데이터가 아닙니다")
	}
	return base64.StdEncoding.DecodeString(dataURL[len(m):])
}

func pngDataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func decodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("이미지 디코딩 실패: %w", err)
	}
	return img, nil
}

// PickImage는 파일 선택 대화상자를 열고 선택된 이미지를 dataURL로 반환합니다.
func (a *App) PickImage() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "베이스 이미지 선택",
		Filters: []runtime.FileFilter{
			{DisplayName: "이미지 (*.png;*.jpg;*.jpeg;*.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // 취소
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("파일 읽기 실패: %w", err)
	}
	img, err := decodeImage(data)
	if err != nil {
		return "", err
	}
	return pngDataURL(img)
}

// ---------- 생성 파이프라인 ----------

// GenerateCharacterArgs는 텍스트 → 베이스 캐릭터 생성 요청입니다.
type GenerateCharacterArgs struct {
	Description string `json:"description"`
	StyleKey    string `json:"styleKey"`
	StyleCustom string `json:"styleCustom"`
}

// GenerateCharacter는 설명만으로 베이스 캐릭터 이미지를 생성합니다.
func (a *App) GenerateCharacter(args GenerateCharacterArgs) (string, error) {
	if strings.TrimSpace(args.Description) == "" {
		return "", errors.New("캐릭터 설명을 입력해 주세요")
	}
	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	prompt := sprite.BuildCharacterPrompt(args.Description, style, 256)

	p, err := a.provider()
	if err != nil {
		return "", err
	}
	// 종료 시 진행 표시 제거
	defer a.emit("progress", map[string]any{"phase": "idle", "message": ""})

	a.emit("progress", map[string]any{"phase": "character", "message": "캐릭터 생성 중..."})
	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	raw, err := p.GenerateImage(genCtx, prompt, nil, "1:1")
	if err != nil {
		return "", friendlyErr(err)
	}
	img, err := decodeImage(raw)
	if err != nil {
		return "", err
	}
	// 배경 제거된 미리보기 제공
	clean := sprite.RemoveBackground(img)
	if n := sprite.PaletteSizeForStyle(args.StyleKey); n > 0 {
		single := []*image.NRGBA{clean}
		sprite.PixelPostProcess(single, n)
		clean = single[0]
	}
	saveGalleryPNG("character-"+galleryStamp(), clean)
	return pngDataURL(clean)
}

// GenerateStateArgs는 상태별 스트립 생성 요청입니다.
type GenerateStateArgs struct {
	BaseImage   string           `json:"baseImage"` // dataURL
	Description string           `json:"description"`
	StyleKey    string           `json:"styleKey"`
	StyleCustom string           `json:"styleCustom"`
	CellSize    int              `json:"cellSize"`
	SafeMargin  int              `json:"safeMargin"`
	Feedback    string           `json:"feedback"`
	RefStrip    string           `json:"refStrip"` // 정면(south) 스트립 dataURL — 방향 세트 생성 시 모션 참조용
	State       sprite.StateSpec `json:"state"`
}

// StateResult는 상태 생성 결과입니다.
type StateResult struct {
	Name     string             `json:"name"`
	RawStrip string             `json:"rawStrip"`
	Frames   []string           `json:"frames"`
	Expected int                `json:"expected"`
	Found    int                `json:"found"`
	Warnings []string           `json:"warnings"`
	Scores   sprite.ScoreResult `json:"scores"`
}

// GenerateState는 한 상태의 스트립을 생성하고 프레임을 추출합니다.
func (a *App) GenerateState(args GenerateStateArgs) (StateResult, error) {
	res := StateResult{Name: args.State.Name, Expected: args.State.Frames}

	if args.State.Frames < 1 || args.State.Frames > 10 {
		return res, errors.New("프레임 수는 1~10 사이여야 합니다")
	}
	baseRaw, err := decodeDataURL(args.BaseImage)
	if err != nil {
		return res, fmt.Errorf("베이스 이미지 오류: %w", err)
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}
	margin := args.SafeMargin
	if margin <= 0 {
		margin = max(8, cellSize/12)
	}

	style := sprite.ResolveStyle(args.StyleKey, args.StyleCustom)
	aspect := sprite.AspectForFrames(args.State.Frames)

	p, err := a.provider()
	if err != nil {
		return res, err
	}

	// 베이스 캐릭터 정체성 검사용 (투명 배경일 때만 InspectFrames가 사용).
	// 뒷면 계열 방향은 정면 베이스와 색 구성이 달라 오탐하므로 검사를 건너뜁니다.
	var baseN *image.NRGBA
	if !sprite.IsBackFacing(args.State.Facing) {
		if bimg, err := decodeImage(baseRaw); err == nil {
			baseN = sprite.ToNRGBA(bimg)
		}
	}

	// 생성 참조 이미지: 베이스 캐릭터 + (선택) 정면 스트립
	refs := [][]byte{baseRaw}
	if strings.TrimSpace(args.RefStrip) != "" {
		if refRaw, err := decodeDataURL(args.RefStrip); err == nil {
			refs = append(refs, refRaw)
		}
	}

	// 완벽한 프레임 수가 나올 때까지 자동 재시도 (최대 3회)
	const maxAttempts = 3
	expected := args.State.Frames
	feedback := args.Feedback
	genCtx, releaseGen := a.genContext()
	defer releaseGen()
	var best StateResult
	var bestImgs []*image.NRGBA
	bestScore := -1 << 30
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		prompt := sprite.BuildStripPrompt(args.Description, style, args.State, feedback, 256)
		if len(refs) > 1 {
			prompt += "\nMotion reference: the second attached image is the FRONT-view animation strip of this same character performing this exact action. Reproduce the same motion timing and pose phases frame by frame, but viewed from the required facing direction above.\n"
		}

		msg := "AI 프레임 생성 중..."
		if attempt > 1 {
			msg = fmt.Sprintf("프레임 수 보정 재생성 중... (%d/%d)", attempt, maxAttempts)
		}
		a.emit("progress", map[string]any{"phase": "generate", "state": args.State.Name, "message": msg})

		stripRaw, err := p.GenerateImage(genCtx, prompt, refs, aspect)
		if err != nil {
			lastErr = friendlyErr(err)
			break // API 오류/취소는 재시도 의미 없음 (클라이언트가 자체 재시도함)
		}

		a.emit("progress", map[string]any{"phase": "extract", "state": args.State.Name, "message": "배경 제거 및 프레임 추출 중..."})
		stripImg, err := decodeImage(stripRaw)
		if err != nil {
			lastErr = err
			continue
		}
		nimg := sprite.ToNRGBA(stripImg)
		bgKey := sprite.DetectBackground(nimg)
		clean := sprite.RemoveBackground(nimg)

		cand := StateResult{Name: args.State.Name, Expected: expected}
		if rawURL, err := pngDataURL(clean); err == nil {
			cand.RawStrip = rawURL
		}
		extracted := sprite.ExtractFrames(clean, expected, cellSize, cellSize, margin)
		// 프레임별 품질 검사는 양자화 전 원본 프레임에 수행 (양자화로 미세한
		// 정체성 차이가 뭉개지면 drift 감지 민감도가 떨어짐)
		insp := sprite.InspectFrames(extracted.Frames, bgKey, baseN)
		// 픽셀아트 스타일이면 공유 팔레트 양자화 + 픽셀 그리드 스냅으로 "진짜" 픽셀아트화
		sprite.PixelPostProcess(extracted.Frames, sprite.PaletteSizeForStyle(args.StyleKey))
		cand.Found = extracted.Found
		cand.Warnings = extracted.Warnings
		for _, f := range extracted.Frames {
			u, err := pngDataURL(f)
			if err != nil {
				return res, err
			}
			cand.Frames = append(cand.Frames, u)
		}
		cand.Warnings = append(cand.Warnings, insp.Errors...)
		cand.Warnings = append(cand.Warnings, insp.Warnings...)
		cand.Scores = sprite.ScoreFramesForState(extracted.Frames, args.State.Name)
		// 인접 프레임 변화가 거의 없으면(사실상 정지) 비차단 경고.
		// 임계값 0.01은 실측 기반: 의도적으로 미동이 적은 meditate(~1.5%)도 통과시키고
		// 0.01 미만, 즉 프레임이 사실상 동일한 "애니메이션이 전혀 움직이지 않는" 결함만 잡습니다.
		if cand.Found >= 2 && sprite.MotionPresence(extracted.Frames) < 0.01 {
			cand.Warnings = append(cand.Warnings,
				"프레임 간 움직임이 거의 없습니다. 동작이 더 분명하게 드러나도록 동작 설명을 보강해 재생성하는 것을 권장합니다.")
		}
		errCount := len(insp.Errors)

		// 프레임 수가 정확하고 심각한 품질 문제가 없으면 즉시 성공
		if cand.Found == expected && insp.Ok() {
			saveGalleryFrames(args.State.Name, extracted.Frames)
			return cand, nil
		}
		// 최선 후보 갱신: 프레임 수 우선, 같으면 오류 적은 쪽
		score := cand.Found*100 - errCount*10
		if score > bestScore {
			best, bestScore, bestImgs = cand, score, extracted.Frames
		}
		lastErr = nil

		// 다음 시도용 보정 피드백 (사용자 피드백 유지 + 측정 기반 보정 지시)
		var fixes []string
		if cand.Found != expected {
			fixes = append(fixes, fmt.Sprintf(
				"IMPORTANT CORRECTION: the last attempt read as %d poses but EXACTLY %d are required. Redraw as one horizontal row of %d equally sized poses, one clearly separated pose per column, each ringed by a clean magenta gap so none touch or overlap. Do not draw any frame, border, or film strip.",
				cand.Found, expected, expected))
		}
		if len(insp.RetryHints) > 0 {
			fixes = append(fixes, "QUALITY CORRECTIONS detected by automated inspection (fix all of these):")
			fixes = append(fixes, insp.RetryHints...)
		}
		auto := strings.Join(fixes, "\n")
		if args.Feedback != "" {
			feedback = args.Feedback + "\n" + auto
		} else {
			feedback = auto
		}

		// 마지막 시도 직전 메시지 갱신용: 품질 보정 재시도임을 표시
		if cand.Found == expected && errCount > 0 && attempt < maxAttempts {
			a.emit("progress", map[string]any{"phase": "generate", "state": args.State.Name,
				"message": fmt.Sprintf("품질 보정 재생성 중... (%d/%d)", attempt+1, maxAttempts)})
		}
	}

	if best.Found == 0 {
		if lastErr != nil {
			return res, lastErr
		}
		return res, errors.New("스프라이트 프레임을 추출하지 못했습니다. 캐릭터 설명을 더 구체적으로 작성해 보세요")
	}
	if best.Found != expected {
		best.Warnings = append(best.Warnings,
			fmt.Sprintf("자동 재시도 후에도 프레임 수가 다릅니다 (요청 %d개 → 추출 %d개). 가장 근접한 결과를 표시합니다.", expected, best.Found))
	} else {
		best.Warnings = append(best.Warnings,
			"자동 재시도 후에도 일부 품질 문제가 남아 있습니다. 프레임을 확인하고 필요하면 피드백과 함께 재생성해 주세요.")
	}
	saveGalleryFrames(args.State.Name, bestImgs)
	return best, nil
}

// ---------- 8방향 세트 ----------

// ListDirections는 8방향 메타데이터 목록을 반환합니다 (3x3 그리드 순서).
func (a *App) ListDirections() []sprite.DirectionInfo {
	return sprite.ListDirections()
}

// ListPresets는 100개 상황 키워드 카탈로그를 반환합니다 (프리셋 선택 UI용).
func (a *App) ListPresets() []sprite.PresetInfo {
	return sprite.ListPresets()
}

// MirrorFrames는 프레임들을 좌우 반전합니다 (east→west 등 미러 방향 생성용).
func (a *App) MirrorFrames(frames []string) ([]string, error) {
	out := make([]string, 0, len(frames))
	for i, fu := range frames {
		raw, err := decodeDataURL(fu)
		if err != nil {
			return nil, fmt.Errorf("프레임 %d 디코딩 실패: %w", i+1, err)
		}
		img, err := decodeImage(raw)
		if err != nil {
			return nil, err
		}
		u, err := pngDataURL(sprite.MirrorNRGBA(sprite.ToNRGBA(img)))
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// ---------- 내보내기 ----------

// ExportState는 내보내기용 상태 데이터입니다.
type ExportState struct {
	Name   string   `json:"name"`
	FPS    int      `json:"fps"`
	Loop   bool     `json:"loop"`
	Frames []string `json:"frames"` // 선택/정렬된 dataURL
}

// ExportArgs는 프로젝트 내보내기 요청입니다.
type ExportArgs struct {
	Character string        `json:"character"`
	CellSize  int           `json:"cellSize"`
	States    []ExportState `json:"states"`
}

// ExportProject는 디렉토리를 선택받아 스프라이트시트/매니페스트/GIF/프레임을 저장합니다.
func (a *App) ExportProject(args ExportArgs) (string, error) {
	if len(args.States) == 0 {
		return "", errors.New("내보낼 애니메이션이 없습니다")
	}
	cellSize := args.CellSize
	if cellSize <= 0 {
		cellSize = 256
	}

	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "내보낼 폴더 선택",
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil // 취소
	}

	charName := strings.TrimSpace(args.Character)
	if charName == "" {
		charName = "character"
	}
	safeName := sanitizeName(charName)
	outDir := filepath.Join(dir, safeName)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	defer a.emit("progress", map[string]any{"phase": "idle", "message": ""})
	a.emit("progress", map[string]any{"phase": "export", "message": "스프라이트시트·GIF 내보내는 중..."})

	// 상태 프레임 디코딩
	var stateFrames []sprite.StateFrames
	for _, st := range args.States {
		if len(st.Frames) == 0 {
			continue
		}
		sf := sprite.StateFrames{
			Spec: sprite.StateSpec{Name: st.Name, Frames: len(st.Frames), FPS: st.FPS, Loop: st.Loop},
		}
		for _, fu := range st.Frames {
			raw, err := decodeDataURL(fu)
			if err != nil {
				return "", fmt.Errorf("%s 프레임 디코딩 실패: %w", st.Name, err)
			}
			img, err := decodeImage(raw)
			if err != nil {
				return "", err
			}
			sf.Frames = append(sf.Frames, sprite.ToNRGBA(img))
		}
		stateFrames = append(stateFrames, sf)
	}
	if len(stateFrames) == 0 {
		return "", errors.New("내보낼 프레임이 없습니다")
	}

	// 파일명 프리픽스: 캐릭터별 산출물이 섞여도 구분되도록 모든 파일 앞에 캐릭터 이름을 붙인다.
	prefix := safeName + "-"

	// 1) 스프라이트시트 + 매니페스트
	sheet, manifest := sprite.ComposeAtlas(safeName, stateFrames, cellSize, cellSize)
	if err := writePNG(filepath.Join(outDir, prefix+"sprite-sheet.png"), sheet); err != nil {
		return "", err
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outDir, prefix+"manifest.json"), manifestJSON, 0o644); err != nil {
		return "", err
	}
	// Aseprite 호환 JSON (Phaser/Unity/Godot 임포터용)
	aseJSON, err := sprite.BuildAsepriteJSON(manifest)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outDir, prefix+"sprite-sheet.json"), aseJSON, 0o644); err != nil {
		return "", err
	}

	// 2) 상태별 GIF + 개별 프레임 PNG
	for _, sf := range stateFrames {
		stateDir := filepath.Join(outDir, "frames", sanitizeName(sf.Spec.Name))
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			return "", err
		}
		for i, frame := range sf.Frames {
			if err := writePNG(filepath.Join(stateDir, fmt.Sprintf("%sframe-%02d.png", prefix, i)), frame); err != nil {
				return "", err
			}
		}
		stateName := sanitizeName(sf.Spec.Name)
		gifBytes, err := sprite.EncodeGIF(sf.Frames, sf.Spec.FPS, sf.Spec.Loop)
		if err != nil {
			return "", fmt.Errorf("%s GIF 인코딩 실패: %w", sf.Spec.Name, err)
		}
		gifDir := filepath.Join(outDir, "gif")
		if err := os.MkdirAll(gifDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(gifDir, prefix+stateName+".gif"), gifBytes, 0o644); err != nil {
			return "", err
		}
		// APNG: 풀 알파 지원 (GIF의 1-bit 투명도 한계 보완)
		apngBytes, err := sprite.EncodeAPNG(sf.Frames, sf.Spec.FPS, sf.Spec.Loop)
		if err != nil {
			return "", fmt.Errorf("%s APNG 인코딩 실패: %w", sf.Spec.Name, err)
		}
		apngDir := filepath.Join(outDir, "apng")
		if err := os.MkdirAll(apngDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(apngDir, prefix+stateName+".png"), apngBytes, 0o644); err != nil {
			return "", err
		}
	}

	return outDir, nil
}

// RevealInFinder는 내보낸 폴더를 파일 탐색기에서 엽니다.
func (a *App) RevealInFinder(path string) {
	runtime.BrowserOpenURL(a.ctx, "file://"+path)
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

var unsafeNameRe = regexp.MustCompile(`[^a-zA-Z0-9가-힣_-]+`)

func sanitizeName(name string) string {
	s := unsafeNameRe.ReplaceAllString(strings.TrimSpace(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "character"
	}
	return s
}
