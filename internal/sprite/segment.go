package sprite

import "image"

// 스트립 → 프레임 분할: 연결요소(flood-fill)가 아니라 수직 알파 투영 프로파일을
// 사용합니다. 컬럼별 알파 질량의 골(gutter)로 자연 포즈 수를 세고, 포즈가 닿아
// 골이 없을 때는 동적계획법(DP)으로 "내용을 가장 적게 가르는" expected-1개의 컷을
// 강제로 찾습니다. OCR 라인/단어 분리에서 쓰는 projection-profile + optimal-cut 기법.

// colSpan은 스트립 좌표계의 컬럼 구간 [start, end)입니다.
type colSpan struct{ start, end int }

// projectAlpha는 컬럼별 알파 질량 P[x] = Σ_y α(x,y) 를 계산합니다.
func projectAlpha(img *image.NRGBA) []float64 {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	p := make([]float64, w)
	for x := 0; x < w; x++ {
		var sum float64
		for y := 0; y < h; y++ {
			sum += float64(img.Pix[img.PixOffset(x, y)+3])
		}
		p[x] = sum
	}
	return p
}

// smoothProfile은 박스 이동평균으로 프로파일을 평활합니다 (압축 잡음/얇은 틈 억제).
func smoothProfile(p []float64, win int) []float64 {
	if win < 1 || len(p) == 0 {
		return p
	}
	out := make([]float64, len(p))
	half := win / 2
	for i := range p {
		var sum float64
		var n int
		for j := i - half; j <= i+half; j++ {
			if j >= 0 && j < len(p) {
				sum += p[j]
				n++
			}
		}
		out[i] = sum / float64(n)
	}
	return out
}

func maxOf(p []float64) float64 {
	m := 0.0
	for _, v := range p {
		if v > m {
			m = v
		}
	}
	return m
}

// contentRuns는 P가 eps를 넘는 연속 구간(포즈)을 찾습니다.
// minW보다 좁거나 봉우리가 peakMin 미만인 구간은 잡티로 보고 버립니다.
func contentRuns(p []float64, eps, peakMin float64, minW int) []colSpan {
	var runs []colSpan
	i := 0
	n := len(p)
	for i < n {
		if p[i] <= eps {
			i++
			continue
		}
		j := i
		peak := 0.0
		for j < n && p[j] > eps {
			if p[j] > peak {
				peak = p[j]
			}
			j++
		}
		if j-i >= minW && peak >= peakMin {
			runs = append(runs, colSpan{i, j})
		}
		i = j
	}
	return runs
}

// runMass는 구간 내 알파 질량 합입니다.
func runMass(p []float64, s colSpan) float64 {
	var m float64
	for x := s.start; x < s.end && x < len(p); x++ {
		m += p[x]
	}
	return m
}

// dropMinorRuns는 최대 런 질량의 frac 미만인 런(원거리 잔여물/잡티)을 제거합니다.
// 옛 연결요소 방식의 "최대 blob 면적 대비 시드 임계값" 가드와 같은 취지입니다.
func dropMinorRuns(p []float64, runs []colSpan, frac float64) []colSpan {
	if len(runs) <= 1 {
		return runs
	}
	maxM := 0.0
	for _, r := range runs {
		if m := runMass(p, r); m > maxM {
			maxM = m
		}
	}
	thr := maxM * frac
	var out []colSpan
	for _, r := range runs {
		if runMass(p, r) >= thr {
			out = append(out, r)
		}
	}
	return out
}

