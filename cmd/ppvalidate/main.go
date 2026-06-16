// Command ppvalidate는 GUI 없이 실제 AI 생성 파이프라인을 구동해
// 스프라이트 품질을 검증하는 헤드리스 하니스입니다.
// 앱의 GenerateState 로직(프롬프트 → 생성 → 배경제거 → 프레임추출 → 품질검사 → 픽셀화)을
// 그대로 재현하여, 카테고리/방향별 생성 결과의 품질 점수를 수집합니다.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	_ "image/gif"
	_ "image/jpeg"

	_ "golang.org/x/image/webp"
	"path/filepath"
	"strings"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/sprite"
)

// stripResult는 한 상태(애니메이션) 생성의 품질 측정 결과입니다.
type stripResult struct {
	Name     string
	Expected int
	Found    int
	Attempts int
	FPS      int
	Loop     bool
	Score    int
	Identity float64
	Motion   float64
	Contact  float64
	Errors   []string
	Warnings []string
	rel      string // outDir 기준 하위 디렉토리 (로스터 모드: char-NN, 그 외 빈 값)
	frames   []*image.NRGBA
	rawClean *image.NRGBA // 양자화 전 정리된 스트립 (방향 세트 모션 참조용)
}

func (r stripResult) ok() bool { return r.Found == r.Expected && len(r.Errors) == 0 }

func decode(raw []byte) (*image.NRGBA, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return sprite.ToNRGBA(img), nil
}

func savePNG(path string, img image.Image) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		fmt.Printf("[저장오류] PNG 인코딩 %s: %v\n", path, err)
		return
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		fmt.Printf("[저장오류] 쓰기 %s: %v\n", path, err)
	}
}

// maxAttempts는 상태별 품질 보정 재생성 최대 시도 횟수입니다 (-attempts 플래그).
var maxAttempts = 3

// genStrip은 앱과 동일한 자동 재시도 품질 보정 루프로 한 상태를 생성합니다.
func genStrip(ctx context.Context, p gen.Provider, desc, styleKey, style string,
	spec sprite.StateSpec, refs [][]byte, baseN *image.NRGBA) (stripResult, error) {

	expected := spec.Frames
	aspect := sprite.AspectForFrames(expected)
	palette := sprite.PaletteSizeForStyle(styleKey)
	feedback := ""

	var best stripResult
	bestScore := -1 << 30
	best.Name, best.Expected = spec.Name, expected

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		prompt := sprite.BuildStripPrompt(desc, style, spec, feedback)
		if len(refs) > 1 {
			prompt += "\nMotion reference: the second attached image is the FRONT-view animation strip of this same character performing this exact action. Reproduce the same motion timing and pose phases frame by frame, but viewed from the required facing direction above.\n"
		}
		raw, err := p.GenerateImage(ctx, prompt, refs, aspect)
		if err != nil {
			return best, err
		}
		nimg, err := decode(raw)
		if err != nil {
			continue
		}
		bgKey := sprite.DetectBackground(nimg)
		clean := sprite.RemoveBackground(nimg)
		ext := sprite.ExtractFrames(clean, expected, 256, 256, 24)
		insp := sprite.InspectFrames(ext.Frames, bgKey, baseN)
		sprite.PixelPostProcess(ext.Frames, palette)

		cand := stripResult{
			Name: spec.Name, Expected: expected, Found: ext.Found, Attempts: attempt,
			FPS: spec.FPS, Loop: spec.Loop, Motion: sprite.MotionPresence(ext.Frames),
			frames: ext.Frames, rawClean: clean,
		}
		cand.Warnings = append(cand.Warnings, ext.Warnings...)
		cand.Warnings = append(cand.Warnings, insp.Warnings...)
		cand.Errors = append(cand.Errors, insp.Errors...)

		if cand.ok() {
			qm := sprite.ScoreFrames(ext.Frames)
			cand.Score = int(qm.Overall * 100)
			cand.Identity = qm.Identity
			cand.Motion = qm.Motion
			cand.Contact = qm.Contact
			return cand, nil
		}
		score := cand.Found*100 - len(cand.Errors)*10
		if score > bestScore {
			best, bestScore = cand, score
		}

		var fixes []string
		if cand.Found != expected {
			fixes = append(fixes, fmt.Sprintf(
				"IMPORTANT CORRECTION: the last attempt read as %d poses but EXACTLY %d are required. Redraw as %d equal columns, one clearly separated pose per column, each ringed by a clean magenta gutter.",
				cand.Found, expected, expected))
		}
		fixes = append(fixes, insp.RetryHints...)
		feedback = strings.Join(fixes, "\n")
	}
	if len(best.frames) > 0 {
		qm := sprite.ScoreFrames(best.frames)
		best.Score = int(qm.Overall * 100)
		best.Identity = qm.Identity
		best.Motion = qm.Motion
		best.Contact = qm.Contact
	}
	return best, nil
}

