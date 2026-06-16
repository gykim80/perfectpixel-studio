package sprite

import (
	"fmt"
	"image"
	"sort"
)

// 프레임 품질 검사 파라미터
const (
	inspectEdgeMargin      = 2    // 가장자리 검사 폭(px)
	inspectEdgeMax         = 24   // 이 수를 넘는 가장자리 픽셀은 잘림 위험
	inspectKeyDist         = 70.0 // 배경 키와 이 거리 이내면 잔여 크로마 후보
	inspectKeyMax          = 120  // 잔여 크로마 픽셀 허용 한도
	inspectSmallRatio      = 0.35 // 중앙값 대비 이 비율 미만이면 비정상적으로 작은 프레임
	inspectLargeRatio      = 2.75 // 중앙값 대비 이 비율 초과면 비정상적으로 큰 프레임
	inspectMinContentAbs   = 400  // 프레임당 최소 콘텐츠 픽셀(절대값)
	inspectContentMinAlpha = 0.25 // 전체 프레임 대비 이 비율 미만 불투명이면 강한 공간 절약 가능
	driftWarnSim           = 0.65 // 색 구성 유사도가 이 미만이면 캐릭터 drift 경고
	driftErrorSim          = 0.45 // 이 미만이면 심각한 drift → 재생성 필요
	baseWarnSim            = 0.60 // 베이스 캐릭터 대비 평균 유사도 경고 한도
	baseErrorSim           = 0.40 // 이 미만이면 전 프레임이 베이스와 다른 캐릭터 → 재생성
)

// keyTinted는 픽셀이 배경 키 색조를 띠는지(잔여 크로마/헤일로) 판정합니다.
// 키의 강한 채널(>192)에서 픽셀도 높고(>150), 키의 약한 채널(<64)에서 픽셀도 낮으면(<110)
// 키 색조로 간주합니다. 이렇게 하면 캐릭터의 빨강/파랑 등은 오탐하지 않습니다.
func keyTinted(r, g, b uint8, key [3]uint8) bool {
	px := [3]uint8{r, g, b}
	for c := 0; c < 3; c++ {
		if key[c] > 192 {
			if px[c] <= 150 {
				return false
			}
		} else if key[c] < 64 {
			if px[c] >= 110 {
				return false
			}
		}
	}
	return true
}

// FrameReport는 단일 프레임의 품질 측정값입니다.
type FrameReport struct {
	Index         int     `json:"index"`
	ContentPixels int     `json:"contentPixels"`
	EdgePixels    int     `json:"edgePixels"`
	KeyResidue    int     `json:"keyResidue"`
	PaletteSim    float64 `json:"paletteSim"` // 다른 프레임 대비 색 구성 유사도 (0~1)
}

// InspectResult는 프레임 품질 검사 결과입니다.
type InspectResult struct {
	Reports    []FrameReport
	Errors     []string    // 재생성이 필요한 심각한 문제 (한국어, 사용자 표시용)
	Warnings   []string    // 참고용 경고 (한국어, 사용자 표시용)
	RetryHints []string    // 재생성 프롬프트에 주입할 보정 지시 (영문)
	Score      ScoreResult `json:"score"`
}

// Ok는 심각한 품질 문제가 없으면 true입니다.
func (r InspectResult) Ok() bool { return len(r.Errors) == 0 }

