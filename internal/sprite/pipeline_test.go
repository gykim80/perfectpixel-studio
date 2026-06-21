package sprite

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// 합성 스트립 생성: 마젠타 배경 + count개의 단색 캐릭터 블롭
func makeSyntheticStrip(w, h, count int, blobColor color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Rect, &image.Uniform{color.NRGBA{255, 0, 255, 255}}, image.Point{}, draw.Src)

	slotW := w / count
	for i := 0; i < count; i++ {
		// 각 슬롯 중앙에 (살짝 다른 높이의) 사각 블롭
		bw, bh := slotW/2, h/2+i*4
		x0 := i*slotW + (slotW-bw)/2
		y0 := h - 20 - bh
		draw.Draw(img, image.Rect(x0, y0, x0+bw, y0+bh), &image.Uniform{blobColor}, image.Point{}, draw.Src)
	}
	return img
}

func TestDetectBackground(t *testing.T) {
	strip := makeSyntheticStrip(800, 200, 4, color.NRGBA{40, 80, 200, 255})
	bg := DetectBackground(strip)
	if bg[0] < 200 || bg[1] > 50 || bg[2] < 200 {
		t.Fatalf("마젠타 배경 감지 실패: %v", bg)
	}
}

func TestRemoveBackgroundAndExtract(t *testing.T) {
	for _, count := range []int{1, 4, 6, 8} {
		strip := makeSyntheticStrip(1200, 260, count, color.NRGBA{40, 80, 200, 255})
		clean := RemoveBackground(strip)

		// 배경이 투명해야 함
		if clean.Pix[3] != 0 {
			t.Fatalf("[%d] 모서리 픽셀이 투명하지 않음", count)
		}

		res := ExtractFrames(clean, count, 256, 256, 24)
		if res.Found != count {
			t.Fatalf("[%d] 프레임 추출 실패: found=%d warnings=%v", count, res.Found, res.Warnings)
		}
		for i, f := range res.Frames {
			if f.Rect.Dx() != 256 || f.Rect.Dy() != 256 {
				t.Fatalf("[%d] 프레임 %d 셀 크기 오류: %v", count, i, f.Rect)
			}
			// 프레임에 실제 콘텐츠가 있어야 함
			has := false
			for p := 3; p < len(f.Pix); p += 4 {
				if f.Pix[p] > 128 {
					has = true
					break
				}
			}
			if !has {
				t.Fatalf("[%d] 프레임 %d 가 비어 있음", count, i)
			}
		}
	}
}

func TestExtractPreservesVerticalOffset(t *testing.T) {
	// 점프 호: 두 번째 블롭이 더 높이 떠 있음
	w, h := 800, 300
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Rect, &image.Uniform{color.NRGBA{255, 0, 255, 255}}, image.Point{}, draw.Src)
	blue := color.NRGBA{40, 80, 200, 255}
	// 블롭1: 바닥
	draw.Draw(img, image.Rect(100, 200, 180, 280), &image.Uniform{blue}, image.Point{}, draw.Src)
	// 블롭2: 공중 (바닥에서 100px 위)
	draw.Draw(img, image.Rect(500, 100, 580, 180), &image.Uniform{blue}, image.Point{}, draw.Src)

	clean := RemoveBackground(img)
	res := ExtractFrames(clean, 2, 256, 256, 24)
	if res.Found != 2 {
		t.Fatalf("프레임 추출 실패: %d", res.Found)
	}

	bottomOf := func(f *image.NRGBA) int {
		for y := f.Rect.Dy() - 1; y >= 0; y-- {
			for x := 0; x < f.Rect.Dx(); x++ {
				if f.Pix[f.PixOffset(x, y)+3] > 128 {
					return y
				}
			}
		}
		return -1
	}
	b0, b1 := bottomOf(res.Frames[0]), bottomOf(res.Frames[1])
	if b1 >= b0 {
		t.Fatalf("수직 오프셋 보존 실패: 지상=%d 공중=%d (공중이 더 위여야 함)", b0, b1)
	}
}

