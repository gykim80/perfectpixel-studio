package sprite

import (
	"image"
	"testing"
)

// maxOpaqueColHeight는 프레임에서 가장 키 큰 불투명 열의 높이를 반환합니다.
func maxOpaqueColHeight(f *image.NRGBA) int {
	w, h := f.Rect.Dx(), f.Rect.Dy()
	best := 0
	for x := 0; x < w; x++ {
		top, bottom := -1, -1
		for y := 0; y < h; y++ {
			if f.Pix[f.PixOffset(x, y)+3] > alphaThreshold {
				if top < 0 {
					top = y
				}
				bottom = y
			}
		}
		if top >= 0 && bottom-top+1 > best {
			best = bottom - top + 1
		}
	}
	return best
}

// TestExtractBodyExtentScale는 한 프레임의 길게 뻗은 얇은 팔다리(가로 outlier)가
// 전체 스트립 스케일을 과대 산정해 정상 프레임 본체까지 축소시키지 않는지 검증합니다.
//
// 셀 100×100, margin 8 → 가용 84×84. 정상 토르소는 60×60.
// 한 프레임만 가로로 폭 200까지 뻗는 얇은 팔을 가진다.
//   - bbox 기반(구): maxW=200 → scale≈84/200≈0.42 → 정상 토르소 ~25px로 축소.
//   - bodyExtent(신): 질량 80%가 60px 본체에 모여 폭≈60 → scale=1 → 토르소 ~60px 유지.
func TestExtractBodyExtentScale(t *testing.T) {
	// 1000px 폭에 250px 슬롯 4개. 슬롯마다 명확한 마젠타 거터로 분리한다.
	strip := image.NewNRGBA(image.Rect(0, 0, 1000, 100))
	// 정상 프레임 3개: 60×60 토르소 (각 슬롯 중앙)
	fillBox(strip, 95, 20, 154, 79, 200, 100, 50)
	fillBox(strip, 595, 20, 654, 79, 200, 100, 50)
	fillBox(strip, 845, 20, 904, 79, 200, 100, 50)
	// outlier 프레임(슬롯2): 60×60 토르소 + 슬롯 안에서 우측으로 뻗는 얇은(4px) 팔.
	// bbox 폭은 145지만 본체(질량 80%)는 ~56px.
	fillBox(strip, 345, 20, 404, 79, 200, 100, 50)
	fillBox(strip, 405, 47, 490, 50, 200, 100, 50)

	res := ExtractFrames(strip, 4, 100, 100, 8)
	if res.Found != 4 {
		t.Fatalf("프레임 수 오류: %d (%v)", res.Found, res.Warnings)
	}

	// 정상 프레임(팔 없음)의 본체 높이가 outlier에 휘둘려 축소되지 않아야 한다.
	// bbox 기반이면 ~25px, bodyExtent면 ~60px. 임계값 45px로 구분.
	for _, i := range []int{0, 2, 3} {
		hgt := maxOpaqueColHeight(res.Frames[i])
		if hgt < 45 {
			t.Fatalf("프레임 %d 본체가 outlier 때문에 축소됨: height=%d (bbox 스케일 회귀 의심)", i+1, hgt)
		}
	}
}
