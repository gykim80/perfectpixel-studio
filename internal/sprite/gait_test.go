package sprite

import (
	"image"
	"testing"
)

// drawRect는 [x0,x1)×[y0,y1) 영역을 불투명 검정으로 채운다.
func drawRect(im *image.NRGBA, x0, y0, x1, y1 int) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			o := im.PixOffset(x, y)
			im.Pix[o], im.Pix[o+1], im.Pix[o+2], im.Pix[o+3] = 0, 0, 0, 255
		}
	}
}

// synthWalkFrame은 몸통 + 두 발(왼/오)을 그린 64x64 프레임을 만든다.
// lx, rx는 왼발/오른발의 중심 x. 발이 벌어졌다 모이게 하면 stride가 진동한다.
func synthWalkFrame(lx, rx, footY int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	drawRect(im, 28, 8, 36, 44)         // 몸통(중앙 고정)
	drawRect(im, lx-3, 44, lx+3, footY) // 왼발
	drawRect(im, rx-3, 44, rx+3, footY) // 오른발
	return im
}

func TestWalkGait_GoodCycle(t *testing.T) {
	// 두 발이 벌어졌다(contact) 모였다(passing)를 반복하면서, contact마다 들린 발이
	// 뒤→앞→뒤로 교대한다(진짜 보행의 핵심). stride 진동 + lead 교대를 모두 만족.
	frames := []*image.NRGBA{
		synthFootFrame(20, 44, 58, 63), // contact wide, 뒷발 들림(58<63)
		synthFootFrame(30, 36, 62, 62), // passing 모임
		synthFootFrame(20, 44, 63, 58), // contact wide, 앞발 들림(교대)
		synthFootFrame(30, 36, 62, 62), // passing
		synthFootFrame(20, 44, 58, 63), // contact wide, 뒷발 들림
		synthFootFrame(30, 36, 62, 62), // passing
	}
	g := WalkGait(frames)
	if g.StrideRange < 0.2 {
		t.Fatalf("StrideRange 너무 낮음: %.3f", g.StrideRange)
	}
	if g.Oscillations < 2 {
		t.Fatalf("Oscillations 부족: %d", g.Oscillations)
	}
	if g.LeadFlips < 1 {
		t.Fatalf("들린 발 교대 증거 없음: LeadFlips=%d", g.LeadFlips)
	}
	if g.Score < 0.55 {
		t.Fatalf("좋은 보행 점수가 임계값 미만: %.3f", g.Score)
	}
}

func TestWalkGait_StaticFails(t *testing.T) {
	// 같은 발 위치 반복(제자리) → stride 진동 없음 → 낮은 점수.
	frames := []*image.NRGBA{
		synthWalkFrame(28, 36, 60),
		synthWalkFrame(28, 36, 60),
		synthWalkFrame(28, 36, 60),
		synthWalkFrame(28, 36, 60),
		synthWalkFrame(28, 36, 60),
		synthWalkFrame(28, 36, 60),
	}
	g := WalkGait(frames)
	if g.Oscillations != 0 {
		t.Fatalf("정지 보행인데 진동 발생: %d", g.Oscillations)
	}
	if g.Score >= 0.55 {
		t.Fatalf("정지 보행이 통과 점수: %.3f", g.Score)
	}
}

// synthWalkFrameH는 머리 꼭대기(top)를 지정할 수 있는 보행 프레임을 만든다.
// 발 바닥(footY)은 공통 지면선에 고정하고 top만 내리면 "캐릭터 축소" 회귀를 재현한다.
func synthWalkFrameH(lx, rx, top, footY int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	bodyBot := (top + footY) / 2
	drawRect(im, 28, top, 36, bodyBot)       // 몸통
	drawRect(im, lx-3, bodyBot, lx+3, footY) // 왼발
	drawRect(im, rx-3, bodyBot, rx+3, footY) // 오른발
	return im
}

func TestWalkGait_ShrinkFails(t *testing.T) {
	// 좌우 교대(osc)와 지면 일관성은 정상이지만, 보폭이 넓은 contact 프레임에서
	// 캐릭터가 통째로 축소(머리가 내려앉음)된다. 보폭만 보상하던 옛 채점은 이를
	// 고득점 처리했으나, scale 일관성 감점으로 이제 통과 임계값 미만이어야 한다.
	frames := []*image.NRGBA{
		synthWalkFrameH(16, 48, 30, 60), // contact wide, 머리 내려앉음(축소)
		synthWalkFrameH(30, 36, 8, 60),  // passing, 정상 키
		synthWalkFrameH(48, 16, 30, 60), // contact wide(거울상), 축소
		synthWalkFrameH(36, 30, 8, 60),  // passing
		synthWalkFrameH(16, 48, 30, 60), // contact wide, 축소
		synthWalkFrameH(30, 36, 8, 60),  // passing
	}
	g := WalkGait(frames)
	if g.ScaleConsistency >= 0.5 {
		t.Fatalf("축소 보행인데 scale 일관성이 높음: %.3f", g.ScaleConsistency)
	}
	if g.Score >= GaitPassScore {
		t.Fatalf("캐릭터가 축소되는데 통과 점수: %.3f", g.Score)
	}
}

// synthFootFrame은 앞/뒤 발의 바닥 y를 독립 지정하는 보행 프레임을 만든다.
// 두 발이 모두 하단 밴드에 닿되 미세한 높이차로 들림(스윙)을 표현한다.
func synthFootFrame(backX, frontX, backFootY, frontFootY int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	drawRect(im, 28, 8, 36, 44)                  // 몸통
	drawRect(im, backX-3, 44, backX+3, backFootY) // 뒷발
	drawRect(im, frontX-3, 44, frontX+3, frontFootY)
	return im
}