// dpNCut은 [x0,x1) 구간을 정확히 n개 세그먼트로 나누는 n-1개 컷 컬럼을 찾습니다.
// 비용 = Σ P[cut] (질량이 적은 곳을 자르는 게 저렴) + 폭 정규화(이상폭에서 벗어날수록 벌점).
// 닿아 있는 포즈를 강제로 expected개로 분리할 때 사용합니다.
func dpNCut(p []float64, x0, x1, n int) []int {
	if n <= 1 || x1-x0 < n {
		return nil
	}
	width := x1 - x0
	ideal := float64(width) / float64(n)
	minW := int(ideal * 0.45)
	if minW < 2 {
		minW = 2
	}
	const lambda = 0.0015 // 폭 정규화 가중 (질량 비용 대비)

	cuts := n - 1
	type cell struct {
		cost float64
		prev int
	}
	dp := make([][]cell, cuts+1)
	for k := range dp {
		dp[k] = make([]cell, x1+1)
		for x := range dp[k] {
			dp[k][x].cost = 1e18
			dp[k][x].prev = -1
		}
	}
	dp[0][x0].cost = 0 // 가상 시작 경계
	for k := 1; k <= cuts; k++ {
		lo := x0 + (k-1)*minW
		for x := x0 + k*minW; x <= x1-(cuts-k+1)*minW; x++ {
			best := 1e18
			bestPrev := -1
			for xp := lo; xp <= x-minW; xp++ {
				if dp[k-1][xp].cost >= 1e17 {
					continue
				}
				d := float64(x-xp) - ideal
				c := dp[k-1][xp].cost + p[x] + lambda*d*d
				if c < best {
					best, bestPrev = c, xp
				}
			}
			dp[k][x] = cell{cost: best, prev: bestPrev}
		}
	}
	bestEnd, bestCost := -1, 1e18
	for x := x0 + cuts*minW; x <= x1-minW; x++ {
		d := float64(x1-x) - ideal
		c := dp[cuts][x].cost + lambda*d*d
		if c < bestCost {
			bestCost, bestEnd = c, x
		}
	}
	if bestEnd < 0 {
		return nil
	}
	out := make([]int, cuts)
	x := bestEnd
	for k := cuts; k >= 1; k-- {
		out[k-1] = x
		x = dp[k][x].prev
		if x < 0 {
			return nil
		}
	}
	return out
}

// segmentStrip은 스트립을 expected개 컬럼 세그먼트로 나누고 감지된 자연 포즈 수를
// 함께 반환합니다. 자연 포즈 수가 expected와 같으면 골(gutter) 중심에서 깔끔히 자르고,
// 아니면 DP로 expected개를 강제 분할합니다.
func segmentStrip(img *image.NRGBA, expected int) (segs []colSpan, natural int) {
	w := img.Rect.Dx()
	if w == 0 || expected < 1 {
		return nil, 0
	}
	raw := projectAlpha(img)
	win := w / 220
	if win < 3 {
		win = 3
	}
	p := smoothProfile(raw, win)
	mx := maxOf(p)
	if mx <= 0 {
		return nil, 0
	}
	eps := 0.045 * mx
	peakMin := 0.18 * mx
	minRun := w / 100
	if minRun < 4 {
		minRun = 4
	}
	runs := contentRuns(p, eps, peakMin, minRun)
	runs = dropMinorRuns(p, runs, 0.20)
	if len(runs) == 0 {
		return nil, 0
	}

	// 런마다 "토르소 봉우리(prominence peak)" 수로 포즈 수를 추정하되, 런 폭으로
	// 상한을 둔다. 봉우리는 "어디서 자를지", 폭은 "몇 개로 자를지"를 정한다:
	// 발차기처럼 한 포즈가 토르소+뻗은 다리로 두 봉우리를 만들어도, 런 폭이 단일
	// 포즈 폭(중앙값)이면 1개로 묶어 과분할을 막는다. 닿아 넓어진 런만 그만큼 쪼갠다.
	med := medianRunWidth(runs)
	widthTotal := 0.0
	for _, r := range runs {
		widthTotal += float64(r.end - r.start)
	}
	for _, r := range runs {
		nPeaks := len(posePeaks(p, r.start, r.end))
		if len(runs) > 1 && med > 0 {
			maxByWidth := int(float64(r.end-r.start)/med + 0.5)
			if maxByWidth < 1 {
				maxByWidth = 1
			}
			if nPeaks > maxByWidth {
				nPeaks = maxByWidth
			}
		}
		// 포즈 사이 간격이 거의 없어(overlapping) 봉우리가 1개뿐이지만
		// 런 폭이 평균 포즈 폭의 1.5배 이상이면 강제로 2개로 의심한다.
		if nPeaks == 1 && len(runs) > 1 && med > 0 {
			if float64(r.end-r.start) > med*1.45 {
				nPeaks = 2
			}
		}
		if nPeaks <= 1 {
			segs = append(segs, r)
		} else {
			segs = append(segs, splitRange(p, r.start, r.end, nPeaks)...)
		}
	}

	// 강제 복구: 감지된 수가 기대와 다르고, 전체 콘텐츠 폭이 기대 개수의
	// 최소 폭을 감당할 수 있다면 전체 strip을 expected개로 균등/DP 분할.
	// AI가 포즈를 마젠타 gutter 없이 완전히 붙여 그리는 경우를 방어한다.
	if len(segs) != expected && widthTotal/float64(expected) >= 16 && w/expected >= 16 {
		segs = splitRange(p, 0, w, expected)
	}

	// 방출 프레임 수 = 추정 포즈 수 (DP로 expected를 강제하지 않고 정직하게 보고).
	return segs, len(segs)
}