// InspectFrames는 추출된 프레임들의 품질을 검사합니다.
// key는 생성에 사용된 크로마 배경색(잔여 크로마 검출용)입니다.
// base가 nil이 아니면 베이스 캐릭터 대비 정체성 검사도 수행합니다.
// 프레임 간(leave-one-out) 검사는 모든 프레임이 함께 드리프트한 경우를 놓치지만,
// 베이스 대비 검사는 이를 잡아냅니다.
func InspectFrames(frames []*image.NRGBA, key [3]uint8, base *image.NRGBA) InspectResult {
	var res InspectResult
	if len(frames) == 0 {
		return res
	}

	hintSet := map[string]bool{}
	addHint := func(h string) {
		if !hintSet[h] {
			hintSet[h] = true
			res.RetryHints = append(res.RetryHints, h)
		}
	}

	areas := make([]int, 0, len(frames))
	var opaqueTotal int
	for _, f := range frames {
		w, h := f.Rect.Dx(), f.Rect.Dy()
		opaqueTotal += w * h
	}
	contentAlphaCutoff := int(float64(opaqueTotal/len(frames)) * inspectContentMinAlpha)
	for i, f := range frames {
		rep := FrameReport{Index: i}
		w, h := f.Rect.Dx(), f.Rect.Dy()

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				pi := f.PixOffset(x, y)
				if f.Pix[pi+3] <= alphaThreshold {
					continue
				}
				rep.ContentPixels++
				if x < inspectEdgeMargin || x >= w-inspectEdgeMargin ||
					y < inspectEdgeMargin || y >= h-inspectEdgeMargin {
					rep.EdgePixels++
				}
				pr, pg, pb := f.Pix[pi], f.Pix[pi+1], f.Pix[pi+2]
				if colorDist(pr, pg, pb, key) <= inspectKeyDist && keyTinted(pr, pg, pb, key) {
					rep.KeyResidue++
				}
			}
		}

		minContent := inspectMinContentAbs
		if rel := w * h / 100; rel > minContent {
			minContent = rel
		}
		if rep.ContentPixels < minContent {
			res.Errors = append(res.Errors,
				fmt.Sprintf("프레임 %d이(가) 비어있거나 너무 희미합니다 (%d픽셀)", i+1, rep.ContentPixels))
			addHint("Every column must hold one complete, fully drawn full-body character. Leave no column empty or faint.")
		}
		if rep.EdgePixels > inspectEdgeMax {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("프레임 %d이(가) 가장자리에 닿아 잘렸을 수 있습니다 (%d픽셀)", i+1, rep.EdgePixels))
			addHint("Keep every pose fully inside its column with clear padding on all sides; no body part may touch or cross a column edge.")
		}
		if rep.KeyResidue > inspectKeyMax {
			res.Errors = append(res.Errors,
				fmt.Sprintf("프레임 %d에 배경색 잔여물이 남아 있습니다 (%d픽셀)", i+1, rep.KeyResidue))
			addHint("The character must not contain magenta or magenta-adjacent colors anywhere (clothes, effects, highlights). Keep the background a perfectly flat pure magenta #FF00FF and keep all character colors far from magenta.")
		}

		if contentAlphaCutoff > 0 && rep.ContentPixels < contentAlphaCutoff {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("프레임 %d의 콘텐츠가 다른 프레임보다 현저히 적습니다 (오차: %d%%)", i+1, int((1-float64(rep.ContentPixels)/float64(contentAlphaCutoff))*100)))
			addHint("Draw the character at a consistent size across the strip; no pose may be much smaller or partially erased.")
		}

		res.Reports = append(res.Reports, rep)
		areas = append(areas, rep.ContentPixels)
	}

	// 크기 일관성: 중앙값 대비 과소/과대 프레임 감지
	if len(areas) >= 3 {
		sorted := append([]int(nil), areas...)
		sort.Ints(sorted)
		median := sorted[len(sorted)/2]
		if median > 0 {
			for i, a := range areas {
				ratio := float64(a) / float64(median)
				if ratio < inspectSmallRatio {
					res.Warnings = append(res.Warnings,
						fmt.Sprintf("프레임 %d이(가) 다른 프레임보다 비정상적으로 작습니다", i+1))
					addHint("Draw the character at the same scale in every frame; no pose may be much smaller or larger than the others.")
				} else if ratio > inspectLargeRatio {
					res.Warnings = append(res.Warnings,
						fmt.Sprintf("프레임 %d이(가) 다른 프레임보다 비정상적으로 큽니다 (포즈가 병합되었을 수 있음)", i+1))
					addHint("Each pose must be completely separate with clear magenta gaps between poses; poses must never touch, overlap, or merge.")
				}
			}
		}
	}

	// 캐릭터 drift 감지: 포즈와 무관한 색 구성(히스토그램)이 프레임 간 크게 다르면
	// AI가 캐릭터 정체성(머리색/의상 등)을 바꿔 그린 것으로 판정
	if len(frames) >= 2 {
		hists := make([][histBins]float64, len(frames))
		for i, f := range frames {
			hists[i] = colorHistogram(f)
		}
		for i := range frames {
			// leave-one-out: 나머지 프레임 평균과 비교
			var avg [histBins]float64
			for j := range frames {
				if j == i {
					continue
				}
				for k := 0; k < histBins; k++ {
					avg[k] += hists[j][k]
				}
			}
			n := float64(len(frames) - 1)
			sim := 0.0
			for k := 0; k < histBins; k++ {
				sim += minf(hists[i][k], avg[k]/n)
			}
			res.Reports[i].PaletteSim = sim
			if sim < driftErrorSim {
				res.Errors = append(res.Errors,
					fmt.Sprintf("프레임 %d의 색 구성이 다른 프레임과 크게 다릅니다 (캐릭터 변형 의심, 유사도 %.0f%%)", i+1, sim*100))
				addHint("CRITICAL: keep the exact same character identity in every frame — identical hair color, skin tone, outfit colors and proportions. Only the pose may change between frames.")
			} else if sim < driftWarnSim {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("프레임 %d의 색 구성이 다른 프레임과 다소 다릅니다 (유사도 %.0f%%)", i+1, sim*100))
				addHint("Keep the character's colors and details consistent across all frames; do not change hair, skin or outfit colors between poses.")
			}
		}
	}

	// 베이스 캐릭터 대비 정체성 검사: 프레임 평균 색 구성이 베이스와 크게 다르면
	// 전체가 다른 캐릭터로 그려진 것 (개별 프레임 검사로는 잡히지 않는 일괄 드리프트)
	if base != nil && len(frames) > 0 && hasTransparency(base) {
		baseHist := colorHistogram(base)
		totalSim := 0.0
		for _, f := range frames {
			h := colorHistogram(f)
			sim := 0.0
			for k := 0; k < histBins; k++ {
				sim += minf(h[k], baseHist[k])
			}
			totalSim += sim
		}
		avg := totalSim / float64(len(frames))
		if avg < baseErrorSim {
			res.Errors = append(res.Errors,
				fmt.Sprintf("생성된 프레임들이 베이스 캐릭터와 크게 다릅니다 (유사도 %.0f%%)", avg*100))
			res.RetryHints = append(res.RetryHints,
				"CRITICAL: the previous attempt drew a different-looking character. Copy the attached reference image's identity exactly — identical hair color, skin tone, outfit colors, proportions and accessories in every frame.")
		} else if avg < baseWarnSim {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("생성된 프레임들의 색 구성이 베이스 캐릭터와 다소 다릅니다 (유사도 %.0f%%)", avg*100))
		}
	}

	// 프레임 수/검사 시점에서도 점수를 계산해 InspectResult에 포함
	res.Score = ScoreFrames(frames)

	return res
}

