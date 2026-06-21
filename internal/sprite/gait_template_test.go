package sprite

import "testing"

// TestGaitTemplate_StrideOscillates는 가이드 스트립의 발 간격이
// 한 행에서 두 번 벌어졌다 모이는지(두 스텝) 확인한다.
func TestGaitTemplate_StrideOscillates(t *testing.T) {
	n, cell := 6, 64
	im := GaitTemplateStrip(n, cell)
	if im.Bounds().Dx() != n*cell {
		t.Fatalf("폭 불일치: %d", im.Bounds().Dx())
	}
	spreads := make([]float64, n)
	for c := 0; c < n; c++ {
		x0 := c * cell
		minX, maxX := 1<<30, -1
		for y := int(float64(cell) * 0.80); y < cell; y++ {
			for x := x0; x < x0+cell; x++ {
				o := im.PixOffset(x, y)
				r, g, b := im.Pix[o], im.Pix[o+1], im.Pix[o+2]
				ink := r < 60 && g < 60 && b < 60
				yellow := r > 200 && g > 180 && b < 80          // near 다리
				blue := b > 150 && r < 100 && g < 140            // far 다리
				// 다리(잉크/노랑/파랑)만. 마젠타 배경/회색 지면선 제외.
				if ink || yellow || blue {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
				}
			}
		}
		if maxX >= minX {
			spreads[c] = float64(maxX - minX)
		}
	}
	// 평균 교차 횟수가 2 이상이어야 "두 스텝"으로 읽힌다.
	var mean float64
	for _, s := range spreads {
		mean += s
	}
	mean /= float64(n)
	if osc := meanCrossings(spreads, mean); osc < 2 {
		t.Fatalf("가이드 보폭이 두 번 진동하지 않음: osc=%d spreads=%v", osc, spreads)
	}
}

// TestGaitTemplate_NearLegAlternates는 정체성 고정 색칠(near=노랑)이
// 행을 따라 앞→뒤로 교대하는지 확인한다. near 발의 중심대비 x부호가
// 한 번 이상 바뀌어야(앞 리드 → 뒤 트레일) "발 교대"를 가르친다.
func TestGaitTemplate_NearLegAlternates(t *testing.T) {
	n, cell := 6, 64
	im := GaitTemplateStrip(n, cell)
	rel := make([]float64, n)
	for c := 0; c < n; c++ {
		x0 := c * cell
		var sx, cnt float64
		for y := int(float64(cell) * 0.75); y < cell; y++ {
			for x := x0; x < x0+cell; x++ {
				o := im.PixOffset(x, y)
				r, g, b := im.Pix[o], im.Pix[o+1], im.Pix[o+2]
				if r > 200 && g > 180 && b < 80 { // near(노랑) 발
					sx += float64(x - x0)
					cnt++
				}
			}
		}
		if cnt > 0 {
			rel[c] = sx/cnt - float64(cell)/2
		}
	}
	fwd, back := false, false
	for _, v := range rel {
		if v > 2 {
			fwd = true
		}
		if v < -2 {
			back = true
		}
	}
	if !(fwd && back) {
		t.Fatalf("near(노랑) 다리가 앞/뒤로 교대하지 않음: rel=%v", rel)
	}
}