// TestWalkGait_LeadFlips는 들린 발이 앞↔뒤로 교대하면 LeadFlips>0,
// 같은 발만 계속 들리면 LeadFlips==0 임을 확인한다.
func TestWalkGait_LeadFlips(t *testing.T) {
	// 교대: contact마다 들린 발(더 높은=작은 y)이 뒤→앞→뒤로 바뀐다.
	alt := []*image.NRGBA{
		synthFootFrame(20, 44, 58, 63), // 뒷발 들림(58<63)
		synthFootFrame(20, 44, 63, 58), // 앞발 들림
		synthFootFrame(20, 44, 58, 63), // 뒷발 들림
		synthFootFrame(20, 44, 63, 58), // 앞발 들림
	}
	if f := WalkGait(alt).LeadFlips; f < 1 {
		t.Fatalf("교대 보행인데 LeadFlips=%d (>=1 기대)", f)
	}
	// 같은 발: 항상 뒷발만 들림 → 교대 없음.
	same := []*image.NRGBA{
		synthFootFrame(20, 44, 58, 63),
		synthFootFrame(20, 44, 58, 63),
		synthFootFrame(20, 44, 58, 63),
		synthFootFrame(20, 44, 58, 63),
	}
	if f := WalkGait(same).LeadFlips; f != 0 {
		t.Fatalf("같은 발 보행인데 LeadFlips=%d (0 기대)", f)
	}
}

// drawRectGray는 [x0,x1)×[y0,y1) 영역을 불투명 회색(v,v,v)으로 채운다.
func drawRectGray(im *image.NRGBA, x0, y0, x1, y1, v int) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			o := im.PixOffset(x, y)
			im.Pix[o], im.Pix[o+1], im.Pix[o+2], im.Pix[o+3] = uint8(v), uint8(v), uint8(v), 255
		}
	}
}

// synthLumaFootFrame은 평발(같은 footY) 두 발을 발마다 지정 휘도로 그린다.
// 발 높이차가 없으므로 lift(LeadFlips) 신호는 0이고, 오직 휘도(정체성)만으로
// 같은 발 여부를 판정하게 한다.
func synthLumaFootFrame(backX, frontX, backV, frontV int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	drawRect(im, 28, 8, 36, 44) // 몸통(검정)
	drawRectGray(im, backX-3, 44, backX+3, 62, backV)
	drawRectGray(im, frontX-3, 44, frontX+3, 62, frontV)
	return im
}

// TestWalkGait_LumaAlternates는 같은 휘도(정체성) 발이 앞↔뒤로 교대하면
// LumaLeadFlips>0 임을 확인한다(lift가 평발이라 못 잡는 교대를 휘도로 포착).
func TestWalkGait_LumaAlternates(t *testing.T) {
	alt := []*image.NRGBA{
		synthLumaFootFrame(20, 44, 40, 90), // 어두운 발이 뒤
		synthLumaFootFrame(20, 44, 90, 40), // 어두운 발이 앞(교대)
		synthLumaFootFrame(20, 44, 40, 90),
		synthLumaFootFrame(20, 44, 90, 40),
	}
	if f := WalkGait(alt).LumaLeadFlips; f < 1 {
		t.Fatalf("휘도 교대 보행인데 LumaLeadFlips=%d (>=1 기대)", f)
	}
}

// TestWalkGait_LumaSameFootFails는 stride 진동(osc>=2)은 정상이지만 어두운(정체성)
// 발이 매 contact마다 항상 뒤에만 있는 "같은 발" 보행이 통과 점수 미만임을 고정한다.
// osc만으로는 같은 발과 진짜 보행을 구분 못 한다는 회귀(사용자 지적)를 막는 핵심 테스트.
func TestWalkGait_LumaSameFootFails(t *testing.T) {
	same := []*image.NRGBA{
		synthLumaFootFrame(20, 44, 40, 90), // contact: 어두운 발 뒤
		synthLumaFootFrame(30, 36, 90, 90), // passing: 모임
		synthLumaFootFrame(20, 44, 40, 90), // contact: 어두운 발 또 뒤(같은 발)
		synthLumaFootFrame(30, 36, 90, 90),
		synthLumaFootFrame(20, 44, 40, 90),
		synthLumaFootFrame(30, 36, 90, 90),
	}
	g := WalkGait(same)
	if g.Oscillations < 2 {
		t.Fatalf("stride 진동은 있어야 함: osc=%d", g.Oscillations)
	}
	if g.LumaLeadFlips != 0 {
		t.Fatalf("같은 발인데 LumaLeadFlips=%d (0 기대)", g.LumaLeadFlips)
	}
	if g.LeadFlips != 0 {
		t.Fatalf("평발인데 LeadFlips=%d (0 기대)", g.LeadFlips)
	}
	if g.Score >= GaitPassScore {
		t.Fatalf("osc는 있으나 같은 발인데 통과 점수: %.3f", g.Score)
	}
}

func TestWalkGait_TooFewFrames(t *testing.T) {
	g := WalkGait([]*image.NRGBA{synthWalkFrame(20, 44, 60)})
	if g.Score != 0 {
		t.Fatalf("단일 프레임은 0이어야 함: %.3f", g.Score)
	}
}
