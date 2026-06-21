package sprite

import (
	"image"
	"math"
	"sort"
)

// snap.go는 fast-pixelizer(handsupmin/fast-pixelizer)의 snap 알고리즘을 Go로 포팅한 것입니다.
// 기존 DetectPixelScale(단일 정수 정사각 스케일, modal run-length)의 약점인
// 비정수 스케일·축별 그리드 차이·그리드 위상 어긋남을 edge-profile + autocorrelation으로 보강합니다.

const (
	// snapMinConfidence는 autocorrelation 주기를 신뢰할 최소 정규화 상관계수입니다.
	snapMinConfidence = 0.20
	// snapMaxShortAxisCells는 짧은 축에서 허용하는 최대 셀 수(텍스처 노이즈 스냅 방지)입니다.
	snapMaxShortAxisCells = 256
)

// grayLuma는 NRGBA 픽셀의 휘도를 반환합니다. 투명 픽셀은 0으로 취급합니다(fast-pixelizer grayAt).
func grayLuma(pix []uint8, off int) float64 {
	if pix[off+3] <= alphaThreshold {
		return 0
	}
	return 0.299*float64(pix[off]) + 0.587*float64(pix[off+1]) + 0.114*float64(pix[off+2])
}

// colEdgeProfile은 각 x열의 수평 색경계 강도(좌우 휘도차의 누적)를 계산합니다.
func colEdgeProfile(img *image.NRGBA) []float64 {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	p := make([]float64, w)
	for y := 0; y < h; y++ {
		for x := 1; x < w-1; x++ {
			left := img.PixOffset(x-1, y)
			right := img.PixOffset(x+1, y)
			p[x] += math.Abs(grayLuma(img.Pix, right) - grayLuma(img.Pix, left))
		}
	}
	return p
}

// rowEdgeProfile은 각 y행의 수직 색경계 강도를 계산합니다.
func rowEdgeProfile(img *image.NRGBA) []float64 {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	p := make([]float64, h)
	for x := 0; x < w; x++ {
		for y := 1; y < h-1; y++ {
			top := img.PixOffset(x, y-1)
			bottom := img.PixOffset(x, y+1)
			p[y] += math.Abs(grayLuma(img.Pix, bottom) - grayLuma(img.Pix, top))
		}
	}
	return p
}

// snapSmoothProfile은 fast-pixelizer와 동일한 5-tap 가중 평활을 적용합니다.
func snapSmoothProfile(profile []float64) []float64 {
	out := make([]float64, len(profile))
	for i := range profile {
		sum := profile[i] * 2
		weight := 2.0
		if i > 0 {
			sum += profile[i-1]
			weight++
		}
		if i+1 < len(profile) {
			sum += profile[i+1]
			weight++
		}
		if i > 1 {
			sum += profile[i-2] * 0.5
			weight += 0.5
		}
		if i+2 < len(profile) {
			sum += profile[i+2] * 0.5
			weight += 0.5
		}
		out[i] = sum / weight
	}
	return out
}

// estimatePeriodicStep은 정규화 autocorrelation으로 그리드 셀 주기(픽셀)를 추정합니다.
// step(픽셀 주기), confidence(정규화 상관계수), ok를 반환합니다.
func estimatePeriodicStep(profile []float64) (int, float64, bool) {
	n := len(profile)
	maxLag := min(256, n/3)
	startLag := max(2, min(maxLag, 2))
	if maxLag < startLag {
		return 0, 0, false
	}

	smoothed := snapSmoothProfile(profile)
	mean := 0.0
	for _, v := range smoothed {
		mean += v
	}
	mean /= float64(len(smoothed))

	centered := make([]float64, len(smoothed))
	energy := 0.0
	for i, v := range smoothed {
		c := v - mean
		centered[i] = c
		energy += c * c
	}
	if energy == 0 {
		return 0, 0, false
	}

	corrs := make([]float64, maxLag+1)
	bestCorr := math.Inf(-1)
	for lag := startLag; lag <= maxLag; lag++ {
		var num, denomA, denomB float64
		for i := lag; i < len(centered); i++ {
			a := centered[i]
			b := centered[i-lag]
			num += a * b
			denomA += a * a
			denomB += b * b
		}
		corr := math.Inf(-1)
		if denomA > 0 && denomB > 0 {
			corr = num / math.Sqrt(denomA*denomB)
		}
		corrs[lag] = corr
		if corr > bestCorr {
			bestCorr = corr
		}
	}

	if math.IsInf(bestCorr, 0) || bestCorr <= 0 {
		return 0, 0, false
	}

	// 깨끗한 주기 신호에서는 기본 주기의 모든 배수가 corr≈최대값이 된다.
	// argmax는 큰 배수(또는 시트의 프레임 주기)로 튈 수 있으므로,
	// near-best 중 가장 작은 lag(기본 주기)를 선택한다.
	fundamental := startLag
	for lag := startLag; lag <= maxLag; lag++ {
		if corrs[lag] >= bestCorr*0.92 {
			fundamental = lag
			break
		}
	}
	return fundamental, corrs[fundamental], true
}

