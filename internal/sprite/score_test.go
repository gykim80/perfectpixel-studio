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
