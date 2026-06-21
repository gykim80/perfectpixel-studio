package sprite

import (
	"image"
	"testing"
)

func filledFrame(x0, y0, x1, y1 int, r, g, b uint8) *image.NRGBA {
	f := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	fillBox(f, x0, y0, x1, y1, r, g, b)
	return f
}

func TestScoreIdenticalFrames(t *testing.T) {
	f := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	fillBox(f, 8, 8, 23, 23, 200, 100, 50)
	frames := []*image.NRGBA{f, f, f}
	s := ScoreFrames(frames)
	if s.Identity < 0.99 {
		t.Fatalf("identical identity too low: %.3f", s.Identity)
	}
	if s.Motion > 1e-6 {
		t.Fatalf("motion should be zero: %.6f", s.Motion)
	}
}

func TestScoreHighMotion(t *testing.T) {
	frames := []*image.NRGBA{
		filledFrame(8, 8, 23, 23, 200, 100, 50),
		filledFrame(12, 10, 27, 25, 200, 100, 50),
		filledFrame(16, 12, 31, 27, 200, 100, 50),
	}
	s := ScoreFrames(frames)
	if s.Motion < 0.05 {
		t.Fatalf("motion too low: %.3f", s.Motion)
	}
	if s.Identity < 0.5 {
		t.Fatalf("identity too low for gradual motion: %.3f", s.Identity)
	}
}

func TestContactConsistentBase(t *testing.T) {
	frames := []*image.NRGBA{
		filledFrame(8, 20, 23, 31, 200, 100, 50),
		filledFrame(8, 20, 23, 31, 200, 100, 50),
	}
	s := ScoreFrames(frames)
	if s.Contact < 0.95 {
		t.Fatalf("contact low: %.3f", s.Contact)
	}
}

func TestContactJitter(t *testing.T) {
	frames := []*image.NRGBA{
		filledFrame(8, 20, 23, 31, 200, 100, 50),
		filledFrame(8, 10, 23, 21, 200, 100, 50),
	}
	s := ScoreFrames(frames)
	if s.Contact > 0.5 {
		t.Fatalf("contact should be low: %.3f", s.Contact)
	}
}

func TestOverallCalibration(t *testing.T) {
	// 정상 모션(motion≥motionFullScale)은 가중치가 온전히 반영돼야 한다.
	// id=0.8, mo=full, co=0.95 → 0.5*0.8+0.3*1+0.2*0.95 = 0.89
	got := overallScore(0.8, motionFullScale, 0.95)
	if got < 0.88 || got > 0.90 {
		t.Fatalf("정상 애니 종합점수 보정 오류: %.3f (기대 ~0.89)", got)
	}
	// 사실상 정지(모션<0.02)는 깨진 애니로 감점돼야 한다.
	still := overallScore(1.0, 0.0, 1.0)
	if still > 0.45 {
		t.Fatalf("동결 클립 감점 안 됨: %.3f", still)
	}
	// 모션 정규화: 큰 모션도 1.0에서 포화(과보상 없음).
	if overallScore(0.7, 0.45, 0.8) != overallScore(0.7, 0.18, 0.8) {
		t.Fatal("모션 포화 실패: motionFullScale 초과가 더 가산됨")
	}
}

func TestContactNaturalVerticalOffset(t *testing.T) {
	// 점프/수영처럼 top만 변하고 bottom은 일정한 경우 contact 손해가 적어야 함
	frames := []*image.NRGBA{
		filledFrame(8, 20, 23, 31, 200, 100, 50),
		filledFrame(8, 12, 23, 31, 200, 100, 50),
		filledFrame(8, 4, 23, 31, 200, 100, 50),
	}
	s := ScoreFrames(frames)
	if s.Contact < 0.85 {
		t.Fatalf("bottom-constant contact should be high: %.3f", s.Contact)
	}
}
