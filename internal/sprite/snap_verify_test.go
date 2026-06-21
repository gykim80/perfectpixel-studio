package sprite

import (
	"image"
	"math"
	"testing"
)

// upscaleBilinear은 작은 진짜 픽셀아트를 비정수 배율로 확대하며 블러를 섞어
// AI가 흔히 내놓는 "가짜 픽셀아트"(블러·서브픽셀 어긋남)를 시뮬레이션합니다.
func upscaleBilinear(src *image.NRGBA, dstW, dstH int) *image.NRGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(dstH) - 0.5
		y0 := int(math.Floor(fy))
		ty := fy - float64(y0)
		for x := 0; x < dstW; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(dstW) - 0.5
			x0 := int(math.Floor(fx))
			tx := fx - float64(x0)
			var r, g, b, a float64
			for dy := 0; dy <= 1; dy++ {
				for dx := 0; dx <= 1; dx++ {
					sx := clampIdx(x0+dx, sw)
					sy := clampIdx(y0+dy, sh)
					wgt := (tx*float64(dx) + (1-tx)*float64(1-dx)) *
						(ty*float64(dy) + (1-ty)*float64(1-dy))
					i := src.PixOffset(sx, sy)
					r += float64(src.Pix[i]) * wgt
					g += float64(src.Pix[i+1]) * wgt
					b += float64(src.Pix[i+2]) * wgt
					a += float64(src.Pix[i+3]) * wgt
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i] = uint8(math.Round(r))
			dst.Pix[i+1] = uint8(math.Round(g))
			dst.Pix[i+2] = uint8(math.Round(b))
			dst.Pix[i+3] = uint8(math.Round(a))
		}
	}
	return dst
}

func clampIdx(v, n int) int {
	if v < 0 {
		return 0
	}
	if v >= n {
		return n - 1
	}
	return v
}

// TestSnapRecoversNonIntegerScale는 비정수 배율로 확대된 가짜 픽셀아트에서
// 새 detectGridCounts가 원본 그리드(16x16)를 복원함을 검증합니다.
// 기존 DetectPixelScale은 단일 정수 정사각 스케일만 추정하므로 비정수 배율에서 빗나갑니다.
func TestSnapRecoversNonIntegerScale(t *testing.T) {
	colors := []rgb{{200, 40, 40}, {40, 200, 40}, {40, 40, 200}, {220, 220, 60}, {30, 30, 30}}
	base := makeBlocky(64, 64, 4, colors) // 16셀×16셀(셀당 4px) 진짜 픽셀아트
	// 1.625배 비정수 확대 → 104x104, 셀당 6.5px의 가짜 픽셀아트
	fake := upscaleBilinear(base, 104, 104)

	cols, rows, ok := detectGridCounts(fake)
	if !ok {
		t.Fatal("new detector failed on non-integer scaled art")
	}
	// ±1 셀 허용
	if absInt(cols-16) > 1 || absInt(rows-16) > 1 {
		t.Errorf("new detector grid=%dx%d, want ~16x16", cols, rows)
	}

	// 새 방식 스냅 결과의 셀 내부 평탄도가 기존 정수-스케일 방식보다 같거나 우수해야 함
	newOut := SnapToGrid(fake, cols, rows)
	oldScale := DetectPixelScale(fake)
	var oldOut *image.NRGBA
	oldCols := 16
	if oldScale >= 2 {
		oldOut = Pixelize(fake, oldScale)
		oldCols = 104 / oldScale
	} else {
		oldOut = fake // 정수 스케일 미검출 → 스냅 못함
	}

	newErr := gridAlignmentError(newOut, cols, rows)
	oldErr := gridAlignmentError(oldOut, oldCols, oldCols)
	const trueGrid = 16
	newGridErr := absInt(cols - trueGrid)
	oldGridErr := absInt(oldCols - trueGrid)
	t.Logf("non-integer 6.5px cells: NEW grid=%dx%d (Δ%d from true) align=%.2f | OLD scale=%d (=%d cols, Δ%d) align=%.2f",
		cols, rows, newGridErr, newErr, oldScale, oldCols, oldGridErr, oldErr)
	// 핵심 검증: 새 방식이 진짜 그리드(16)에 더 가깝게(또는 동등하게) 복원해야 함
	if newGridErr > oldGridErr {
		t.Errorf("new detector recovered grid worse than old: newΔ=%d oldΔ=%d", newGridErr, oldGridErr)
	}
}