// MotionPresence는 인접 프레임 간 평균 변화율(0~1)을 반환합니다.
// 같은 크기의 두 프레임에서 한쪽이라도 불투명한 픽셀을 대상으로 정규화된 RGBA 차이를
// 평균냅니다. 0에 가까우면 사실상 정지 화면(실제 움직임 없음)으로, idle/sleep 같은
// 의도적 미동을 제외하면 "애니메이션이 움직이지 않는" 결함 신호입니다.
// InspectFrames의 drift 검사(프레임이 너무 다름)와 정반대 축(너무 같음)을 측정합니다.
func MotionPresence(frames []*image.NRGBA) float64 {
	if len(frames) < 2 {
		return 0
	}
	var total float64
	pairs := 0
	for i := 1; i < len(frames); i++ {
		a, b := frames[i-1], frames[i]
		if a.Rect != b.Rect {
			continue
		}
		var diffSum float64
		var count int
		for p := 0; p+3 < len(a.Pix) && p+3 < len(b.Pix); p += 4 {
			aa, ba := a.Pix[p+3], b.Pix[p+3]
			if aa <= alphaThreshold && ba <= alphaThreshold {
				continue
			}
			d := absDiff(a.Pix[p], b.Pix[p]) + absDiff(a.Pix[p+1], b.Pix[p+1]) +
				absDiff(a.Pix[p+2], b.Pix[p+2]) + absDiff(aa, ba)
			diffSum += float64(d) / (255.0 * 4.0)
			count++
		}
		if count > 0 {
			total += diffSum / float64(count)
			pairs++
		}
	}
	if pairs == 0 {
		return 0
	}
	return total / float64(pairs)
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// hasTransparency는 이미지에 의미 있는 투명 영역이 있는지 확인합니다.
// 불투명 배경 이미지(사진 등)는 배경색이 히스토그램을 오염시켜
// 베이스 대비 검사가 오탐을 내므로 검사 대상에서 제외합니다.
func hasTransparency(img *image.NRGBA) bool {
	total, transparent := 0, 0
	for i := 3; i < len(img.Pix); i += 4 {
		total++
		if img.Pix[i] <= alphaThreshold {
			transparent++
		}
	}
	return total > 0 && float64(transparent)/float64(total) >= 0.05
}

const histBins = 64 // 4×4×4 RGB 양자화 빈

// colorHistogram은 불투명 픽셀의 정규화된 coarse RGB 히스토그램을 계산합니다.
// 포즈가 달라도 같은 캐릭터면 분포가 유사하다는 점을 이용합니다.
func colorHistogram(f *image.NRGBA) [histBins]float64 {
	var hist [histBins]float64
	total := 0
	for i := 0; i+3 < len(f.Pix); i += 4 {
		if f.Pix[i+3] <= alphaThreshold {
			continue
		}
		bin := int(f.Pix[i]>>6)<<4 | int(f.Pix[i+1]>>6)<<2 | int(f.Pix[i+2]>>6)
		hist[bin]++
		total++
	}
	if total > 0 {
		for k := range hist {
			hist[k] /= float64(total)
		}
	}
	return hist
}
