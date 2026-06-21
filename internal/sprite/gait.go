package sprite

import (
	"fmt"
	"image"
	"math"
)

// GaitResult는 측면 보행(walk/run) 스트립의 보행 품질 측정값입니다.
// 핵심 가설: 올바른 보행은 (1) 두 발이 벌어졌다 모이며 stride가 진동하고,
// (2) 다리 영역이 프레임마다 충분히 변하며, (3) 발이 공통 지면선 위에 머물고,
// (4) 들린(스윙) 발의 좌우가 사이클 동안 교대한다.
// "같은 발만 반복" 실패는 stride 진동과 lead 교대가 모두 사라지는 형태로 나타난다.
type GaitResult struct {
	StrideRange      float64 `json:"strideRange"`      // (maxStride-minStride)/bodyWidth, 발 벌림 진폭
	Oscillations     int     `json:"oscillations"`     // stride 시퀀스가 평균을 가로지른 횟수(스텝 수 근사)
	LegMotion        float64 `json:"legMotion"`        // 다리 영역 인접 프레임 변화 0~1
	FootGround       float64 `json:"footGround"`       // 발 높이(지면선) 일관성 0~1
	LeadFlips        int     `json:"leadFlips"`        // 들린 발(스윙)의 앞/뒤가 바뀐 횟수
	LumaLeadFlips    int     `json:"lumaLeadFlips"`    // 같은 휘도(정체성) 발이 앞↔뒤로 바뀐 횟수
	LumaSameSide     bool    `json:"lumaSameSide"`     // 밝은 발이 항상 앞 OR 항상 뒤 (같은 발 반복의 강한 신호)
	ScaleConsistency float64 `json:"scaleConsistency"` // 프레임 간 캐릭터 크기/머리높이 일관성 0~1 (1=축소 없음)
	Score            float64 `json:"score"`            // 0~1 종합 보행 점수
	Frames           int     `json:"frames"`
}

// frameGait는 한 프레임의 하반신 기하 디스크립터입니다.
type frameGait struct {
	has        bool
	cx         float64 // 전체 불투명 픽셀 x 무게중심
	stride     float64 // 발 영역 좌우 폭 (footMaxX-footMinX)
	bodyW      float64 // 전체 폭
	topY       float64 // 캐릭터 최상단 y (머리 꼭대기). 작을수록 위.
	height     float64 // 캐릭터 전체 높이 (maxY-minY+1)
	leftFootY  float64 // 무게중심 왼쪽 다리의 최저(지면쪽) y, 클수록 지면
	rightFootY float64 // 무게중심 오른쪽 다리의 최저 y
	footBaseY  float64 // 발 영역 최저 y (지면선)

	twoFeet    bool    // 얇은 지면 밴드에서 두 발이 분리되어 보임
	frontFootX float64 // 앞발(진행방향=오른쪽) x중심
	backFootX  float64 // 뒷발 x중심
	frontFootY float64 // 앞발 최저 y (클수록 지면에 붙음)
	backFootY  float64 // 뒷발 최저 y
	frontFootL float64 // 앞발 평균 휘도 (다리 정체성 단서: 가까운/먼 다리 음영)
	backFootL  float64 // 뒷발 평균 휘도
}

