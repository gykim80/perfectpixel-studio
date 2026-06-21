package sprite

import (
	"image"
	"testing"
)

// blurEdges는 블록 경계에 AA 노이즈(중간색)를 주입해 "가짜 픽셀아트"를 흉내냅니다.
func blurEdges(img *image.NRGBA, scale int) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x%scale != 0 && y%scale != 0 {
				continue
			}
			i := img.PixOffset(x, y)
			img.Pix[i] = uint8((int(img.Pix[i]) + 128) / 2)
			img.Pix[i+1] = uint8((int(img.Pix[i+1]) + 128) / 2)
			img.Pix[i+2] = uint8((int(img.Pix[i+2]) + 128) / 2)
		}
	}
}

func TestDetectGridCounts(t *testing.T) {
	colors := []rgb{{200, 40, 40}, {40, 200, 40}, {40, 40, 200}, {220, 220, 60}}
	for _, scale := range []int{4, 8, 16} {
		img := makeBlocky(128, 128, scale, colors)
		blurEdges(img, scale)
		cols, rows, ok := detectGridCounts(img)
		if !ok {
			t.Errorf("scale %d: detection failed", scale)
			continue
		}
		want := 128 / scale
		if cols != want || rows != want {
			t.Errorf("scale %d: got %dx%d, want %dx%d", scale, cols, rows, want, want)
		}
	}
}

func TestDetectGridCountsNoise(t *testing.T) {
	// 1px 노이즈 → 신뢰할 그리드 없음
	colors := []rgb{{10, 20, 30}, {200, 100, 50}, {90, 180, 210}, {250, 250, 250}, {120, 60, 200}}
	img := makeBlocky(64, 64, 1, colors)
	if _, _, ok := detectGridCounts(img); ok {
		t.Error("noise image should not detect a grid")
	}
}

func TestSnapToGridUniformCells(t *testing.T) {
	colors := []rgb{{200, 40, 40}, {40, 200, 40}}
	img := makeBlocky(64, 64, 8, colors)
	blurEdges(img, 8)
	out := SnapToGrid(img, 8, 8)
	if out.Rect.Dx() != 64 || out.Rect.Dy() != 64 {
		t.Fatalf("output size changed: %v", out.Rect)
	}
	// 각 8x8 셀이 단일 색이어야 함
	for ry := 0; ry < 8; ry++ {
		for cx := 0; cx < 8; cx++ {
			bx, by := cx*8, ry*8
			f := out.PixOffset(bx, by)
			fr, fg, fb := out.Pix[f], out.Pix[f+1], out.Pix[f+2]
			for dy := 0; dy < 8; dy++ {
				for dx := 0; dx < 8; dx++ {
					i := out.PixOffset(bx+dx, by+dy)
					if out.Pix[i] != fr || out.Pix[i+1] != fg || out.Pix[i+2] != fb {
						t.Fatalf("cell (%d,%d) not uniform", cx, ry)
					}
				}
			}
		}
	}
}

func TestPixelPostProcessGridSnap(t *testing.T) {
	colors := []rgb{{200, 40, 40}, {40, 200, 40}, {40, 40, 200}}
	f := makeBlocky(96, 96, 8, colors)
	blurEdges(f, 8)
	frames := []*image.NRGBA{f}
	PixelPostProcess(frames, 32)
	out := frames[0]
	// 후처리 결과는 8x8 균일 셀이어야 함
	for by := 0; by < 96; by += 8 {
		for bx := 0; bx < 96; bx += 8 {
			ff := out.PixOffset(bx, by)
			fr, fg, fb := out.Pix[ff], out.Pix[ff+1], out.Pix[ff+2]
			for dy := 0; dy < 8; dy++ {
				for dx := 0; dx < 8; dx++ {
					i := out.PixOffset(bx+dx, by+dy)
					if out.Pix[i+3] > alphaThreshold && (out.Pix[i] != fr || out.Pix[i+1] != fg || out.Pix[i+2] != fb) {
						t.Fatalf("block (%d,%d) not uniform after post-process", bx, by)
					}
				}
			}
		}
	}
}
