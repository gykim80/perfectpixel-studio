package sprite

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func makeSolidFrame(w, h int, drawFn func(img *image.NRGBA)) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.NRGBA{0, 0, 0, 0}}, image.Point{}, draw.Src)
	drawFn(img)
	return img
}

func fillR(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

// TestTailBlobFilter는 꼬리가 지면 밴드에 들어와도 발 식별이 올바른지 확인한다.
// 꼬리가 없으면 lumaLeadFlips=1 (앞발/뒷발 밝기 교대), 꼬리가 있어도 동일해야 한다.
func TestTailBlobFilter(t *testing.T) {
	light := color.NRGBA{200, 200, 200, 255}
	dark := color.NRGBA{80, 80, 80, 255}
	body := color.NRGBA{150, 150, 150, 255}
	tail := color.NRGBA{150, 100, 50, 255}

	makeContact := func(rightLighter bool, withTail bool) *image.NRGBA {
		return makeSolidFrame(256, 256, func(img *image.NRGBA) {
			fillR(img, 108, 10, 148, 210, body) // 몸통
			var leftC, rightC color.NRGBA
			if rightLighter {
				rightC, leftC = light, dark
			} else {
				rightC, leftC = dark, light
			}
			fillR(img, 138, 218, 158, 240, rightC) // 오른발(앞쪽)
			fillR(img, 98, 218, 118, 240, leftC)   // 왼발(뒤쪽)
			if withTail {
				// 꼬리: 신체 중심(~128)에서 약 100px 왼쪽 → cx≈28, |28-128|=100 > h*0.42≈99
				fillR(img, 15, 225, 30, 240, tail)
			}
		})
	}
	makePassing := func() *image.NRGBA {
		return makeSolidFrame(256, 256, func(img *image.NRGBA) {
			fillR(img, 108, 10, 148, 240, body) // 발이 모인 통과 포즈
		})
	}

	// 꼬리 없는 경우: step1(오른발앞밝음) passing step2(왼발앞밝음) passing
	framesNoTail := []*image.NRGBA{
		makeContact(true, false),
		makePassing(),
		makePassing(),
		makeContact(false, false),
		makePassing(),
		makePassing(),
	}
	gNoTail := WalkGait(framesNoTail)

	// 꼬리 있는 경우: 동일한 패턴
	framesTail := []*image.NRGBA{
		makeContact(true, true),
		makePassing(),
		makePassing(),
		makeContact(false, true),
		makePassing(),
		makePassing(),
	}
	gTail := WalkGait(framesTail)

	t.Logf("NoTail: lflip=%d flip=%d score=%.3f", gNoTail.LumaLeadFlips, gNoTail.LeadFlips, gNoTail.Score)
	t.Logf("Tail:   lflip=%d flip=%d score=%.3f", gTail.LumaLeadFlips, gTail.LeadFlips, gTail.Score)

	if gNoTail.LumaLeadFlips < 1 {
		t.Errorf("꼬리 없는 경우 lumaLeadFlips=%d, 1 이상이어야 함", gNoTail.LumaLeadFlips)
	}
	if gTail.LumaLeadFlips < 1 {
		t.Errorf("꼬리 있는 경우 lumaLeadFlips=%d, 꼬리 필터 후 1 이상이어야 함", gTail.LumaLeadFlips)
	}
}

// TestLumaSameSide는 밝은 발이 항상 같은 쪽에 있을 때 LumaSameSide=true를 반환하는지 확인한다.
func TestLumaSameSide(t *testing.T) {
	light := color.NRGBA{200, 200, 200, 255}
	dark := color.NRGBA{80, 80, 80, 255}
	body := color.NRGBA{150, 150, 150, 255}

	makeContact := func(rightLighter bool) *image.NRGBA {
		return makeSolidFrame(256, 256, func(img *image.NRGBA) {
			fillR(img, 108, 10, 148, 210, body)
			var leftC, rightC color.NRGBA
			if rightLighter {
				rightC, leftC = light, dark
			} else {
				rightC, leftC = dark, light
			}
			fillR(img, 138, 218, 158, 240, rightC)
			fillR(img, 98, 218, 118, 240, leftC)
		})
	}
	makePassing := func() *image.NRGBA {
		return makeSolidFrame(256, 256, func(img *image.NRGBA) {
			fillR(img, 108, 10, 148, 240, body)
		})
	}

	// 같은 발 반복: rightLighter=true 4회 → 앞발(오른쪽)이 항상 밝음 → LumaSameSide=true
	sameFootFrames := []*image.NRGBA{
		makeContact(true), makePassing(),
		makeContact(true), makePassing(),
		makeContact(true), makePassing(),
	}
	gSameFoot := WalkGait(sameFootFrames)
	t.Logf("SameFoot: lflip=%d sameSide=%v score=%.3f", gSameFoot.LumaLeadFlips, gSameFoot.LumaSameSide, gSameFoot.Score)
	if !gSameFoot.LumaSameSide {
		t.Errorf("같은 발 반복(앞발 항상 밝음): LumaSameSide=%v, true여야 함", gSameFoot.LumaSameSide)
	}
	if gSameFoot.LumaLeadFlips != 0 {
		t.Errorf("같은 발 반복: LumaLeadFlips=%d, 0이어야 함", gSameFoot.LumaLeadFlips)
	}

	// 올바른 교대: rightLighter true→false 교대 → LumaSameSide=false
	altFrames := []*image.NRGBA{
		makeContact(true), makePassing(),
		makeContact(false), makePassing(),
		makeContact(true), makePassing(),
	}
	gAlt := WalkGait(altFrames)
	t.Logf("Alternating: lflip=%d sameSide=%v score=%.3f", gAlt.LumaLeadFlips, gAlt.LumaSameSide, gAlt.Score)
	if gAlt.LumaSameSide {
		t.Errorf("올바른 교대: LumaSameSide=%v, false여야 함", gAlt.LumaSameSide)
	}
	if gAlt.LumaLeadFlips < 1 {
		t.Errorf("올바른 교대: LumaLeadFlips=%d, 1 이상이어야 함", gAlt.LumaLeadFlips)
	}
}

// TestDarkSpriteLumaFlip는 매우 어두운 픽셀아트(L<20)에서도 luma 교대를 감지하는지 확인한다.
// threshold를 5→3으로 낮춘 덕에 luma 차이가 3-4인 어두운 다리도 감지해야 한다.
func TestDarkSpriteLumaFlip(t *testing.T) {
	// L=13 near(lighter), L=10 far(darker) → 차이=3, 구 임계값(5)에서는 감지 안 됨
	nearDark := color.NRGBA{13, 13, 13, 255}
	farDark := color.NRGBA{10, 10, 10, 255}
	body := color.NRGBA{15, 15, 15, 255}

	makeContactDark := func(rightLighter bool) *image.NRGBA {
		return makeSolidFrame(256, 256, func(img *image.NRGBA) {
			fillR(img, 108, 10, 148, 210, body)
			var leftC, rightC color.NRGBA
			if rightLighter {
				rightC, leftC = nearDark, farDark
			} else {
				rightC, leftC = farDark, nearDark
			}
			fillR(img, 138, 218, 158, 240, rightC)
			fillR(img, 98, 218, 118, 240, leftC)
		})
	}
	makePassingDark := func() *image.NRGBA {
		return makeSolidFrame(256, 256, func(img *image.NRGBA) {
			fillR(img, 108, 10, 148, 240, body)
		})
	}

	frames := []*image.NRGBA{
		makeContactDark(true),
		makePassingDark(),
		makePassingDark(),
		makeContactDark(false),
		makePassingDark(),
		makePassingDark(),
	}
	g := WalkGait(frames)
	t.Logf("DarkSprite: lflip=%d flip=%d score=%.3f strideRange=%.2f", g.LumaLeadFlips, g.LeadFlips, g.Score, g.StrideRange)
	if g.LumaLeadFlips < 1 {
		t.Errorf("어두운 스프라이트(L≈10-13): lumaLeadFlips=%d, 1 이상이어야 함 (threshold 3 이하)", g.LumaLeadFlips)
	}
}