func TestDespillRemovesMagentaFringe(t *testing.T) {
	// 캐릭터 가장자리에 마젠타 프린지가 섞인 경우
	w, h := 400, 200
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Rect, &image.Uniform{color.NRGBA{255, 0, 255, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(150, 50, 250, 150), &image.Uniform{color.NRGBA{40, 200, 80, 255}}, image.Point{}, draw.Src)
	// 프린지 라인 (마젠타 틴트가 섞인 녹색)
	draw.Draw(img, image.Rect(148, 50, 150, 150), &image.Uniform{color.NRGBA{180, 120, 190, 255}}, image.Point{}, draw.Src)

	clean := RemoveBackground(img)
	// 프린지 픽셀이 살아남았다면 마젠타 성향이 줄어야 함
	for y := 50; y < 150; y++ {
		i := clean.PixOffset(148, y)
		if clean.Pix[i+3] == 0 {
			continue // 제거되었으면 OK
		}
		r, g, b := int(clean.Pix[i]), int(clean.Pix[i+1]), int(clean.Pix[i+2])
		if (r+b)/2-g > 80 {
			t.Fatalf("despill 미적용: r=%d g=%d b=%d", r, g, b)
		}
	}
}

func TestComposeAtlasAndManifest(t *testing.T) {
	mk := func(n int) []*image.NRGBA {
		var out []*image.NRGBA
		for i := 0; i < n; i++ {
			f := image.NewNRGBA(image.Rect(0, 0, 64, 64))
			draw.Draw(f, image.Rect(10, 10, 50, 50), &image.Uniform{color.NRGBA{200, 100, 50, 255}}, image.Point{}, draw.Src)
			out = append(out, f)
		}
		return out
	}
	states := []StateFrames{
		{Spec: StateSpec{Name: "idle", FPS: 6, Loop: true}, Frames: mk(4)},
		{Spec: StateSpec{Name: "attack", FPS: 12, Loop: false}, Frames: mk(6)},
	}
	sheet, manifest := ComposeAtlas("hero", states, 64, 64)
	if sheet.Rect.Dx() != 6*64 || sheet.Rect.Dy() != 2*64 {
		t.Fatalf("시트 크기 오류: %v", sheet.Rect)
	}
	if manifest.Animations["attack"].Row != 1 || len(manifest.Animations["attack"].Rects) != 6 {
		t.Fatalf("매니페스트 오류: %+v", manifest.Animations["attack"])
	}
	if manifest.Animations["idle"].FPS != 6 || !manifest.Animations["idle"].Loop {
		t.Fatalf("idle 매니페스트 오류")
	}
}

func TestEncodeGIF(t *testing.T) {
	var frames []*image.NRGBA
	for i := 0; i < 4; i++ {
		f := image.NewNRGBA(image.Rect(0, 0, 64, 64))
		draw.Draw(f, image.Rect(i*10, 10, i*10+20, 40), &image.Uniform{color.NRGBA{uint8(50 * i), 120, 220, 255}}, image.Point{}, draw.Src)
		frames = append(frames, f)
	}
	data, err := EncodeGIF(frames, 8, true)
	if err != nil {
		t.Fatalf("GIF 인코딩 실패: %v", err)
	}
	if len(data) < 100 || string(data[:6]) != "GIF89a" {
		t.Fatalf("GIF 형식 오류 (len=%d)", len(data))
	}
}

func TestBuildStripPrompt(t *testing.T) {
	// cellSize=128 → refMin=32, refMax=64 → "32-64px"
	p := BuildStripPrompt("blue wizard", StylePresets["pixel"], StateSpec{
		Name: "walk", Frames: 6, FPS: 10, Loop: true, Action: "walking",
	}, "make arms bigger", 128)
	for _, want := range []string{"exactly 6", "blue wizard", "magenta", "loops", "make arms bigger", "32-64px", "limited palette"} {
		if !containsFold(p, want) {
			t.Fatalf("프롬프트에 %q 누락", want)
		}
	}
	// cellSize=256 → refMin=64, refMax=128 → "64-128px"
	p256 := BuildStripPrompt("blue wizard", StylePresets["pixel"], StateSpec{
		Name: "walk", Frames: 6, FPS: 10, Loop: true, Action: "walking",
	}, "", 256)
	if !containsFold(p256, "64-128px") {
		t.Fatalf("256px 프롬프트에 '64-128px' 누락")
	}
}

func containsFold(haystack, needle string) bool {
	h, n := []rune(haystack), []rune(needle)
	lower := func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		return r
	}
outer:
	for i := 0; i+len(n) <= len(h); i++ {
		for j := range n {
			if lower(h[i+j]) != lower(n[j]) {
				continue outer
			}
		}
		return true
	}
	return false
}
