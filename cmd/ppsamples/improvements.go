package main

// 직전 세션 개선(extract.go bodyExtent 스케일, segment.go overlap 복구)의
// 적용 전/후를 합성 스트립으로 시연한다. 실제 AI 호출 없이 sprite 파이프라인을
// 그대로 호출하고, "전(WITHOUT)"은 구 동작을 충실히 재현해 나란히 보여준다.

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"sort"

	"perfectpixel/internal/sprite"

	xdraw "golang.org/x/image/draw"
)

// contentColRuns는 빈 열(gap 이상 연속)로 구분되는 콘텐츠 열 구간들을 반환한다.
func contentColRuns(strip *image.NRGBA, gap int) [][2]int {
	w, h := strip.Rect.Dx(), strip.Rect.Dy()
	colFull := make([]bool, w)
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			if strip.Pix[strip.PixOffset(x, y)+3] > aThresh {
				colFull[x] = true
				break
			}
		}
	}
	var runs [][2]int
	x := 0
	for x < w {
		if !colFull[x] {
			x++
			continue
		}
		start := x
		empty := 0
		for x < w && empty < gap {
			if colFull[x] {
				empty = 0
			} else {
				empty++
			}
			x++
		}
		end := x - empty
		runs = append(runs, [2]int{start, end})
	}
	return runs
}

func runBBox(strip *image.NRGBA, x0, x1, h int) (minX, minY, maxX, maxY int) {
	minX, minY, maxX, maxY = x1, h, x0-1, -1
	for x := x0; x < x1; x++ {
		for y := 0; y < h; y++ {
			if strip.Pix[strip.PixOffset(x, y)+3] > aThresh {
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
			}
		}
	}
	return
}

// bboxSharedExtract는 구 ExtractFrames 동작을 재현한다: 전체 프레임이 공유하는
// 글로벌 스케일을 "가장 큰 바운딩 박스"에서 끌어온다(=한 프레임의 뻗은 팔다리가
// 모든 프레임을 함께 축소시킴).
func bboxSharedExtract(strip *image.NRGBA, cellW, cellH, margin int) []*image.NRGBA {
	h := strip.Rect.Dy()
	runs := contentColRuns(strip, 6)
	type box struct{ minX, minY, maxX, maxY int }
	var boxes []box
	baseline := 0
	maxW, maxEffH := 1, 1
	for _, r := range runs {
		bx0, by0, bx1, by1 := runBBox(strip, r[0], r[1], h)
		if bx1 < bx0 {
			continue
		}
		boxes = append(boxes, box{bx0, by0, bx1, by1})
		if by1 > baseline {
			baseline = by1
		}
	}
	for _, b := range boxes {
		if w := b.maxX - b.minX + 1; w > maxW {
			maxW = w
		}
		if eff := (b.maxY - b.minY + 1) + (baseline - b.maxY); eff > maxEffH {
			maxEffH = eff
		}
	}
	availW, availH := cellW-margin*2, cellH-margin*2
	scale := float64(availW) / float64(maxW)
	if s := float64(availH) / float64(maxEffH); s < scale {
		scale = s
	}
	if scale > 1 {
		scale = 1
	}
	var frames []*image.NRGBA
	for _, b := range boxes {
		gw, gh := b.maxX-b.minX+1, b.maxY-b.minY+1
		sw, sh := int(float64(gw)*scale+0.5), int(float64(gh)*scale+0.5)
		src := image.NewNRGBA(image.Rect(0, 0, gw, gh))
		for y := b.minY; y <= b.maxY; y++ {
			for x := b.minX; x <= b.maxX; x++ {
				si := strip.PixOffset(x, y)
				if strip.Pix[si+3] > aThresh {
					di := src.PixOffset(x-b.minX, y-b.minY)
					copy(src.Pix[di:di+4], strip.Pix[si:si+4])
				}
			}
		}
		scaled := image.NewNRGBA(image.Rect(0, 0, sw, sh))
		xdraw.CatmullRom.Scale(scaled, scaled.Rect, src, src.Rect, xdraw.Over, nil)
		cell := newCanvas(cellW, cellH, color.NRGBA{0, 0, 0, 0})
		left := (cellW - sw) / 2
		offset := int(float64(baseline-b.maxY)*scale + 0.5)
		top := cellH - margin - offset - sh
		if top < 0 {
			top = 0
		}
		xdraw.Copy(cell, image.Point{X: left, Y: top}, scaled, scaled.Rect, xdraw.Over, nil)
		frames = append(frames, cell)
	}
	return frames
}

// bodyHeightPx는 프레임에서 가장 키 큰 불투명 열의 높이(렌더된 캐릭터 크기 척도).
func bodyHeightPx(f *image.NRGBA) int {
	w, h := f.Rect.Dx(), f.Rect.Dy()
	best := 0
	for x := 0; x < w; x++ {
		top, bot := -1, -1
		for y := 0; y < h; y++ {
			if f.Pix[f.PixOffset(x, y)+3] > aThresh {
				if top < 0 {
					top = y
				}
				bot = y
			}
		}
		if top >= 0 && bot-top+1 > best {
			best = bot - top + 1
		}
	}
	return best
}

func meanBodyHeight(frames []*image.NRGBA, skip int) float64 {
	sum, n := 0.0, 0
	for i, f := range frames {
		if i == skip {
			continue
		}
		sum += float64(bodyHeightPx(f))
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// scanBodyExtent는 모든 실스트립에서 구 bbox 공유 스케일과 신 ExtractFrames의
// 정상-프레임 본체 높이 비율을 비교해, body-extent 데모로 가장 극적인 후보를 찾는다.
func scanBodyExtent() {
	strips, _ := filepath.Glob("sample/*/*/_strip.png")
	type cand struct {
		path  string
		n     int
		ratio float64
	}
	var cands []cand
	for _, p := range strips {
		dir := filepath.Dir(p)
		n := countFrames(dir)
		if n < 3 {
			continue
		}
		strip, err := loadPNG(p)
		if err != nil {
			continue
		}
		without := bboxSharedExtract(strip, cell, cell, 16)
		ext := sprite.ExtractFrames(strip, n, cell, cell, 16)
		if len(without) < 2 || ext.Found != n {
			continue
		}
		hWo := meanBodyHeight(without, -1)
		hWi := meanBodyHeight(ext.Frames, -1)
		if hWi <= 0 {
			continue
		}
		cands = append(cands, cand{p, n, hWo / hWi})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].ratio < cands[j].ratio })
	for i := 0; i < len(cands) && i < 12; i++ {
		fmt.Printf("  %-46s n=%d  bodyHeight WITHOUT/WITH=%.2f\n", cands[i].path, cands[i].n, cands[i].ratio)
	}
}