// medianRunWidth는 런 폭의 중앙값입니다 (전형적 단일 포즈 폭 추정).
func medianRunWidth(runs []colSpan) float64 {
	if len(runs) == 0 {
		return 0
	}
	ws := make([]int, len(runs))
	for i, r := range runs {
		ws[i] = r.end - r.start
	}
	for i := 1; i < len(ws); i++ {
		for j := i; j > 0 && ws[j-1] > ws[j]; j-- {
			ws[j-1], ws[j] = ws[j], ws[j-1]
		}
	}
	return float64(ws[len(ws)/2])
}

// posePeaks는 [s,e) 구간에서 prominence(돌출도) 기준의 강한 봉우리(=포즈) 컬럼을
// 찾습니다. 봉우리 후보는 런 최대값의 45% 이상인 국소 최대이고, 더 높은 봉우리와의
// 사이 골이 충분히 깊어야(자기 높이의 62% 미만으로 내려가야) 별개 포즈로 인정됩니다.
func posePeaks(p []float64, s, e int) []int {
	if e-s < 3 {
		return []int{(s + e) / 2}
	}
	runMax := 0.0
	for x := s; x < e; x++ {
		if p[x] > runMax {
			runMax = p[x]
		}
	}
	if runMax <= 0 {
		return []int{(s + e) / 2}
	}
	cand := []int{}
	for x := s + 1; x < e-1; x++ {
		if p[x] >= p[x-1] && p[x] > p[x+1] && p[x] >= 0.45*runMax {
			cand = append(cand, x)
		}
	}
	if len(cand) == 0 {
		return []int{(s + e) / 2}
	}
	keep := []int{}
	for _, m := range cand {
		prominent := true
		for _, k := range cand {
			if k == m || p[k] < p[m] {
				continue // 자기보다 높은 봉우리에 대해서만 골 깊이 검사
			}
			lo, hi := m, k
			if lo > hi {
				lo, hi = hi, lo
			}
			vmin := p[lo]
			for x := lo; x <= hi; x++ {
				if p[x] < vmin {
					vmin = p[x]
				}
			}
			if vmin > 0.62*p[m] { // 사이 골이 얕다 → 같은 포즈의 일부
				prominent = false
				break
			}
		}
		if prominent {
			keep = append(keep, m)
		}
	}
	if len(keep) == 0 {
		return []int{cand[0]}
	}
	return keep
}

// splitRange는 [s,e) 구간을 DP 최소 절단으로 n개 세그먼트로 나눕니다(실패 시 균등).
func splitRange(p []float64, s, e, n int) []colSpan {
	if n <= 1 || e-s < n {
		return []colSpan{{s, e}}
	}
	var out []colSpan
	if cuts := dpNCut(p, s, e, n); len(cuts) == n-1 {
		prev := s
		for _, c := range cuts {
			out = append(out, colSpan{prev, c})
			prev = c
		}
		out = append(out, colSpan{prev, e})
		return out
	}
	for i := 0; i < n; i++ {
		out = append(out, colSpan{s + (e-s)*i/n, s + (e-s)*(i+1)/n})
	}
	return out
}