func main() {
	var (
		percat    = flag.Int("percat", 1, "카테고리당 샘플 키워드 수 (0이면 베이스만)")
		listFlag  = flag.String("keywords", "", "쉼표로 구분된 특정 키워드 목록 (지정 시 percat 무시)")
		dirset    = flag.String("dirset", "", "8방향 세트를 생성할 키워드 (빈 값이면 생략)")
		outDir    = flag.String("out", filepath.Join(os.TempDir(), "ppvalidate"), "출력 디렉토리")
		desc      = flag.String("desc", "a small knight with silver armor and a blue plume on the helmet", "캐릭터 설명")
		styleKey  = flag.String("style", "pixel", "스타일 키")
		timeout   = flag.Duration("timeout", 30*time.Minute, "전체 타임아웃")
		roster    = flag.Int("roster", 0, "로스터 모드: 생성할 캐릭터 수 (다양한 캐릭터×상황×방향)")
		statesPer = flag.Int("statesper", 5, "로스터 모드: 캐릭터당 상태 수")
		batch     = flag.Int("batch", 10, "로스터 모드: 한 배치에서 동시에 생성할 캐릭터 수")
		attempts  = flag.Int("attempts", 3, "상태별 품질 보정 재생성 최대 시도 횟수")
	)
	dump := flag.Bool("dump", false, "프리셋+방향 카탈로그를 JSON으로 출력하고 종료 (UI 검증용 목 데이터)")
	flag.Parse()
	if *attempts >= 1 {
		maxAttempts = *attempts
	}

	if *dump {
		out := map[string]any{
			"presets":    sprite.ListPresets(),
			"directions": sprite.ListDirections(),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}

	if *roster > 0 {
		runRosterMode(*roster, *statesPer, *batch, *outDir, *timeout)
		return
	}
	run(*percat, *listFlag, *dirset, *outDir, *desc, *styleKey, *timeout)
}

// runRosterMode는 프로바이더를 준비하고 로스터(캐릭터 다양성) 검증을 실행합니다.
func runRosterMode(chars, statesPer, batch int, outDir string, timeout time.Duration) {
	s := config.Load()
	cfg := s.Cfg(s.Provider)
	if cfg.APIKey == "" {
		fmt.Printf("키 없음: 프로바이더 %s에 API 키가 설정되지 않았습니다\n", s.Provider)
		os.Exit(1)
	}
	p, err := gen.New(s.Provider, cfg.APIKey, cfg.Model)
	if err != nil {
		fmt.Printf("프로바이더 생성 실패: %v\n", err)
		os.Exit(1)
	}
	model := cfg.Model
	if model == "" {
		model = gen.DefaultModelFor(s.Provider)
	}
	fmt.Printf("로스터 모드 · 프로바이더: %s · 모델: %s · 캐릭터 %d × 상태 %d = 샘플 %d개 · 배치 %d 동시 · 픽셀 스타일 · 출력: %s\n",
		s.Provider, model, chars, statesPer, chars*statesPer, batch, outDir)
	_ = os.MkdirAll(outDir, 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	results := runRoster(ctx, p, chars, statesPer, batch, outDir)
	report(outDir, results)
	writeGallery(outDir, results)
}

func pngBytes(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// selectKeywords는 검증 대상 키워드를 고릅니다.
func selectKeywords(percat int, list string) []sprite.PresetInfo {
	byName := map[string]sprite.PresetInfo{}
	for _, p := range sprite.Presets {
		byName[p.Name] = p
	}
	if strings.TrimSpace(list) != "" {
		var out []sprite.PresetInfo
		for _, n := range strings.Split(list, ",") {
			if p, ok := byName[strings.TrimSpace(n)]; ok {
				out = append(out, p)
			}
		}
		return out
	}
	if percat <= 0 {
		return nil
	}
	count := map[string]int{}
	var out []sprite.PresetInfo
	for _, p := range sprite.Presets {
		if count[p.Category] < percat {
			out = append(out, p)
			count[p.Category]++
		}
	}
	return out
}

func run(percat int, list, dirset, outDir, desc, styleKey string, timeout time.Duration) {
	s := config.Load()
	cfg := s.Cfg(s.Provider)
	if cfg.APIKey == "" {
		fmt.Printf("키 없음: 프로바이더 %s에 API 키가 설정되지 않았습니다\n", s.Provider)
		os.Exit(1)
	}
	p, err := gen.New(s.Provider, cfg.APIKey, cfg.Model)
	if err != nil {
		fmt.Printf("프로바이더 생성 실패: %v\n", err)
		os.Exit(1)
	}
	model := cfg.Model
	if model == "" {
		model = gen.DefaultModelFor(s.Provider)
	}
	fmt.Printf("프로바이더: %s · 모델: %s · 출력: %s\n", s.Provider, model, outDir)
	_ = os.MkdirAll(outDir, 0o755)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	style := sprite.ResolveStyle(styleKey, "")
	palette := sprite.PaletteSizeForStyle(styleKey)

	// 1) 베이스 캐릭터 생성 (키 유효성 검증 겸용)
	t0 := time.Now()
	fmt.Print("베이스 캐릭터 생성 중... ")
	craw, err := p.GenerateImage(ctx, sprite.BuildCharacterPrompt(desc, style), nil, "1:1")
	if err != nil {
		fmt.Printf("실패: %v\n", err)
		os.Exit(1)
	}
	cimg, err := decode(craw)
	if err != nil {
		fmt.Printf("디코딩 실패: %v\n", err)
		os.Exit(1)
	}
	baseClean := sprite.RemoveBackground(cimg)
	if palette > 0 {
		single := []*image.NRGBA{baseClean}
		sprite.PixelPostProcess(single, palette)
		baseClean = single[0]
	}
	savePNG(filepath.Join(outDir, "base.png"), baseClean)
	baseBytes := pngBytes(baseClean)
	fmt.Printf("완료 (%.0fs)\n", time.Since(t0).Seconds())

	var results []stripResult

	// 2) 단일 방향 키워드 샘플 생성
	for _, kw := range selectKeywords(percat, list) {
		spec := sprite.StateSpec{Name: kw.Name, Frames: kw.Frames, FPS: kw.FPS, Loop: kw.Loop, Action: kw.Action}
		ts := time.Now()
		fmt.Printf("[%s] %s 생성 중... ", kw.Category, kw.Name)
		res, err := genStrip(ctx, p, desc, styleKey, style, spec, [][]byte{baseBytes}, baseClean)
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			results = append(results, stripResult{Name: kw.Name, Expected: kw.Frames, Errors: []string{err.Error()}})
			continue
		}
		saveFrames(outDir, res)
		fmt.Printf("%d/%d프레임 시도%d %s (%.0fs)\n", res.Found, res.Expected, res.Attempts, status(res), time.Since(ts).Seconds())
		results = append(results, res)
	}

	// 3) 8방향 세트 (지정 시)
	if strings.TrimSpace(dirset) != "" {
		results = append(results, genDirectionSet(ctx, p, desc, styleKey, style, dirset, baseBytes, baseClean, outDir)...)
	}

	report(outDir, results)
	writeGallery(outDir, results)
}

// writeGallery는 사람이 육안 검수할 수 있는 정적 HTML 갤러리를 outDir/index.html에 만듭니다.
// 각 상태의 애니메이션 GIF + 프레임 스트립 + 품질 점수를 카드로 나열합니다.
func writeGallery(outDir string, results []stripResult) {
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=utf-8><title>PerfectPixel QA</title>")
	b.WriteString("<style>body{background:#16181d;color:#e6e6e6;font:14px system-ui;margin:24px}")
	b.WriteString("h1{font-size:18px}.grid{display:flex;flex-wrap:wrap;gap:16px}")
	b.WriteString(".card{background:#23262e;border-radius:10px;padding:12px;width:230px}")
	b.WriteString(".card img{image-rendering:pixelated;background:#0d0e11 url('data:image/svg+xml;utf8,<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"16\" height=\"16\"><rect width=\"8\" height=\"8\" fill=\"%23222\"/><rect x=\"8\" y=\"8\" width=\"8\" height=\"8\" fill=\"%23222\"/></svg>');border-radius:6px;max-width:206px}")
	b.WriteString(".s{font-weight:700}.ex{color:#7ee787}.go{color:#9ecbff}.fa{color:#e3b341}.po{color:#ff7b72}</style>")
	fmt.Fprintf(&b, "<h1>PerfectPixel 스프라이트 QA · %d개</h1><div class=grid>", len(results))
	cls := func(s int) string {
		switch {
		case s >= 85:
			return "ex"
		case s >= 70:
			return "go"
		case s >= 50:
			return "fa"
		default:
			return "po"
		}
	}
	for _, r := range results {
		gif := filepath.Join(r.rel, r.Name, r.Name+".gif")
		b.WriteString("<div class=card>")
		fmt.Fprintf(&b, "<div><b>%s</b></div>", r.Name)
		fmt.Fprintf(&b, "<img src=\"%s\" alt=\"%s\"><div>", gif, r.Name)
		fmt.Fprintf(&b, "<span class='s %s'>점수 %d</span> · %d/%d · 동일성 %.0f%% · 움직임 %.1f%%</div>",
			cls(r.Score), r.Score, r.Found, r.Expected, r.Identity*100, r.Motion*100)
		if len(r.Errors) > 0 {
			fmt.Fprintf(&b, "<div class=po>%s</div>", strings.Join(r.Errors, "; "))
		}
		b.WriteString("</div>")
	}
	b.WriteString("</div>")
	_ = os.WriteFile(filepath.Join(outDir, "index.html"), []byte(b.String()), 0o644)
	fmt.Printf("갤러리: %s\n", filepath.Join(outDir, "index.html"))
}

func status(r stripResult) string {
	if r.ok() {
		return "OK"
	}
	if r.Found != r.Expected {
		return "프레임수불일치"
	}
	return "품질문제"
}

func saveFrames(outDir string, r stripResult) {
	dir := filepath.Join(outDir, r.Name)
	_ = os.MkdirAll(dir, 0o755)
	if r.rawClean != nil {
		savePNG(filepath.Join(dir, "_strip.png"), r.rawClean)
	}
	for i, f := range r.frames {
		savePNG(filepath.Join(dir, fmt.Sprintf("frame-%02d.png", i)), f)
	}
	// 모션 자연스러움을 사람이 검수할 수 있도록 애니메이션 GIF도 저장
	if len(r.frames) > 0 {
		fps := r.FPS
		if fps <= 0 {
			fps = 8
		}
		if gifBytes, err := sprite.EncodeGIF(r.frames, fps, r.Loop); err == nil {
			_ = os.WriteFile(filepath.Join(dir, r.Name+".gif"), gifBytes, 0o644)
		}
	}
}

// genDirectionSet은 5방향 AI 생성 + 3방향 미러링으로 8방향 세트를 만듭니다.
func genDirectionSet(ctx context.Context, p gen.Provider, desc, styleKey, style, key string,
	baseBytes []byte, baseClean *image.NRGBA, outDir string) []stripResult {

	pre, ok := sprite.PresetByName(key)
	if !ok {
		fmt.Printf("8방향 세트: 알 수 없는 키워드 %q\n", key)
		return nil
	}
	fmt.Printf("=== 8방향 세트: %s ===\n", key)
	var out []stripResult
	frameByDir := map[string][]*image.NRGBA{}
	var southRef []byte

	aiDirs := []string{"south", "east", "north", "south-east", "north-east"}
	for _, d := range aiDirs {
		spec := sprite.StateSpec{Name: key + "-" + d, Frames: pre.Frames, FPS: pre.FPS, Loop: pre.Loop, Action: pre.Action, Facing: d}
		refs := [][]byte{baseBytes}
		if d != "south" && southRef != nil {
			refs = append(refs, southRef)
		}
		var bN *image.NRGBA
		if !sprite.IsBackFacing(d) {
			bN = baseClean
		}
		ts := time.Now()
		fmt.Printf("  [%s] 생성 중... ", d)
		res, err := genStrip(ctx, p, desc, styleKey, style, spec, refs, bN)
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			out = append(out, stripResult{Name: spec.Name, Expected: pre.Frames, Errors: []string{err.Error()}})
			continue
		}
		saveFrames(outDir, res)
		frameByDir[d] = res.frames
		if d == "south" && res.rawClean != nil {
			southRef = pngBytes(res.rawClean)
		}
		fmt.Printf("%d/%d %s (%.0fs)\n", res.Found, res.Expected, status(res), time.Since(ts).Seconds())
		out = append(out, res)
	}

	// 미러 방향: west<-east, south-west<-south-east, north-west<-north-east
	mirror := map[string]string{"west": "east", "south-west": "south-east", "north-west": "north-east"}
	for dst, src := range mirror {
		srcFrames := frameByDir[src]
		if len(srcFrames) == 0 {
			continue
		}
		mres := stripResult{Name: key + "-" + dst, Expected: pre.Frames, Found: len(srcFrames), Attempts: 0, FPS: pre.FPS, Loop: pre.Loop}
		for _, f := range srcFrames {
			mres.frames = append(mres.frames, sprite.MirrorNRGBA(f))
		}
		mres.Motion = sprite.MotionPresence(mres.frames)
		saveFrames(outDir, mres)
		fmt.Printf("  [%s] 미러링(%s) %d프레임\n", dst, src, len(mres.frames))
		out = append(out, mres)
	}
	return out
}

func report(outDir string, results []stripResult) {
	if len(results) == 0 {
		fmt.Println("\n생성된 결과 없음.")
		return
	}
	var pass, frameFail, qualFail, scoreSum int
	fmt.Println("\n=========== 품질 리포트 ===========")
	for _, r := range results {
		fmt.Printf("%-22s %d/%d  점수%3d  동일성%3.0f%%  움직임%4.1f%%  %-12s",
			r.Name, r.Found, r.Expected, r.Score, r.Identity*100, r.Motion*100, status(r))
		if r.Found >= 2 && r.Motion < 0.02 {
			fmt.Print("  ⚠정지")
		}
		if len(r.Errors) > 0 {
			fmt.Printf("  오류:%v", r.Errors)
		}
		if len(r.Warnings) > 0 {
			fmt.Printf("  경고:%d건", len(r.Warnings))
		}
		fmt.Println()
		scoreSum += r.Score
		switch {
		case r.ok():
			pass++
		case r.Found != r.Expected:
			frameFail++
		default:
			qualFail++
		}
	}
	fmt.Println("-----------------------------------")
	avgScore := float64(scoreSum) / float64(len(results))
	fmt.Printf("총 %d개 · 통과 %d · 프레임수불일치 %d · 품질문제 %d · 통과율 %.0f%% · 평균점수 %.1f\n",
		len(results), pass, frameFail, qualFail, 100*float64(pass)/float64(len(results)), avgScore)
	writeJSONReport(outDir, results, avgScore, pass)
}

// writeJSONReport는 기계 판독용 품질 리포트를 outDir/report.json에 씁니다.
func writeJSONReport(outDir string, results []stripResult, avgScore float64, pass int) {
	type row struct {
		Name     string   `json:"name"`
		Expected int      `json:"expected"`
		Found    int      `json:"found"`
		Score    int      `json:"score"`
		Identity float64  `json:"identity"`
		Motion   float64  `json:"motion"`
		Attempts int      `json:"attempts"`
		Status   string   `json:"status"`
		Errors   []string `json:"errors,omitempty"`
		Warnings []string `json:"warnings,omitempty"`
	}
	out := struct {
		Total     int     `json:"total"`
		Pass      int     `json:"pass"`
		PassRate  float64 `json:"passRate"`
		AvgScore  float64 `json:"avgScore"`
		Generated string  `json:"generated"`
		Results   []row   `json:"results"`
	}{Total: len(results), Pass: pass, AvgScore: avgScore, Generated: time.Now().Format(time.RFC3339)}
	if len(results) > 0 {
		out.PassRate = 100 * float64(pass) / float64(len(results))
	}
	for _, r := range results {
		out.Results = append(out.Results, row{
			Name: r.Name, Expected: r.Expected, Found: r.Found, Score: r.Score,
			Identity: r.Identity, Motion: r.Motion, Attempts: r.Attempts,
			Status: status(r), Errors: r.Errors, Warnings: r.Warnings,
		})
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(outDir, "report.json"), data, 0o644)
	fmt.Printf("리포트 저장: %s\n", filepath.Join(outDir, "report.json"))
}
