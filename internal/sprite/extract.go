package sprite

import (
	"fmt"
	"image"

	xdraw "golang.org/x/image/draw"
)

const alphaThreshold = 10 // 이 값 이하의 알파는 빈(투명) 픽셀로 취급

// frameContent는 스트립 좌표계에서 추출한 한 포즈의 콘텐츠입니다.
type frameContent struct {
	img    *image.NRGBA // bbox로 자른 콘텐츠
	minX   int
	cx     float64 // 알파 가중 질량 중심 X (스트립 좌표)
	bottom int     // 베이스라인(콘텐츠 최하단 행, 스트립 좌표)
}

// extractContent는 컬럼 구간 span 안의 불투명 픽셀을 bbox로 잘라냅니다.
// clipL, clipR는 이 포즈의 "슬롯 경계" (인접 슬롯과의 중간 지점)로,
// span이 이웃 포즈 영역까지 뻗어 있을 때 그 영역의 픽셀을 배제합니다.
// 꼬리·날개·무기 끝이 이웃 슬롯까지 들어와 프레임에 유령처럼 나타나는
// 문제를 차단하는 핵심 경계입니다.
func extractContent(strip *image.NRGBA, span colSpan, h, clipL, clipR int) frameContent {
	// 실제 읽기 범위 = span ∩ [clipL, clipR)
	x0 := span.start
	if x0 < clipL {
		x0 = clipL
	}
	x1 := span.end
	if x1 > clipR {
		x1 = clipR
	}
	if x0 >= x1 {
		return frameContent{}
	}

	minX, minY, maxX, maxY := x1, h, x0-1, -1
	var sumWX, sumW float64
	for x := x0; x < x1; x++ {
		for y := 0; y < h; y++ {
			a := strip.Pix[strip.PixOffset(x, y)+3]
			if a <= alphaThreshold {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			sumWX += float64(x) * float64(a)
			sumW += float64(a)
		}
	}
	if maxX < minX || maxY < minY {
		return frameContent{}
	}
	gw, gh := maxX-minX+1, maxY-minY+1
	dst := image.NewNRGBA(image.Rect(0, 0, gw, gh))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			si := strip.PixOffset(x, y)
			if strip.Pix[si+3] <= alphaThreshold {
				continue
			}
			di := dst.PixOffset(x-minX, y-minY)
			copy(dst.Pix[di:di+4], strip.Pix[si:si+4])
		}
	}
	cx := float64(minX+maxX+1) / 2
	if sumW > 0 {
		cx = sumWX / sumW
	}
	return frameContent{img: dst, minX: minX, cx: cx, bottom: maxY}
}