// footBandBlobs는 프레임 하단 얇은 밴드(키의 bandFrac)에서 분리된 발 덩어리의
// x중심과 최저y를 반환합니다. 갠 2px 초과면 발이 분리된 것으로 봅니다.
// 측면 보행에서 두 발이 벌어진 contact 포즈는 두 덩어리로, 모인 passing 포즈는
// 한 덩어리로 읽혀, 들린 발(스윙)의 좌우 교대를 추정하는 1차 재료가 됩니다.
func footBandBlobs(f *image.NRGBA, minX, maxX, minY, maxY int, bandFrac float64) (blobsX, blobsY, blobsL []float64) {
	h := float64(maxY - minY + 1)
	bandTop := float64(maxY) - bandFrac*h
	width := maxX - minX + 1
	if width <= 0 {
		return nil, nil, nil
	}
	occ := make([]bool, width)
	colBase := make([]int, width)
	colLuma := make([]float64, width)
	colN := make([]float64, width)
	for x := minX; x <= maxX; x++ {
		for y := minY; y <= maxY; y++ {
			if float64(y) < bandTop {
				continue
			}
			o := f.PixOffset(x, y)
			if f.Pix[o+3] <= alphaThreshold {
				continue
			}
			occ[x-minX] = true
			if y > colBase[x-minX] {
				colBase[x-minX] = y
			}
			colLuma[x-minX] += 0.299*float64(f.Pix[o]) + 0.587*float64(f.Pix[o+1]) + 0.114*float64(f.Pix[o+2])
			colN[x-minX]++
		}
	}
	gap, runStart := 0, -1
	flush := func(end int) {
		if runStart < 0 {
			return
		}
		var sx, sl, sn float64
		base := 0
		for i := runStart; i <= end; i++ {
			sx += float64(minX + i)
			sl += colLuma[i]
			sn += colN[i]
			if colBase[i] > base {
				base = colBase[i]
			}
		}
		cnt := float64(end - runStart + 1)
		blobsX = append(blobsX, sx/cnt)
		blobsY = append(blobsY, float64(base))
		if sn > 0 {
			blobsL = append(blobsL, sl/sn)
		} else {
			blobsL = append(blobsL, 0)
		}
		runStart = -1
	}
	last := -1
	for i := 0; i < width; i++ {
		if occ[i] {
			if runStart < 0 {
				runStart = i
			}
			last = i
			gap = 0
		} else if runStart >= 0 {
			gap++
			if gap > 2 {
				flush(last)
			}
		}
	}
	flush(last)
	return blobsX, blobsY, blobsL
}

// analyzeFrameGait는 한 프레임에서 하반신 디스크립터를 추출합니다.
func analyzeFrameGait(f *image.NRGBA) frameGait {
	b := f.Bounds()
	minX, maxX := math.MaxInt32, math.MinInt32
	minY, maxY := math.MaxInt32, math.MinInt32
	var sumX, nOpaque float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if f.Pix[f.PixOffset(x, y)+3] <= alphaThreshold {
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
			sumX += float64(x)
			nOpaque++
		}
	}
	if nOpaque == 0 {
		return frameGait{}
	}
	cx := sumX / nOpaque
	h := float64(maxY - minY + 1)
	footTopY := float64(maxY) - 0.18*h // 발 영역: 하단 18%
	legsTopY := float64(maxY) - 0.50*h // 다리 영역: 하단 50%

	footMinX, footMaxX := math.MaxInt32, math.MinInt32
	leftFootY, rightFootY := math.Inf(-1), math.Inf(-1)
	footBaseY := math.Inf(-1)
	for y := minY; y <= maxY; y++ {
		fy := float64(y)
		if fy < legsTopY {
			continue
		}
		inFoot := fy >= footTopY
		for x := minX; x <= maxX; x++ {
			if f.Pix[f.PixOffset(x, y)+3] <= alphaThreshold {
				continue
			}
			if inFoot {
				// 꼬리/부속지 픽셀은 보폭 측정에서 제외한다 (횡방향으로 신체 높이의 42% 이상이면 꼬리로 간주).
				if math.Abs(float64(x)-cx) <= h*0.42 {
					if x < footMinX {
						footMinX = x
					}
					if x > footMaxX {
						footMaxX = x
					}
				}
				if fy > footBaseY {
					footBaseY = fy
				}
			}
			if float64(x) < cx {
				if fy > leftFootY {
					leftFootY = fy
				}
			} else if fy > rightFootY {
				rightFootY = fy
			}
		}
	}
	stride := 0.0
	if footMaxX >= footMinX {
		stride = float64(footMaxX - footMinX)
	}
	g := frameGait{
		has: true, cx: cx, stride: stride, bodyW: float64(maxX - minX + 1),
		topY: float64(minY), height: h,
		leftFootY: leftFootY, rightFootY: rightFootY, footBaseY: footBaseY,
	}
	// 얇은 지면 밴드에서 두 발 분리 시 앞/뒤 발의 위치·높이·휘도를 기록한다.
	bx, by, bl := footBandBlobs(f, minX, maxX, minY, maxY, 0.12)
	// 꼬리/날개 등 부속지가 지면 밴드를 침범할 때 발로 오인되는 오류를 방지한다.
	// 발은 신체 높이의 ±42% 이내에 있어야 한다(꼬리는 신체 중심보다 훨씬 멀리 있음).
	if len(bx) > 2 {
		maxLateral := h * 0.42
		var fx2, fy2, fl2 []float64
		for i := range bx {
			if math.Abs(bx[i]-cx) <= maxLateral {
				fx2 = append(fx2, bx[i])
				fy2 = append(fy2, by[i])
				fl2 = append(fl2, bl[i])
			}
		}
		if len(fx2) >= 2 {
			bx, by, bl = fx2, fy2, fl2
		}
	}
	if len(bx) >= 2 {
		// 가장 왼쪽=뒷발, 가장 오른쪽=앞발(진행 방향=오른쪽).
		bi, fi := 0, 0
		for i := range bx {
			if bx[i] < bx[bi] {
				bi = i
			}
			if bx[i] > bx[fi] {
				fi = i
			}
		}
		g.twoFeet = true
		g.backFootX, g.backFootY, g.backFootL = bx[bi], by[bi], bl[bi]
		g.frontFootX, g.frontFootY, g.frontFootL = bx[fi], by[fi], bl[fi]
	}
	return g
}

