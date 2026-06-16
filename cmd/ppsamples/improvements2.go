package main

import (
	"fmt"
	"image"
	"image/color"

	"perfectpixel/internal/sprite"
)

// buildOverview는 저장된 전/후 비교 PNG들을 한 장으로 세로 합본해
// 수정 내역을 한눈에 볼 수 있는 컨택트 시트를 만든다.
func buildOverview() {
	files := []string{
		"01-matting.png", "02-segmentation.png", "03-centroid.png",
		"04-pixelize.png", "05-bodyextent.png", "06-overlap.png",
	}
	const colW = 920
	white := color.NRGBA{255, 255, 255, 255}
	panels := []*image.NRGBA{
		titleBar(colW, "PerfectPixel sprite pipeline — before / after at a glance (red = WITHOUT, green = WITH)"),
	}
	for _, f := range files {
		im, err := loadPNG("report-images/" + f)
		if err != nil {
			continue
		}
		if im.Rect.Dx() > colW {
			im = resizeW(im, colW)
		}
		panels = append(panels, im)
	}
	out := vstack(14, white, panels...)
	save("00-overview.png", out)
}

// transparentStrip은 알파 0 배경의 빈 스트립(이미 배경제거된 상태)을 만든다.
func transparentStrip(w, h int) *image.NRGBA {
	return newCanvas(w, h, color.NRGBA{0, 0, 0, 0})
}

// demoBodyExtent는 한 프레임의 뻗은 무기/팔이 전체 스케일을 지배해 모든 프레임을
// 축소시키던 문제를, body-extent 스케일(extract.go)이 어떻게 막는지
// 실제 AI 스트립(검을 뻗는 slash 포즈가 섞인 세트)으로 시연한다.
func demoBodyExtent() {
	fmt.Println("[5] body-extent scaling (extract.go) — 실제 AI 스트립의 뻗은 무기 outlier")
	strip, n := pickSampleStrip("sample/archer/slash-south-east", 5)

	without := bboxSharedExtract(strip, cell, cell, 16)
	ext := sprite.ExtractFrames(strip, n, cell, cell, 16)
	with := ext.Frames

	hWo := meanBodyHeight(without, -1)
	hWi := meanBodyHeight(with, -1)
	ratio := 0.0
	if hWi > 0 {
		ratio = hWo / hWi
	}
	fmt.Printf("  평균 본체 높이: 없음=%.0fpx  있음=%.0fpx  (없음이 %.0f%%로 축소)\n",
		hWo, hWi, ratio*100)

	gray := newCanvas(cell, cell, colBG)
	woRow := scaleRow(without, gray, 3)
	wiRow := scaleRow(with, gray, 3)
	dispW := woRow.Rect.Dx()
	if wiRow.Rect.Dx() > dispW {
		dispW = wiRow.Rect.Dx()
	}
	out := vstack(6, color.NRGBA{255, 255, 255, 255},
		titleBar(dispW, "5. Frame scale: body-extent vs bounding-box (extract.go) — real AI strip"),
		captionBar(dispW, "One pose thrusts a sword far out. Old global scale shrinks EVERY frame to fit that bbox.", colInk),
		labeledPanel(woRow, fmt.Sprintf("WITHOUT: bbox global scale — characters shrink to %.0f%% (outstretched weapon dominates)", ratio*100), colWithout),
		labeledPanel(wiRow, "WITH: body-extent scale — 80% alpha mass drives scale, character size stays consistent", colWith),
	)
	save("05-bodyextent.png", out)
}

// tightCrop은 콘텐츠 바운딩 박스로 잘라낸 새 이미지를 반환한다.
func tightCrop(im *image.NRGBA) *image.NRGBA {
	w, h := im.Rect.Dx(), im.Rect.Dy()
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if im.Pix[im.PixOffset(x, y)+3] > aThresh {
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
	if maxX < minX {
		return cloneNRGBA(im)
	}
	return cropRect(im, minX, minY, maxX-minX+1, maxY-minY+1)
}

// loadFrame은 sample 디렉토리의 frame-NN.png(최종 추출된 단일 포즈)를 읽는다.
func loadFrame(dir string, idx int) *image.NRGBA {
	im, err := loadPNG(fmt.Sprintf("%s/frame-%02d.png", dir, idx))
	if err != nil {
		return nil
	}
	return im
}

// demoOverlapRecovery는 실제 AI 포즈 두 개를 마젠타 거터 없이 맞붙여 한 덩어리로
// 만든 뒤, segment.go의 강제 분할이 이를 2개로 복구하는지 시연한다.
func demoOverlapRecovery() {
	fmt.Println("[6] overlap segmentation recovery (segment.go) — 실제 포즈를 거터 없이 맞붙임")
	const n = 2
	dir := "sample/ranger/walk"
	a, b := loadFrame(dir, 2), loadFrame(dir, 3)
	if a == nil || b == nil {
		fmt.Println("  [경고] 실제 프레임 없음 → 데모 생략")
		return
	}
	a, b = tightCrop(a), tightCrop(b)
	// 두 포즈를 8px만 겹치도록(거터 없음) 한 스트립에 바닥 정렬 배치.
	pad := 24
	overlap := 8
	stripW := pad + a.Rect.Dx() + b.Rect.Dx() - overlap + pad
	stripH := a.Rect.Dy()
	if b.Rect.Dy() > stripH {
		stripH = b.Rect.Dy()
	}
	stripH += 16
	strip := transparentStrip(stripW, stripH)
	paste(strip, a, pad, stripH-16-a.Rect.Dy())
	paste(strip, b, pad+a.Rect.Dx()-overlap, stripH-16-b.Rect.Dy())

	ext := sprite.ExtractFrames(strip, n, cell, cell, 16)
	with := ext.Frames
	merged := bboxSharedExtract(strip, cell, cell, 16)

	fmt.Printf("  검출 포즈 수: 없음=%d(병합)  있음=%d(복구)\n", len(merged), ext.Found)

	gray := newCanvas(cell, cell, colBG)
	woRow := scaleRow(merged, gray, 3)
	wiRow := scaleRow(with, gray, 3)
	stripView := resizeW(overOn(newCanvas(strip.Rect.Dx(), strip.Rect.Dy(), colBG), strip), 360)
	dispW := wiRow.Rect.Dx()
	for _, d := range []int{woRow.Rect.Dx(), stripView.Rect.Dx()} {
		if d > dispW {
			dispW = d
		}
	}
	out := vstack(6, color.NRGBA{255, 255, 255, 255},
		titleBar(dispW, "6. Overlap recovery: forced expected split (segment.go) — real AI poses"),
		captionBar(dispW, "Two real poses abutted with NO magenta gutter (AI sometimes draws them touching).", colInk),
		labeledPanel(stripView, "input: two poses merged into one blob (no gutter)", colInk),
		labeledPanel(woRow, fmt.Sprintf("WITHOUT: single peak read as %d pose (both bodies merged)", len(merged)), colWithout),
		labeledPanel(wiRow, fmt.Sprintf("WITH: DP forces expected count → %d clean poses recovered", ext.Found), colWith),
	)
	save("06-overlap.png", out)
}
