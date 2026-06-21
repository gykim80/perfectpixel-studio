package sprite

import (
	"image"
	"image/color"
)

// 포즈 가이드 색: 타깃 키잉 배경과 동일한 마젠타 위에 검정 스틱맨.
// 흰 배경을 쓰면 모델이 밝은 배경을 따라 그려 배경 제거 단계에서 캐릭터가
// 통째로 지워지는 사례(빈 프레임)가 있어, 가이드도 마젠타로 맞춘다.
var (
	guideBG   = color.NRGBA{255, 0, 255, 255} // 키잉용 마젠타 #FF00FF
	guideInk  = color.NRGBA{20, 20, 20, 255}
	guideNear = color.NRGBA{255, 220, 0, 255} // 가까운(카메라쪽) 다리: 노랑 — 정체성 고정
	guideFar  = color.NRGBA{40, 90, 200, 255} // 먼(몸 뒤) 다리: 파랑 — 정체성 고정
)

// GaitTemplateStrip은 측면 보행 포즈 가이드 스트립을 그립니다.
// N개의 스틱맨 포즈를 한 행에 배치하며, 다리 간격이 한 행에서 두 번
// 벌어졌다 모이도록(=두 스텝) 그려 모델이 "두 보폭" 타이밍을 따라 그리게 합니다.
// 순수 함수(외부 호출 없음)라 오프라인 테스트가 가능합니다.
func GaitTemplateStrip(n, cell int) *image.NRGBA {
	if n < 2 {
		n = 2
	}
	if cell < 32 {
		cell = 32
	}
	w := cell * n
	im := image.NewNRGBA(image.Rect(0, 0, w, cell))
	fill(im, guideBG)
	phases := gaitPhases(n)
	for i, p := range phases {
		drawGuidePose(im, i*cell, cell, p)
	}
	return im
}

// drawGuidePose는 한 셀에 측면 스틱맨 한 포즈를 그립니다.
// 시각 가이드 요소: 빨간 머리-꼭대기선(스케일 앵커), 녹색 지면 허용 밴드,
// 자홍 열 경계선(부속지 침범 금지), 꼬리 호(허용 영역 표시).
func drawGuidePose(im *image.NRGBA, x0, cell int, p gaitPhase) {
	cx := x0 + cell/2
	ground := int(float64(cell) * 0.90)
	hipBase := int(float64(cell) * 0.52)
	stride := int(float64(cell) * 0.22) // 최대 보폭(앞뒤 발 x오프셋)
	headR := cell / 12

	// 위상별 다리 벌림/몸통 높이.
	var frontDX, backDX, swingLift, bob int
	switch p.sub {
	case "contact":
		frontDX, backDX = stride, -stride // 발 최대로 벌어짐
		swingLift, bob = 0, 0
	case "passing":
		frontDX, backDX = stride/5, -stride/6 // 발 모임
		swingLift = int(float64(cell) * 0.06)  // 스윙발 들림
		bob = int(float64(cell) * 0.04)        // 몸 최저 → 살짝 아래
	default: // push
		frontDX, backDX = stride*2/3, -stride*4/5
		swingLift = int(float64(cell) * 0.05)
		bob = -int(float64(cell) * 0.03) // 몸 최고
	}

	hipY := hipBase + bob
	shoulderY := hipY - cell/4
	headCY := shoulderY - headR - 2

	// ── 열 경계선(자홍) ─────────────────────────────────────────────────────
	// 부속지(꼬리/귀/날개/무기)는 이 선 밖으로 나가면 안 된다는 시각적 경고.
	borderColor := color.NRGBA{200, 0, 200, 220}
	for y := int(float64(cell) * 0.05); y < cell-2; y++ {
		setPx(im, x0+2, y, borderColor)
		setPx(im, x0+3, y, borderColor)
		setPx(im, x0+cell-3, y, borderColor)
		setPx(im, x0+cell-2, y, borderColor)
	}

	// ── 지면 허용 밴드(연두) ───────────────────────────────────────────────
	// ±3px 이내에 발이 있어야 함을 시각화.
	groundBandColor := color.NRGBA{100, 220, 100, 180}
	for x := x0 + 4; x < x0+cell-4; x++ {
		for dy := -3; dy <= 4; dy++ {
			if y := ground + dy; y >= 0 && y < cell {
				c := groundBandColor
				if dy == 0 {
					c = color.NRGBA{40, 160, 40, 255} // 지면선 자체는 진한 녹색
				}
				setPx(im, x, y, c)
			}
		}
	}

	// 머리 + 몸통
	drawDisc(im, cx, headCY, headR, guideInk)
	drawThick(im, cx, shoulderY, cx, hipY, 3, guideInk)

	// 다리 기하: 앞다리는 ground, 뒷다리는 push/passing에서 들림.
	frontFootX := cx + frontDX
	backFootX := cx + backDX
	frontFootY, backFootY := ground, ground
	if p.sub == "passing" || p.sub == "push" {
		backFootY = ground - swingLift // 스윙 다리(뒷다리)가 들림
	}

	// 정체성 고정 색칠: 가까운(near) 다리는 RIGHT 다리로 고정해 항상 노랑.
	// step0(RIGHT leads): RIGHT가 앞(front) → 노랑이 앞.
	// step1(LEFT leads): RIGHT가 뒤(back) → 노랑이 뒤.
	// 노랑 다리가 앞→뒤로 이동하며 "발 교대"를 시각적으로 증명한다.
	frontNear := p.lead == "RIGHT"
	frontColor, backColor := guideFar, guideNear
	if frontNear {
		frontColor, backColor = guideNear, guideFar
	}
	// 먼 다리를 먼저(아래에) 그려 가까운 다리가 위에 겹치게 한다.
	if frontNear {
		drawThick(im, cx, hipY, backFootX, backFootY, 3, backColor)
		drawThick(im, cx, hipY, frontFootX, frontFootY, 4, frontColor)
		drawDisc(im, frontFootX, frontFootY, 3, frontColor) // near 발 마커
	} else {
		drawThick(im, cx, hipY, frontFootX, frontFootY, 3, frontColor)
		drawThick(im, cx, hipY, backFootX, backFootY, 4, backColor)
		drawDisc(im, backFootX, backFootY, 3, backColor) // near 발 마커
	}

	// 팔: 다리와 반대로 스윙. 앞다리쪽 반대팔이 앞으로.
	armDX := frontDX
	drawThick(im, cx, shoulderY, cx-armDX, hipY-cell/12, 3, guideInk)
	drawThick(im, cx, shoulderY, cx+backDX/2, hipY-cell/10, 3, guideInk)

	// ── 꼬리 허용 영역 표시(주황 호) ─────────────────────────────────────
	// 오른쪽 보행에서 꼬리는 캐릭터 왼쪽(뒤쪽)에서 나와 안쪽으로 휘어야 함.
	// 허용 영역: 왼쪽 경계선에서 15% 이내에서 시작, 반드시 안쪽으로 되돌아와야 함.
	drawTailAllowanceArc(im, x0, cell, cx, hipY)

	// ── 머리 꼭대기 수평 가이드선(빨강) — 스케일 고정 기준선 ──────────────
	// 모든 셀에서 headCY - headR이 동일 → 캐릭터 높이 불변을 시각적으로 확인.
	headTopY := headCY - headR - 1
	headLineColor := color.NRGBA{220, 60, 60, 220}
	for x := x0 + 1; x < x0+cell-1; x++ {
		setPx(im, x, headTopY, headLineColor)
		if headTopY+1 < cell { // 2px 두께로 더 선명하게
			setPx(im, x, headTopY+1, headLineColor)
		}
	}
}