// GaitPassScore는 보행 상태가 통과로 간주되는 최소 gait 점수입니다.
const GaitPassScore = 0.55

// GaitRetryHint는 gait 검증 실패 유형에 맞춘 영문 재생성 지시를 만듭니다.
// 정적(다리 안 움직임) → 같은 발 반복(교대 없음) → 일반 보정 순으로 분기합니다.
func GaitRetryHint(g GaitResult) string {
	switch {
	case g.LegMotion < 0.05:
		return "IMPORTANT CORRECTION: the previous attempt was almost static — the legs barely moved between poses. Redraw a real side-view WALK: each frame must show a clearly different leg position, with the legs visibly opening into a stride and closing again."
	case g.Oscillations < 2 && g.LumaLeadFlips >= 2:
		// 교대는 있으나 두 번째 wide contact가 없음 → stride oscillation 미완성
		return "IMPORTANT CORRECTION: the leg alternation was detected (good!) but the STRIDE only opened ONCE — frame 1 had a wide contact pose but the second wide contact pose (frame " + fmt.Sprintf("%d", g.Frames/2+1) + ") was NOT wide enough. The stride must open-and-close TWICE across the full row (a 'double valley' shape). Fix: draw frame " + fmt.Sprintf("%d", g.Frames/2+1) + " with the feet at THE SAME wide spread as frame 1 — left foot heel-striking FAR FORWARD, right foot toe-pushing FAR BEHIND. The foot gap in frame " + fmt.Sprintf("%d", g.Frames/2+1) + " must equal the foot gap in frame 1."
	case g.Oscillations < 2:
		return "IMPORTANT CORRECTION: the previous attempt only completed ONE stride instead of TWO. The row must have exactly TWO contact poses with a wide foot spread: frame 1 (RIGHT foot forward) AND frame " + fmt.Sprintf("%d", g.Frames/2+1) + " (LEFT foot forward). Both contact frames need the same wide leg spread. The second half (frames " + fmt.Sprintf("%d", g.Frames/2+1) + " to " + fmt.Sprintf("%d", g.Frames) + ") must be a complete mirror-image step: open wide → passing → push-off."
	case g.LeadFlips < 1 && g.LumaLeadFlips < 1 && g.LumaSameSide:
		return "IMPORTANT CORRECTION: the legs opened and closed, but the LIGHTER (near/RIGHT) leg STAYED IN FRONT for BOTH steps — it never moved to the back. This is the exact same-foot failure: the light leg (RIGHT) is always the frontmost leg. The fix: in frames " + fmt.Sprintf("%d", g.Frames/2+1) + " through " + fmt.Sprintf("%d", g.Frames) + " (SECOND STEP), the RIGHT (LIGHTER) leg must be clearly BEHIND the LEFT (DARKER) leg. Concretely: draw the dark LEFT leg with its heel stretched FAR FORWARD in frame " + fmt.Sprintf("%d", g.Frames/2+1) + ", while the light RIGHT leg is BEHIND pushing off. The lighter leg MUST be in the back half of the frame's stride spread."
	case g.LeadFlips < 1 && g.LumaLeadFlips < 1:
		return "IMPORTANT CORRECTION: the legs opened and closed, but the SAME foot stayed in front for both steps — the foot never alternated (same-foot walk). Two fixes required: (1) ALTERNATION — the two contact poses must be left-right MIRRORS of each other: frame 1 has the RIGHT foot forward, frame " + fmt.Sprintf("%d", g.Frames/2+1) + " must have the LEFT foot forward. (2) LEG SHADING — the near (camera-side = character's RIGHT) leg must be drawn LIGHTER than the far (character's LEFT) leg in every frame. RIGHT leg shading: LIGHT throughout. LEFT leg shading: DARK throughout. The RIGHT leg naturally switches position (FRONT in step 1, BACK in step 2) — this switch is the visual proof of correct alternation. Both the position-switch AND the shading are required."
	case g.ScaleConsistency < 0.5:
		return "IMPORTANT CORRECTION: the character changed SIZE between frames — it shrank (the head dropped lower) on the wide-stride poses, as if zoomed out to fit the splayed legs. Keep the character the EXACT same size and the head at the SAME height in every frame. Widen the stride by moving the FEET apart along the ground, never by scaling the whole body down. Only a small natural head bob is allowed."
	default:
		return "IMPORTANT CORRECTION: make the walking cycle read more clearly — open the legs into a wide stride at the contact poses and bring them together at the passing poses, and alternate which foot is in front between the two steps."
	}
}

