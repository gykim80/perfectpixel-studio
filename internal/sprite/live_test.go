package sprite

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"perfectpixel/internal/gen"
)

// TestPipelineLive는 실제 AI(fal)로 스트립 생성 → 배경 제거 → 프레임 추출까지
// 전체 파이프라인을 검증합니다. PP_LIVE_TEST=1 + FAL_KEY 설정 시에만 실행됩니다.
func TestPipelineLive(t *testing.T) {
	if os.Getenv("PP_LIVE_TEST") != "1" {
		t.Skip("PP_LIVE_TEST=1 설정 시에만 실행")
	}
	key := os.Getenv("FAL_KEY")
	if key == "" {
		t.Skip("FAL_KEY 미설정")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c := gen.NewFal(key, "")

	outDir := filepath.Join(os.TempDir(), "perfectpixel-live")
	_ = os.MkdirAll(outDir, 0o755)

	const expected = 4
	spec := StateSpec{Name: "walk", Frames: expected, FPS: 10, Loop: true, Action: "walking in place, side view"}
	desc := "a small knight with silver armor and a blue plume on the helmet"
	style := StylePresets["pixel"]

	// 앱과 동일한 품질 기반 자동 재시도 로직 (최대 3회)
	feedback := ""
	var found int
	var frames []*image.NRGBA
	var insp InspectResult
	for attempt := 1; attempt <= 3; attempt++ {
		prompt := BuildStripPrompt(desc, style, spec, feedback, 256)
		raw, err := c.GenerateImage(ctx, prompt, nil, AspectForFrames(expected))
		if err != nil {
			t.Fatalf("[시도 %d] 스트립 생성 실패: %v", attempt, err)
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("[시도 %d] PNG 디코딩 실패: %v", attempt, err)
		}
		nimg := ToNRGBA(img)
		bgKey := DetectBackground(nimg)
		clean := RemoveBackground(nimg)
		res := ExtractFrames(clean, expected, 256, 256, 24)
		found = res.Found
		frames = res.Frames
		insp = InspectFrames(frames, bgKey, nil)
		t.Logf("[시도 %d] 추출 %d/%d개, 추출 경고: %v, 품질 오류: %v, 품질 경고: %v",
			attempt, found, expected, res.Warnings, insp.Errors, insp.Warnings)

		savePNG(t, filepath.Join(outDir, fmt.Sprintf("strip-attempt%d.png", attempt)), clean)
		if found == expected && insp.Ok() {
			break
		}
		var fixes []string
		if found != expected {
			fixes = append(fixes, fmt.Sprintf(
				"IMPORTANT CORRECTION: the previous attempt contained %d separate sprites but EXACTLY %d are required. Redraw with exactly %d clearly separated poses, each fully surrounded by magenta.",
				found, expected, expected))
		}
		fixes = append(fixes, insp.RetryHints...)
		feedback = strings.Join(fixes, "\n")
	}

	if found != expected {
		t.Fatalf("자동 재시도 후에도 프레임 수 불일치: %d/%d (출력: %s)", found, expected, outDir)
	}
	if !insp.Ok() {
		t.Fatalf("자동 재시도 후에도 품질 오류 잔존: %v (출력: %s)", insp.Errors, outDir)
	}
	for i, f := range frames {
		savePNG(t, filepath.Join(outDir, fmt.Sprintf("frame-%02d.png", i)), f)
		// 각 프레임에 실제 콘텐츠 픽셀이 충분해야 함 (셀의 1% 이상)
		solid := 0
		for p := 3; p < len(f.Pix); p += 4 {
			if f.Pix[p] > 128 {
				solid++
			}
		}
		if solid < 256*256/100 {
			t.Fatalf("프레임 %d 콘텐츠 부족: %d픽셀", i, solid)
		}
	}
	t.Logf("E2E 성공: %d프레임 추출, 결과물: %s", found, outDir)
}

func savePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("PNG 인코딩 실패: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("파일 저장 실패: %v", err)
	}
}