// ExtractFrames는 투명 배경 스트립에서 포즈를 투영 분할로 검출해 셀 크기 프레임으로
// 만듭니다. 모든 프레임에 공통 스케일을 적용하고, 질량 중심으로 수평 정렬하며,
// 공통 베이스라인 기준으로 수직 오프셋(점프 호 등)을 보존합니다.
func ExtractFrames(strip *image.NRGBA, expected, cellW, cellH, margin int) ExtractResult {
	res := ExtractResult{Expected: expected}
	segs, natural := segmentStrip(strip, expected)
	if len(segs) == 0 {
		res.Warnings = append(res.Warnings, "이미지에서 캐릭터를 찾지 못했습니다. 다시 생성해 주세요.")
		return res
	}
	h := strip.Rect.Dy()
	W := strip.Rect.Dx()

	// 슬롯 경계: 인접 span 사이의 중간 지점을 기준으로 각 포즈의 수집 범위를 제한합니다.
	// 꼬리·날개·무기 끝이 이웃 span까지 뻗어도, 중간 지점 너머의 픽셀은 해당 포즈에
	// 귀속하지 않으므로 이웃 프레임에 유령(ghost)처럼 나타나는 문제를 차단합니다.
	clipL := make([]int, len(segs))
	clipR := make([]int, len(segs))
	for i, s := range segs {
		// 왼쪽 경계: 이전 span과의 gutter 중간점 (또는 스트립 왼쪽 끝)
		if i == 0 {
			clipL[i] = 0
		} else {
			clipL[i] = (segs[i-1].end + s.start) / 2
		}
		// 오른쪽 경계: 다음 span과의 gutter 중간점 (또는 스트립 오른쪽 끝)
		if i == len(segs)-1 {
			clipR[i] = W
		} else {
			clipR[i] = (s.end + segs[i+1].start) / 2
		}
	}

	var fcs []frameContent
	for i, s := range segs {
		fc := extractContent(strip, s, h, clipL[i], clipR[i])
		if fc.img != nil {
			fcs = append(fcs, fc)
		}
	}
	if len(fcs) == 0 {
		res.Warnings = append(res.Warnings, "유효한 포즈를 찾지 못했습니다. 다시 생성해 주세요.")
		return res
	}

	// 공통 베이스라인 + 공유 스케일
	baseline := 0
	for _, g := range fcs {
		if g.bottom > baseline {
			baseline = g.bottom
		}
	}
	availW := cellW - margin*2
	availH := cellH - margin*2
	if availW < 8 || availH < 8 {
		availW, availH = cellW, cellH
	}
	maxBodyW, maxBodyH := 1, 1
	for _, g := range fcs {
		bw, bh := bodyExtent(g.img)
		if bw > maxBodyW {
			maxBodyW = bw
		}
		if bh > maxBodyH {
			maxBodyH = bh
		}
	}
	scale := minf(float64(availW)/float64(maxBodyW), float64(availH)/float64(maxBodyH))
	if scale > 1 {
		scale = 1
	}

	for _, g := range fcs {
		// scale은 body extent 기준으로 계산했으므로, 그 외 바운딩 박스 빈 공간은
		// 여유 공간에 맞춰 조정한다.
		boxScale := minf(scale, minf(float64(availW)/float64(g.img.Rect.Dx()), float64(availH)/float64(g.img.Rect.Dy())))
		if boxScale > 1 {
			boxScale = 1
		}
		// 세로 정렬에서는 실제 발/하체 부분(bottom)이 아니라 콘텐츠 하단 기준 사용
		sw := int(float64(g.img.Rect.Dx())*boxScale + 0.5)
		sh := int(float64(g.img.Rect.Dy())*boxScale + 0.5)
		if sw < 1 {
			sw = 1
		}
		if sh < 1 {
			sh = 1
		}
		scaled := g.img
		if sw != g.img.Rect.Dx() || sh != g.img.Rect.Dy() {
			scaled = image.NewNRGBA(image.Rect(0, 0, sw, sh))
			xdraw.CatmullRom.Scale(scaled, scaled.Rect, g.img, g.img.Rect, xdraw.Over, nil)
		}
		// 공통 baseline 보정을 위해 strip 내 콘텐츠 하단 대비 offset을 scale
		contentBaseline := int(float64(baseline-g.bottom)*boxScale + 0.5)

		cell := image.NewNRGBA(image.Rect(0, 0, cellW, cellH))
		// 질량 중심이 셀 중앙에 오도록 수평 배치 (팔다리가 한쪽으로 뻗어도
		// 면적이 큰 몸통이 지배해 프레임 간 흔들림이 적음).
		left := int(float64(cellW)/2 - (g.cx-float64(g.minX))*boxScale + 0.5)
		if left < 0 {
			left = 0
		}
		if left+sw > cellW {
			left = cellW - sw
		}
		top := cellH - margin - contentBaseline - sh
		if top < 0 {
			top = 0
		}
		xdraw.Copy(cell, image.Point{X: left, Y: top}, scaled, scaled.Rect, xdraw.Over, nil)
		res.Frames = append(res.Frames, cell)
	}

	res.Found = natural
	if natural != expected {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("기대한 %d개와 다른 %d개의 포즈가 감지되었습니다. 포즈가 겹쳤거나 누락됐을 수 있어 재생성을 권장합니다.", expected, natural))
	}
	return res
}

// bodyExtent는 알파 질량 80%를 포함하는 최소 크기를 "실제 바디" extent로 반환합니다.
// 길게 뻗은 팔다리 outlier가 스케일을 과대 산정하는 것을 막습니다.
func bodyExtent(img *image.NRGBA) (int, int) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w == 0 || h == 0 {
		return 1, 1
	}
	alphaX := make([]float64, w)
	alphaY := make([]float64, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := float64(img.Pix[img.PixOffset(x, y)+3])
			alphaX[x] += a
			alphaY[y] += a
		}
	}
	cutX := cumulativeExtent(alphaX, 0.80)
	cutY := cumulativeExtent(alphaY, 0.80)
	if cutX < 1 {
		cutX = 1
	}
	if cutY < 1 {
		cutY = 1
	}
	return cutX, cutY
}

// cumulativeExtent는 질량 누적 비율 massFrac를 커버하는 가장 좁은 연속 구간의 길이를 반환합니다.
func cumulativeExtent(mass []float64, massFrac float64) int {
	total := 0.0
	for _, v := range mass {
		total += v
	}
	if total == 0 {
		return 0
	}
	target := total * massFrac
	n := len(mass)
	best := n
	left := 0
	cur := 0.0
	for right := 0; right < n; right++ {
		cur += mass[right]
		for cur >= target {
			if span := right - left + 1; span < best {
				best = span
			}
			cur -= mass[left]
			left++
		}
	}
	return best
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
