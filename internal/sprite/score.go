package sprite

import (
	"image"
	"math"
)

// ScoreResult는 프레임 세트의 quality metric을 담습니다.
type ScoreResult struct {
	Identity float64 `json:"identity"` // 인접 프레임 간 평균 perceptual 유사도 (0~1)
	Motion   float64 `json:"motion"`   // MotionPresence 0~1
	Contact  float64 `json:"contact"`  // 땅선/가장자리 일관성 0~1
	Overall  float64 `json:"overall"`  // 0~1 종합 점수
}

// ScoreFrames는 프레임 세트의 완성도 점수를 계산합니다.
func ScoreFrames(frames []*image.NRGBA) ScoreResult {
	r := ScoreResult{}
	if len(frames) < 2 {
		return r
	}
	r.Motion = MotionPresence(frames)
	r.Identity = pairwiseIdentity(frames)
	r.Contact = contactScore(frames)
	r.Overall = 0.5*r.Identity + 0.3*r.Motion + 0.2*r.Contact
	return r
}

// pairwiseIdentity는 인접 프레임 간 가중 색/알파 차이를 0~1로 정규화합니다.
func pairwiseIdentity(frames []*image.NRGBA) float64 {
	var total float64
	pairs := 0
	for i := 1; i < len(frames); i++ {
		a, b := frames[i-1], frames[i]
		if a.Rect != b.Rect {
			continue
		}
		var diff float64
		var n int
		for p := 0; p+3 < len(a.Pix) && p+3 < len(b.Pix); p += 4 {
			// 색상 거리 인지 가중 + 알파 차이
			dr := float64(int(a.Pix[p]) - int(b.Pix[p]))
			dg := float64(int(a.Pix[p+1]) - int(b.Pix[p+1]))
			db := float64(int(a.Pix[p+2]) - int(b.Pix[p+2]))
			da := float64(int(a.Pix[p+3]) - int(b.Pix[p+3]))
			// 인지 RGB 거리
			d := math.Sqrt(0.299*dr*dr + 0.587*dg*dg + 0.114*db*db)
			d += 0.5 * math.Abs(da)
			if a.Pix[p+3] > alphaThreshold || b.Pix[p+3] > alphaThreshold {
				diff += math.Min(d/(255.0*1.5), 1.0)
				n++
			}
		}
		if n > 0 {
			total += 1.0 - diff/float64(n)
			pairs++
		}
	}
	if pairs == 0 {
		return 0
	}
	return total / float64(pairs)
}

// contactScore는 베이스라인/상단 컨택의 수직 일관성을 측정합니다.
// 캐릭터의 발/머리 높이가 프레임 간 크게 변하면 낮은 점수를 줍니다.
func contactScore(frames []*image.NRGBA) float64 {
	type bounds struct {
		top, bottom, h int
		has            bool
	}
	bbs := make([]bounds, len(frames))
	for i, f := range frames {
		w, h := f.Rect.Dx(), f.Rect.Dy()
		top, bottom := -1, -1
		for y := 0; y < h; y++ {
			rowOpaque := false
			for x := 0; x < w; x++ {
				if f.Pix[f.PixOffset(x, y)+3] > alphaThreshold {
					rowOpaque = true
					break
				}
			}
			if rowOpaque {
				if top < 0 {
					top = y
				}
				bottom = y
			}
		}
		bbs[i] = bounds{top, bottom, h, top >= 0}
	}
	var n int
	meanBottom, meanTop := 0.0, 0.0
	maxH := 1
	for _, b := range bbs {
		if b.h > maxH {
			maxH = b.h
		}
		if b.has {
			meanBottom += float64(b.bottom)
			meanTop += float64(b.top)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	meanBottom /= float64(n)
	meanTop /= float64(n)
	var bottomVar, topVar float64
	for _, b := range bbs {
		if b.has {
			bottomVar += math.Abs(float64(b.bottom) - meanBottom)
			topVar += math.Abs(float64(b.top) - meanTop)
		}
	}
	bottomMAE := bottomVar / float64(n)
	topMAE := topVar / float64(n)
	// 높이 대비 허용 범위: top(머리) 변화는 28% 이내, bottom(발)은 10% 이내.
	// 수영/점프는 발끝을 고정으로 두고 상체가 위아래로 움직이므로 bottom이
	// 안정적일 때 contact가 높아야 한다.
	tolBottom := math.Max(float64(maxH)*0.10, 2.0)
	tolTop := math.Max(float64(maxH)*0.28, 2.0)
	bottomScore := 1.0 - math.Min(bottomMAE/tolBottom, 1.0)
	topScore := 1.0 - math.Min(topMAE/tolTop, 1.0)
	return 0.75*bottomScore + 0.25*topScore
}
