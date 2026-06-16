package sprite

import (
	"image"
	"testing"
)

// TestSegmentForceExpectedOnOverlap는 AI가 마젠타 gutter 없이 포즈를 겹쳐 그려도
// expected 개수만큼 DP 분할로 복구하는지 검증합니다.
func TestSegmentForceExpectedOnOverlap(t *testing.T) {
	strip := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	fillBox(strip, 40, 20, 140, 79, 200, 100, 50)
	fillBox(strip, 120, 20, 220, 79, 200, 100, 50)
	segs, natural := segmentStrip(strip, 2)
	if natural != 2 {
		t.Fatalf("natural=%d segs=%v", natural, segs)
	}
}