// WalkGait는 측면 보행 스트립의 보행 품질을 계산합니다.
func WalkGait(frames []*image.NRGBA) GaitResult {
	r := GaitResult{Frames: len(frames)}
	if len(frames) < 2 {
		return r
	}
	gs := make([]frameGait, 0, len(frames))
	for _, f := range frames {
		if g := analyzeFrameGait(f); g.has {
			gs = append(gs, g)
		}
	}
	if len(gs) < 2 {
		return r
	}

	var sumStride, sumBodyW float64
	strides := make([]float64, len(gs))
	for i, g := range gs {
		strides[i] = g.stride
		sumStride += g.stride
		sumBodyW += g.bodyW
	}
	meanStride := sumStride / float64(len(gs))
	meanBodyW := math.Max(sumBodyW/float64(len(gs)), 1)
	minS, maxS := strides[0], strides[0]
	for _, s := range strides {
		minS = math.Min(minS, s)
		maxS = math.Max(maxS, s)
	}
	r.StrideRange = (maxS - minS) / meanBodyW
	r.Oscillations = meanCrossings(strides, meanStride)
	r.LegMotion = legsRegionMotion(frames)
	r.FootGround = footGroundScore(gs, meanBodyW)
	r.LeadFlips = leadFootFlips(gs)
	r.LumaLeadFlips = lumaLeadFlips(gs)
	r.LumaSameSide = lumaLeadSameSide(gs)
	r.ScaleConsistency = scaleConsistencyScore(gs)

	sStride := math.Min(r.StrideRange/0.45, 1.0)
	sMotion := math.Min(r.LegMotion/0.12, 1.0)
	// 교대 증거: 들린 발(lift)의 앞/뒤 교대 + 같은 휘도(정체성) 발의 앞/뒤 교대.
	// 둘 중 무엇이든 1회 이상이면 "같은 발 반복"이 아님을 시사한다.
	altEvidence := r.LeadFlips + r.LumaLeadFlips
	sAlt := math.Min(float64(altEvidence)/2.0, 1.0)
	base := 0.34*sStride + 0.24*sMotion + 0.22*r.FootGround + 0.20*sAlt
	if r.Oscillations < 2 {
		base *= 0.5 // 보행 사이클 자체가 없음(정지/한 스텝)
	}
	// 핵심: 사이클(osc)이 있어도 발 교대 증거가 전혀 없으면 "같은 발" 의심 케이스다.
	// osc는 두 발이 벌어졌다 모이기만 해도 올라가므로 같은 발 보행과 구분되지 않는다.
	// 따라서 교대 증거가 0이면 통과(GaitPassScore)에 못 미치도록 상한을 둔다 →
	// 생성기가 발 교대가 실제로 보이는 시도를 선호하도록 강제한다.
	if altEvidence == 0 {
		base = math.Min(base, GaitPassScore-0.05)
	}
	// 보폭을 넓히려 캐릭터를 통째로 축소(머리가 내려앉음)하는 흔한 회귀를 감점한다.
	// stride 점수가 이런 축소를 오히려 보상하므로, 크기 일관성으로 상쇄한다.
	base *= 0.45 + 0.55*r.ScaleConsistency
	r.Score = base
	return r
}

