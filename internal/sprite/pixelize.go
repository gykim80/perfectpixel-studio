package sprite

import (
	"image"
	"sort"
)

// DetectPixelScale은 AI가 생성한 "가짜 픽셀아트"의 실제 픽셀 블록 크기를 추정합니다.
// 수평/수직 동일-색 run 길이의 최빈값을 사용합니다 (unfake.js 기법).
// 감지 실패 또는 이미 네이티브 해상도면 1을 반환합니다.
func DetectPixelScale(img *image.NRGBA) int {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w < 32 || h < 32 {
		return 1
	}
	maxScale := min(w, h) / 8
	if maxScale > 32 {
		maxScale = 32
	}
	if maxScale < 2 {
		return 1
	}
	hist := make([]int, maxScale+1)

	countRuns := func(get func(a, b int) (uint8, uint8, uint8, uint8), outer, inner int) {
		for o := 0; o < outer; o++ {
			runLen := 1
			pr, pg, pb, pa := get(o, 0)
			for i := 1; i < inner; i++ {
				r, g, b, al := get(o, i)
				same := (al <= alphaThreshold && pa <= alphaThreshold) ||
					(al > alphaThreshold && pa > alphaThreshold && nearRGB(r, g, b, pr, pg, pb))
				if same {
					runLen++
				} else {
					if runLen >= 2 && runLen <= maxScale {
						hist[runLen]++
					}
					runLen = 1
				}
				pr, pg, pb, pa = r, g, b, al
			}
		}
	}
	countRuns(func(y, x int) (uint8, uint8, uint8, uint8) {
		i := img.PixOffset(x, y)
		return img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
	}, h, w)
	countRuns(func(x, y int) (uint8, uint8, uint8, uint8) {
		i := img.PixOffset(x, y)
		return img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
	}, w, h)

	best, bestCount := 1, 0
	for s := 2; s <= maxScale; s++ {
		// 짧은 run이 항상 많으므로 run 길이로 가중
		weighted := hist[s] * s
		if weighted > bestCount {
			best, bestCount = s, weighted
		}
	}
	if best < 2 {
		return 1
	}
	return best
}

// nearRGB는 약간의 AA 노이즈를 허용하는 색 비교입니다.
func nearRGB(r1, g1, b1, r2, g2, b2 uint8) bool {
	const tol = 12
	return absInt(int(r1)-int(r2)) <= tol && absInt(int(g1)-int(g2)) <= tol && absInt(int(b1)-int(b2)) <= tol
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Pixelize는 이미지를 scale×scale 블록 단위로 스냅해 진짜 픽셀아트 그리드로 만듭니다.
// 각 블록의 dominant color를 채택하고 출력 크기는 입력과 동일하게 유지합니다.
func Pixelize(img *image.NRGBA, scale int) *image.NRGBA {
	if scale < 2 {
		return img
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	type counted struct {
		c rgb
		n int
	}
	for by := 0; by < h; by += scale {
		for bx := 0; bx < w; bx += scale {
			bw, bh := min(scale, w-bx), min(scale, h-by)
			// 블록 내 dominant color 결정 (알파 과반 → 색 최빈)
			opaque := 0
			counts := make(map[rgb]int, 8)
			for dy := 0; dy < bh; dy++ {
				for dx := 0; dx < bw; dx++ {
					i := img.PixOffset(bx+dx, by+dy)
					if img.Pix[i+3] <= alphaThreshold {
						continue
					}
					opaque++
					counts[rgb{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}]++
				}
			}
			if opaque*2 < bw*bh {
				continue // 블록 과반이 투명 → 빈 블록
			}
			var dom counted
			for c, n := range counts {
				if n > dom.n {
					dom = counted{c, n}
				}
			}
			for dy := 0; dy < bh; dy++ {
				for dx := 0; dx < bw; dx++ {
					i := out.PixOffset(bx+dx, by+dy)
					out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = dom.c.r, dom.c.g, dom.c.b, 255
				}
			}
		}
	}
	return out
}

// PaletteSizeForStyle은 스타일별 권장 팔레트 색 수를 반환합니다 (0이면 후처리 비활성).
func PaletteSizeForStyle(styleKey string) int {
	switch styleKey {
	case "retro16":
		return 16
	case "pixel":
		return 32
	default:
		return 0 // chibi/cartoon/custom은 픽셀 그리드 강제하지 않음
	}
}

// PixelPostProcess는 한 상태의 프레임 묶음에 공유 팔레트 양자화 + 픽셀 그리드 스냅을 적용합니다.
// 프레임들은 in-place로 수정되거나 교체됩니다.
func PixelPostProcess(frames []*image.NRGBA, paletteSize int) {
	if paletteSize <= 0 || len(frames) == 0 {
		return
	}
	palette := BuildSharedPalette(frames, paletteSize)
	if palette == nil {
		return
	}
	for _, f := range frames {
		ApplyPalette(f, palette)
	}

	// 1차: fast-pixelizer 기반 edge-profile + autocorrelation 그리드 검출.
	// 축별 독립 셀 수를 추정하므로 비정수 스케일·위상 어긋남에 견고하다.
	cols := make([]int, 0, len(frames))
	rows := make([]int, 0, len(frames))
	for _, f := range frames {
		if c, r, ok := detectGridCounts(f); ok {
			cols = append(cols, c)
			rows = append(rows, r)
		}
	}
	if len(cols) > len(frames)/2 {
		// 프레임 간 그리드 일관성을 위해 중앙값 셀 수 공유
		mc, mr := medianInt(cols), medianInt(rows)
		for i, f := range frames {
			frames[i] = SnapToGrid(f, mc, mr)
		}
		return
	}

	// 폴백: 그리드 검출 불확실 → 기존 modal run-length 정수 스케일 스냅
	scales := make([]int, 0, len(frames))
	for _, f := range frames {
		scales = append(scales, DetectPixelScale(f))
	}
	sorted := append([]int(nil), scales...)
	sort.Ints(sorted)
	scale := sorted[len(sorted)/2]
	if scale < 2 {
		return
	}
	for i, f := range frames {
		frames[i] = Pixelize(f, scale)
	}
}
