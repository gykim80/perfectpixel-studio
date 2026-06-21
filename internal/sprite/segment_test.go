package sprite

import (
	"image"
	"testing"
)

// TestSegmentCleanGutters는 깨끗한 골이 있는 스트립에서 자연 포즈 수와 분할이
// 정확한지 검증합니다 (투영 골 분할 경로).
func TestSegmentCleanGutters(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 600, 100))
	fillBox(strip, 20, 20, 79, 79, 200, 100, 50)
	fillBox(strip, 220, 20, 279, 79, 200, 100, 50)
	fillBox(strip, 420, 20, 479, 79, 200, 100, 50)

	segs, natural := segmentStrip(strip, 3)
	if natural != 3 {
		t.Fatalf("자연 포즈 수 오류: %d", natural)
	}
	if len(segs) != 3 {
		t.Fatalf("세그먼트 수 오류: %d", len(segs))
	}
}

// TestSegmentTouchingDP는 두 포즈가 얇은 부위로 닿아 한 런이 됐을 때, prominence
// 봉우리 검출 + DP 최소절단이 두 토르소를 정확히 2개로 분리하는지 검증합니다
// (연결요소 방식이 실패하는, 붙어버린 포즈 케이스).
func TestSegmentTouchingDP(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	// 토르소 A(키 큼) ── 얇은 팔(낮은 질량)로 연결 ── 토르소 B(키 큼): 하나의 런이지만
	// 사이 골이 깊어 두 봉우리로 갈린다.
	fillBox(strip, 40, 10, 110, 89, 200, 100, 50)  // 토르소 A
	fillBox(strip, 110, 48, 250, 55, 200, 100, 50) // 얇은 연결부
	fillBox(strip, 250, 10, 330, 89, 200, 100, 50) // 토르소 B

	res := ExtractFrames(strip, 2, 100, 100, 8)
	if len(res.Frames) != 2 {
		t.Fatalf("닿은 포즈 분리 실패: 프레임 %d개 (기대 2)", len(res.Frames))
	}
	// 두 프레임 모두 실질 콘텐츠가 있어야 함
	for i, f := range res.Frames {
		opaque := 0
		for p := 3; p < len(f.Pix); p += 4 {
			if f.Pix[p] > alphaThreshold {
				opaque++
			}
		}
		if opaque < 500 {
			t.Fatalf("프레임 %d 콘텐츠 부족: %d", i+1, opaque)
		}
	}
}

// TestExtractSlotBoundaryClip은 포즈 A의 꼬리가 포즈 B 슬롯까지 침범할 때,
// 슬롯 경계 클리핑이 꼬리 색상 픽셀을 포즈 B 프레임에서 완전히 배제하는지 검증합니다.
// 꼬리(녹색)와 몸통(파란색)을 다른 색으로 구분해 색상 기반으로 침범 여부를 판정합니다.
func TestExtractSlotBoundaryClip(t *testing.T) {
	// 포즈 A(왼쪽): 파란 몸통(50-120) + 녹색 꼬리(120-230, 포즈 B 슬롯 깊이 침범)
	// 포즈 B(오른쪽): 파란 몸통(260-360)
	// → 포즈 B 프레임에 녹색(꼬리) 픽셀이 단 1개도 없어야 함
	strip := image.NewNRGBA(image.Rect(0, 0, 440, 100))
	fillBox(strip, 50, 10, 120, 89, 60, 80, 200)  // 포즈 A 몸통 (파란색)
	fillBox(strip, 120, 44, 230, 56, 40, 200, 60) // 포즈 A 꼬리 (녹색, 포즈 B 슬롯 깊이 침범)
	fillBox(strip, 260, 10, 360, 89, 60, 80, 200) // 포즈 B 몸통 (파란색)

	res := ExtractFrames(strip, 2, 120, 120, 8)
	if len(res.Frames) != 2 {
		t.Fatalf("프레임 수 오류: %d (기대 2)", len(res.Frames))
	}

	// 포즈 B 프레임에서 녹색(꼬리) 픽셀 검색 — 색상 기반 침범 감지
	frameB := res.Frames[1]
	tailGhosts := 0
	for p := 0; p+3 < len(frameB.Pix); p += 4 {
		if frameB.Pix[p+3] <= alphaThreshold {
			continue
		}
		r, g, b := int(frameB.Pix[p]), int(frameB.Pix[p+1]), int(frameB.Pix[p+2])
		// 녹색 판정: G가 R과 B보다 현저히 높음
		if g > r+80 && g > b+80 {
			tailGhosts++
		}
	}
	if tailGhosts > 0 {
		t.Fatalf("슬롯 경계 클리핑 실패: 포즈 B 프레임에 포즈 A 꼬리(녹색) 유령 픽셀 %d개", tailGhosts)
	}
	// 포즈 B 몸통(파란색)은 충분히 있어야 함
	bodyPixels := 0
	for p := 0; p+3 < len(frameB.Pix); p += 4 {
		if frameB.Pix[p+3] > alphaThreshold {
			bodyPixels++
		}
	}
	if bodyPixels < 500 {
		t.Fatalf("포즈 B 몸통 콘텐츠 부족: %d px", bodyPixels)
	}
}

// TestChromaMatteYCbCr는 마젠타 배경 위 녹색 사각형이 색차 평면 매팅으로
// 올바르게 분리되는지(녹색 불투명, 마젠타 투명) 검증합니다.
func TestChromaMatteYCbCr(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i+3 < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 255, 0, 255, 255 // 마젠타
	}
	fillBox(src, 20, 20, 43, 43, 40, 200, 60) // 녹색 사각형

	out := RemoveBackground(src)
	// 중앙(녹색)은 불투명
	ci := out.PixOffset(32, 32)
	if out.Pix[ci+3] < 200 {
		t.Fatalf("녹색 피사체가 투명 처리됨: alpha=%d", out.Pix[ci+3])
	}
	// 모서리(마젠타)는 투명
	ei := out.PixOffset(2, 2)
	if out.Pix[ei+3] > alphaThreshold {
		t.Fatalf("마젠타 배경이 남음: alpha=%d", out.Pix[ei+3])
	}
}