// scaleConsistencyScore는 프레임 간 캐릭터 크기 일관성을 측정합니다.
// (a) topY 편차: 머리가 내려앉음 — 보폭을 위해 전신을 축소하는 주요 찌그러짐 신호.
// (b) height 편차: 전체 키가 줄어듦 — 또 다른 형태의 축소.
// 두 신호를 가중 결합하여 어느 쪽 찌그러짐도 감지한다.
func scaleConsistencyScore(gs []frameGait) float64 {
	if len(gs) < 2 {
		return 1.0
	}
	var meanTop, meanH float64
	for _, g := range gs {
		meanTop += g.topY
		meanH += g.height
	}
	meanTop /= float64(len(gs))
	meanH /= float64(len(gs))

	var maeTop, maeH float64
	for _, g := range gs {
		maeTop += math.Abs(g.topY - meanTop)
		maeH += math.Abs(g.height - meanH)
	}
	maeTop /= float64(len(gs))
	maeH /= float64(len(gs))

	tolTop := math.Max(meanH*0.10, 2.0) // 키의 10%까지 머리 상하 보빙 허용
	tolH := math.Max(meanH*0.12, 2.0)   // 키의 12%까지 전체 키 변동 허용
	topScore := 1.0 - math.Min(maeTop/tolTop, 1.0)
	heightScore := 1.0 - math.Min(maeH/tolH, 1.0)
	return 0.60*topScore + 0.40*heightScore
}

// meanCrossings는 시퀀스가 평균선을 가로지른 횟수를 셉니다(작은 떨림은 무시).
func meanCrossings(xs []float64, mean float64) int {
	var amp float64
	for _, x := range xs {
		amp = math.Max(amp, math.Abs(x-mean))
	}
	dead := amp * 0.25
	prev, crossings := 0, 0
	for _, x := range xs {
		s := 0
		if x-mean > dead {
			s = 1
		} else if x-mean < -dead {
			s = -1
		}
		if s != 0 {
			if prev != 0 && s != prev {
				crossings++
			}
			prev = s
		}
	}
	return crossings
}

