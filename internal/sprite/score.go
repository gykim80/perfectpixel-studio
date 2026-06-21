package sprite

import (
	"image"
	"math"
	"strings"
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
	return ScoreFramesForState(frames, "")
}

// ScoreFramesForState는 상태 이름을 고려하여 점수를 계산합니다.
// 점프/낙하처럼 캐릭터가 공중에 뜨는 상태는 발끝 위치 변화에 관대한 허용치를 적용합니다.
func ScoreFramesForState(frames []*image.NRGBA, stateName string) ScoreResult {
	r := ScoreResult{}
	if len(frames) < 2 {
		return r
	}
	r.Motion = MotionPresence(frames)
	r.Identity = pairwiseIdentity(frames)
	r.Contact = contactScoreForState(frames, stateName)
	r.Overall = overallScore(r.Identity, r.Motion, r.Contact)
	return r
}

// motionFullScale은 "확실히 살아있는" 모션으로 간주하는 MotionPresence 값입니다.
// MotionPresence는 인접 프레임 평균 픽셀 변화율로 보통 0.05~0.45 범위에 머뭅니다.
// 이 값으로 0~1 정규화해야 Overall의 Motion 가중치(0.3)가 실제로 반영됩니다.
const motionFullScale = 0.18

// overallScore는 정체성·모션·컨택을 0~1 종합 점수로 합칩니다.
// Motion은 motionFullScale로 정규화하여 가중치가 유효 범위에서 작동하게 합니다.
// 사실상 정지(모션<0.02)인 클립은 깨진 애니메이션으로 보고 감점합니다.
func overallScore(identity, motion, contact float64) float64 {
	motionScore := motion / motionFullScale
	if motionScore > 1 {
		motionScore = 1
	}
	o := 0.5*identity + 0.3*motionScore + 0.2*contact
	if motion < 0.02 {
		o *= 0.6
	}
	return o
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

// isAirborneState는 발끝/머리 수직 위치가 크게 변하는 것이 정상인 상태를 식별합니다.
// 점프·낙하 같은 공중 동작뿐 아니라, 바닥으로 쓰러졌다 일어나는 피해/회복 동작도
// 캐릭터가 의도적으로 지면 높이를 벗어나므로 컨택 허용치를 넓혀
// "둥둥 뜸(floating)" 오류로 부당하게 감점되지 않게 합니다.
func isAirborneState(stateName string) bool {
	base := stripDirectionSuffix(strings.ToLower(strings.TrimSpace(stateName)))
	switch base {
	case "jump", "fall", "fly", "leap", "dodge", "roll",
		"knockback", "knockdown", "death", "death-fall", "get-up", "revive", "slide":
		return true
	}
	return false
}

func contactScore(frames []*image.NRGBA) float64 {
	return contactScoreForState(frames, "")
}

// contactScoreForState는 베이스라인/상단 컨택의 수직 일관성을 측정합니다.
// 공중 상태(jump/fall/fly)는 발끝 허용치를 60%로 넓혀 정상적인 수직 변위를 허용합니다.
func contactScoreForState(frames []*image.NRGBA, stateName string) float64 {
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
	// 공중 상태(점프/낙하/비행)는 발끝이 크게 이동하므로 bottom 허용치를 60%로 확대.
	tolBottomFrac := 0.10
	if isAirborneState(stateName) {
		tolBottomFrac = 0.60
	}
	tolBottom := math.Max(float64(maxH)*tolBottomFrac, 2.0)
	tolTop := math.Max(float64(maxH)*0.28, 2.0)
	bottomScore := 1.0 - math.Min(bottomMAE/tolBottom, 1.0)
	topScore := 1.0 - math.Min(topMAE/tolTop, 1.0)
	return 0.75*bottomScore + 0.25*topScore
}