// detectGridCounts는 이미지의 픽셀 그리드를 축별 독립으로 검출해 (cols, rows) 셀 수를 반환합니다.
func detectGridCounts(img *image.NRGBA) (cols, rows int, ok bool) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w < 16 || h < 16 {
		return 0, 0, false
	}

	// 양자화로 색 노이즈/블러를 제거하면 edge-profile이 그리드 라인에만 스파이크가
	// 남는 형태가 되어, 작은 lag의 가짜 autocorrelation(블러 평탄화)에 갇히지 않는다.
	det := img
	if q := quantizeForDetect(img); q != nil {
		det = q
	}

	// flatness 가드: 인접 픽셀이 동색인 비율이 낮으면 셀이 1px(네이티브)이라는 뜻 →
	// autocorrelation이 색 주기를 그리드로 오인하지 않도록 스냅을 포기한다.
	if flatPixelFraction(det) < 0.5 {
		return 0, 0, false
	}

	colStep, colConf, colOK := estimatePeriodicStep(colEdgeProfile(det))
	rowStep, rowConf, rowOK := estimatePeriodicStep(rowEdgeProfile(det))
	if !colOK || !rowOK {
		return 0, 0, false
	}
	if colConf < snapMinConfidence || rowConf < snapMinConfidence {
		return 0, 0, false
	}

	minStep := max(2, int(math.Ceil(float64(min(w, h))/snapMaxShortAxisCells)))
	if colStep < minStep || rowStep < minStep {
		return 0, 0, false
	}

	cols = int(math.Round(float64(w) / float64(colStep)))
	rows = int(math.Round(float64(h) / float64(rowStep)))
	if cols < 2 || rows < 2 {
		return 0, 0, false
	}
	return arbitrateSquare(w, h, cols, rows), squareRows(w, h, cols, rows), true
}

// arbitrateSquare는 정사각 캔버스에서 X/Y 셀 수가 어긋나면 작은 쪽으로 통일한 cols를 반환합니다.
func arbitrateSquare(w, h, cols, rows int) int {
	if math.Abs(float64(w-h))/float64(max(w, h)) <= 0.05 && cols != rows {
		return min(cols, rows)
	}
	return cols
}

func squareRows(w, h, cols, rows int) int {
	if math.Abs(float64(w-h))/float64(max(w, h)) <= 0.05 && cols != rows {
		return min(cols, rows)
	}
	return rows
}

// SnapAuto는 edge-profile + autocorrelation으로 그리드를 자동 검출해 스냅합니다.
// 검출 실패 시 원본을 그대로 반환합니다. 반환값: 결과, 검출 cols, rows, 성공 여부.
func SnapAuto(img *image.NRGBA) (*image.NRGBA, int, int, bool) {
	cols, rows, ok := detectGridCounts(img)
	if !ok {
		return img, 0, 0, false
	}
	return SnapToGrid(img, cols, rows), cols, rows, true
}

// SnapToGrid는 이미지를 cols×rows 셀로 리샘플(셀별 dominant color, 알파 인지)한 뒤
// 원본 해상도로 균일 재렌더링합니다. 출력 크기는 입력과 동일합니다.
func SnapToGrid(img *image.NRGBA, cols, rows int) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if cols < 1 || rows < 1 || w == 0 || h == 0 {
		return img
	}

	type cell struct {
		c      rgb
		opaque bool
	}
	cells := make([]cell, cols*rows)
	for ry := 0; ry < rows; ry++ {
		y0, y1 := ry*h/rows, (ry+1)*h/rows
		for cx := 0; cx < cols; cx++ {
			x0, x1 := cx*w/cols, (cx+1)*w/cols
			counts := make(map[rgb]int, 8)
			opaque, total := 0, 0
			for py := y0; py < y1; py++ {
				for px := x0; px < x1; px++ {
					i := img.PixOffset(px, py)
					total++
					if img.Pix[i+3] <= alphaThreshold {
						continue
					}
					opaque++
					counts[rgb{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}]++
				}
			}
			if total == 0 || opaque*2 < total {
				continue // 셀 과반이 투명 → 빈 셀
			}
			var dom rgb
			best := 0
			for c, n := range counts {
				if n > best {
					best, dom = n, c
				}
			}
			cells[ry*cols+cx] = cell{dom, true}
		}
	}

	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		ry := y * rows / h
		for x := 0; x < w; x++ {
			cx := x * cols / w
			cl := cells[ry*cols+cx]
			if !cl.opaque {
				continue
			}
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = cl.c.r, cl.c.g, cl.c.b, 255
		}
	}
	return out
}

// quantizeForDetect는 검출 전용으로 32색 공유 팔레트를 적용한 사본을 만듭니다.
// 원본은 건드리지 않으며, 팔레트 생성 실패 시 nil을 반환합니다.
func quantizeForDetect(img *image.NRGBA) *image.NRGBA {
	b := img.Bounds()
	c := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			di := c.PixOffset(x, y)
			copy(c.Pix[di:di+4], img.Pix[si:si+4])
		}
	}
	pal := BuildSharedPalette([]*image.NRGBA{c}, 32)
	if pal == nil {
		return nil
	}
	ApplyPalette(c, pal)
	return c
}

// flatPixelFraction은 인접한(우측·하단) 픽셀이 근사 동색(nearRGB tolerance)인 비율을 반환합니다.
// 진짜 셀(>=2px)이나 블러로 확대된 가짜 픽셀아트는 셀 내부가 평탄해 높고,
// 1px 네이티브/노이즈는 인접 색차가 커서 낮습니다.
func flatPixelFraction(img *image.NRGBA) float64 {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	same, total := 0, 0
	eq := func(a, b int) bool {
		ta := img.Pix[a+3] <= alphaThreshold
		tb := img.Pix[b+3] <= alphaThreshold
		if ta || tb {
			return ta && tb // 둘 다 투명이면 평탄, 한쪽만 투명이면 경계
		}
		return nearRGB(img.Pix[a], img.Pix[a+1], img.Pix[a+2], img.Pix[b], img.Pix[b+1], img.Pix[b+2])
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := img.PixOffset(x, y)
			if x+1 < w {
				total++
				if eq(i, img.PixOffset(x+1, y)) {
					same++
				}
			}
			if y+1 < h {
				total++
				if eq(i, img.PixOffset(x, y+1)) {
					same++
				}
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(same) / float64(total)
}

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	return sorted[len(sorted)/2]
}