// legsRegionMotion은 하단 50% 영역만으로 인접 프레임 변화를 측정합니다.
func legsRegionMotion(frames []*image.NRGBA) float64 {
	var total float64
	pairs := 0
	for i := 1; i < len(frames); i++ {
		a, b := frames[i-1], frames[i]
		if a.Rect != b.Rect {
			continue
		}
		bnd := a.Bounds()
		startY := bnd.Min.Y + bnd.Dy()/2
		var diffSum float64
		var count int
		for y := startY; y < bnd.Max.Y; y++ {
			for x := bnd.Min.X; x < bnd.Max.X; x++ {
				o := a.PixOffset(x, y)
				aa, ba := a.Pix[o+3], b.Pix[o+3]
				if aa <= alphaThreshold && ba <= alphaThreshold {
					continue
				}
				d := absDiff(a.Pix[o], b.Pix[o]) + absDiff(a.Pix[o+1], b.Pix[o+1]) +
					absDiff(a.Pix[o+2], b.Pix[o+2]) + absDiff(aa, ba)
				diffSum += float64(d) / (255.0 * 4.0)
				count++
			}
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

// footGroundScore는 발 바닥(footBaseY)이 프레임 간 일정할수록 높습니다.
func footGroundScore(gs []frameGait, bodyW float64) float64 {
	var mean float64
	for _, g := range gs {
		mean += g.footBaseY
	}
	mean /= float64(len(gs))
	var mae float64
	for _, g := range gs {
		mae += math.Abs(g.footBaseY - mean)
	}
	mae /= float64(len(gs))
	tol := math.Max(bodyW*0.18, 2.0)
	return 1.0 - math.Min(mae/tol, 1.0)
}

// leadFootFlips는 들린(스윙) 발이 앞↔뒤로 바뀐 횟수를 셉니다.
// 얇은 지면 밴드에서 두 발이 분리된 프레임만 사용하며, 더 높이 들린 발이
// 앞발인지 뒷발인지의 부호(frontFootY-backFootY, 양수면 뒷발이 더 들림)가
// 사이클 동안 교대하면 진짜 보행, 한쪽만 계속 들리면 제자리 행진(=같은 발)입니다.
// 측면 실루엣에서 좌우 식별은 불가하므로 "들린 발의 앞/뒤 교대"를 대용 신호로 씁니다.
func leadFootFlips(gs []frameGait) int {
	var diffs []float64
	var amp float64
	for _, g := range gs {
		if !g.twoFeet {
			continue
		}
		d := g.frontFootY - g.backFootY // >0: 뒷발이 더 들림, <0: 앞발이 더 들림
		diffs = append(diffs, d)
		amp = math.Max(amp, math.Abs(d))
	}
	if len(diffs) < 2 || amp < 1 {
		return 0
	}
	dead := amp * 0.3
	prev, flips := 0, 0
	for _, d := range diffs {
		s := 0
		if d > dead {
			s = 1
		} else if d < -dead {
			s = -1
		}
		if s != 0 {
			if prev != 0 && s != prev {
				flips++
			}
			prev = s
		}
	}
	return flips
}

// lumaLeadSameSide는 밝은 발이 항상 앞 OR 항상 뒤에 있는지를 반환합니다 (같은 발 반복의 강한 신호).
// 2.5 이상의 루마 차이가 나는 프레임이 3개 이상이고, 모두 같은 부호(전부 앞발밝음 or 전부 뒷발밝음)이면 true.
func lumaLeadSameSide(gs []frameGait) bool {
	pos, neg := 0, 0
	for _, g := range gs {
		if !g.twoFeet {
			continue
		}
		d := g.frontFootL - g.backFootL
		if d > 2.5 {
			pos++
		} else if d < -2.5 {
			neg++
		}
	}
	return (pos >= 3 && neg == 0) || (neg >= 3 && pos == 0)
}

// lumaLeadFlips는 "같은 휘도(=같은 다리 정체성) 발이 앞↔뒤로 바뀐 횟수"를 셉니다.
// 측면 실루엣은 좌우 대칭이라 발 정체성을 알 수 없지만, 모델은 가까운(near) 다리와
// 먼(far) 다리를 음영으로 다르게 칠하는 경향이 있어 휘도가 정체성 단서가 됩니다.
// 올바른 보행에서는 같은 다리가 한 스텝은 앞, 다음 스텝은 뒤로 가므로 앞발-뒷발의
// 휘도 차 부호(frontL-backL)가 사이클 동안 교대합니다. 부호가 끝까지 안 바뀌면
// (예: 어두운 다리가 늘 뒤) 같은 발만 끄는 보행을 의심합니다. lift(leadFootFlips)가
// 평발이라 못 잡는 교대를 휘도로 보완하는 신호입니다.
func lumaLeadFlips(gs []frameGait) int {
	var diffs []float64
	var amp float64
	for _, g := range gs {
		if !g.twoFeet {
			continue
		}
		d := g.frontFootL - g.backFootL // >0: 앞발이 더 밝음, <0: 뒷발이 더 밝음
		diffs = append(diffs, d)
		amp = math.Max(amp, math.Abs(d))
	}
	// 휘도 차가 미미하면(amp가 작으면) 정체성 단서가 약하므로 신호로 쓰지 않는다.
	// 임계값 2.5: 어두운 픽셀아트(L<20)에서도 다리 음영을 포착한다.
	// 주의: float64 누산으로 실제 luma 3이 2.9999...로 계산되므로 임계값은 5→3이 아닌 2.5여야 한다.
	if len(diffs) < 2 || amp < 2.5 {
		return 0
	}
	dead := amp * 0.30
	prev, flips := 0, 0
	for _, d := range diffs {
		s := 0
		if d > dead {
			s = 1
		} else if d < -dead {
			s = -1
		}
		if s != 0 {
			if prev != 0 && s != prev {
				flips++
			}
			prev = s
		}
	}
	return flips
}