// drawTailAllowanceArc는 꼬리 허용 영역을 시각화하는 작은 호를 그린다.
// 오른쪽 보행 기준: 꼬리가 왼쪽으로 최대 15% 나간 뒤 반드시 안쪽으로 되돌아와야 함.
func drawTailAllowanceArc(im *image.NRGBA, x0, cell, cx, hipY int) {
	tailColor := color.NRGBA{255, 140, 0, 180} // 주황색
	// 꼬리 기저: 캐릭터 엉덩이 왼쪽
	tailBaseX := cx - cell/6
	// 최대 확장 지점: 왼쪽 경계에서 15% 이내
	maxLeft := x0 + cell/7
	// 작은 'J'형 arc 표시: 왼쪽으로 나갔다가 안쪽으로 되돌아오는 모양
	arcPts := [][2]int{
		{tailBaseX, hipY - cell/10},
		{tailBaseX - cell/8, hipY},
		{maxLeft + 2, hipY + cell/10},
		{maxLeft + cell/12, hipY + cell/6},
		{tailBaseX - cell/10, hipY + cell/5},
	}
	for i := 1; i < len(arcPts); i++ {
		x1, y1 := arcPts[i-1][0], arcPts[i-1][1]
		x2, y2 := arcPts[i][0], arcPts[i][1]
		drawThick(im, x1, y1, x2, y2, 2, tailColor)
	}
	// 화살표 끝: 안쪽으로 향함을 강조
	drawDisc(im, arcPts[len(arcPts)-1][0], arcPts[len(arcPts)-1][1], 2, tailColor)
}

// fill은 이미지를 단색으로 채웁니다.
func fill(im *image.NRGBA, c color.NRGBA) {
	b := im.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			setPx(im, x, y, c)
		}
	}
}

func setPx(im *image.NRGBA, x, y int, c color.NRGBA) {
	if !(image.Point{x, y}).In(im.Bounds()) {
		return
	}
	o := im.PixOffset(x, y)
	im.Pix[o], im.Pix[o+1], im.Pix[o+2], im.Pix[o+3] = c.R, c.G, c.B, c.A
}

// drawDisc는 (cx,cy) 중심 반지름 r의 채워진 원을 그립니다.
func drawDisc(im *image.NRGBA, cx, cy, r int, c color.NRGBA) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				setPx(im, cx+dx, cy+dy, c)
			}
		}
	}
}

// drawThick는 두께 t의 선분을 그립니다 (Bresenham + 정사각 브러시).
func drawThick(im *image.NRGBA, x0, y0, x1, y1, t int, c color.NRGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	half := t / 2
	for {
		for by := -half; by <= half; by++ {
			for bx := -half; bx <= half; bx++ {
				setPx(im, x0+bx, y0+by, c)
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
