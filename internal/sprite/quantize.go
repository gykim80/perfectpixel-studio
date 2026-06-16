package sprite

import (
	"image"
	"sort"
)

// rgb는 양자화용 불투명 색상입니다.
type rgb struct{ r, g, b uint8 }

// collectOpaque는 프레임들에서 불투명 픽셀 색상을 수집합니다 (다운샘플링 포함).
func collectOpaque(frames []*image.NRGBA, maxSamples int) []rgb {
	total := 0
	for _, f := range frames {
		total += len(f.Pix) / 4
	}
	step := 1
	if maxSamples > 0 && total > maxSamples {
		step = total/maxSamples + 1
	}
	var out []rgb
	idx := 0
	for _, f := range frames {
		for i := 0; i+3 < len(f.Pix); i += 4 {
			if f.Pix[i+3] <= alphaThreshold {
				continue
			}
			if idx%step == 0 {
				out = append(out, rgb{f.Pix[i], f.Pix[i+1], f.Pix[i+2]})
			}
			idx++
		}
	}
	return out
}

// BuildSharedPalette는 여러 프레임 전체에서 공유 팔레트를 추출합니다 (median-cut).
// 애니메이션 전 프레임에 같은 팔레트를 강제하면 프레임 간 색 일관성이 크게 향상됩니다.
func BuildSharedPalette(frames []*image.NRGBA, maxColors int) []rgb {
	if maxColors < 2 {
		maxColors = 2
	}
	samples := collectOpaque(frames, 1<<16)
	if len(samples) == 0 {
		return nil
	}
	buckets := [][]rgb{samples}
	for len(buckets) < maxColors {
		// 가장 분산 범위가 큰 버킷 선택
		bestIdx, bestRange := -1, 0
		bestCh := 0
		for bi, b := range buckets {
			if len(b) < 2 {
				continue
			}
			minC := [3]int{255, 255, 255}
			maxC := [3]int{0, 0, 0}
			for _, c := range b {
				ch := [3]int{int(c.r), int(c.g), int(c.b)}
				for k := 0; k < 3; k++ {
					if ch[k] < minC[k] {
						minC[k] = ch[k]
					}
					if ch[k] > maxC[k] {
						maxC[k] = ch[k]
					}
				}
			}
			for k := 0; k < 3; k++ {
				if r := maxC[k] - minC[k]; r > bestRange {
					bestIdx, bestRange, bestCh = bi, r, k
				}
			}
		}
		if bestIdx < 0 || bestRange == 0 {
			break // 더 나눌 수 없음
		}
		b := buckets[bestIdx]
		sort.Slice(b, func(i, j int) bool {
			switch bestCh {
			case 0:
				return b[i].r < b[j].r
			case 1:
				return b[i].g < b[j].g
			default:
				return b[i].b < b[j].b
			}
		})
		mid := len(b) / 2
		buckets[bestIdx] = b[:mid]
		buckets = append(buckets, b[mid:])
	}

	type entry struct {
		c rgb
		n int
	}
	entries := make([]entry, 0, len(buckets))
	for _, b := range buckets {
		if len(b) == 0 {
			continue
		}
		var sr, sg, sb int
		for _, c := range b {
			sr += int(c.r)
			sg += int(c.g)
			sb += int(c.b)
		}
		n := len(b)
		entries = append(entries, entry{rgb{uint8(sr / n), uint8(sg / n), uint8(sb / n)}, n})
	}

	// 근접 색 병합: 프레임 간 미세 색 drift(채널당 ~8 이내)를 하나의 색으로 수렴
	const mergeThresh = 600 // colorDist2 기준 ≈ 채널당 8
	sort.Slice(entries, func(i, j int) bool { return entries[i].n > entries[j].n })
	merged := entries[:0]
	for _, e := range entries {
		absorbed := false
		for mi := range merged {
			if colorDist2(e.c, merged[mi].c) < mergeThresh {
				// 가중 평균으로 흡수
				tot := merged[mi].n + e.n
				merged[mi].c = rgb{
					uint8((int(merged[mi].c.r)*merged[mi].n + int(e.c.r)*e.n) / tot),
					uint8((int(merged[mi].c.g)*merged[mi].n + int(e.c.g)*e.n) / tot),
					uint8((int(merged[mi].c.b)*merged[mi].n + int(e.c.b)*e.n) / tot),
				}
				merged[mi].n = tot
				absorbed = true
				break
			}
		}
		if !absorbed {
			merged = append(merged, e)
		}
	}
	// 빈 버킷/팔레트 부족으로 색이 2개 미만이면 0번과 255번 회색이라도 추가
	if len(merged) < 2 {
		if len(merged) == 0 {
			merged = append(merged, entry{rgb{0, 0, 0}, 1}, entry{rgb{255, 255, 255}, 1})
		} else {
			merged = append(merged, entry{rgb{255, 255, 255}, 1})
		}
	}
	palette := make([]rgb, len(merged))
	for i, e := range merged {
		palette[i] = e.c
	}
	return palette
}

func colorDist2(a, b rgb) int {
	dr, dg, db := int(a.r)-int(b.r), int(a.g)-int(b.g), int(a.b)-int(b.b)
	// 인지 가중치 (녹색 민감도 높음)
	return 2*dr*dr + 4*dg*dg + 3*db*db
}

func nearestColor(c rgb, palette []rgb, cache map[rgb]rgb) rgb {
	if hit, ok := cache[c]; ok {
		return hit
	}
	best, bestD := palette[0], 1<<62
	for _, p := range palette {
		if d := colorDist2(c, p); d < bestD {
			best, bestD = p, d
		}
	}
	cache[c] = best
	return best
}

// ApplyPalette는 이미지의 모든 불투명 픽셀을 팔레트 최근접 색으로 치환하고
// 알파를 0/255로 이진화합니다 (픽셀아트는 부분 투명을 쓰지 않음).
func ApplyPalette(img *image.NRGBA, palette []rgb) {
	if len(palette) == 0 {
		return
	}
	cache := make(map[rgb]rgb, 512)
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if img.Pix[i+3] < 128 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 0, 0, 0, 0
			continue
		}
		img.Pix[i+3] = 255
		c := nearestColor(rgb{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}, palette, cache)
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = c.r, c.g, c.b
	}
}
