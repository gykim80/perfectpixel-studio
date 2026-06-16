// Command ppsamples는 기술 분석 보고서용 "기술 적용 전/후" 비교 이미지를
// 실제 sprite 파이프라인 코드로 생성한다. AI 호출 없이 합성 마젠타 스트립을
// 입력으로 써서, 각 알고리즘을 켰을 때와 껐을 때를 나란히 보여준다.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"perfectpixel/internal/sprite"
)

const outDir = "report-images"

func save(name string, im image.Image) {
	p := filepath.Join(outDir, name)
	f, err := os.Create(p)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, im); err != nil {
		panic(err)
	}
	fmt.Printf("  saved %s (%dx%d)\n", p, im.Bounds().Dx(), im.Bounds().Dy())
}

// labeledPanel은 이미지 위에 색 라벨 바를 붙인 패널을 만든다.
// 라벨이 이미지보다 길면 바 폭을 넓히고 이미지를 가운데 정렬한다(텍스트 잘림 방지).
func labeledPanel(im *image.NRGBA, label string, bar color.NRGBA) *image.NRGBA {
	w := textWidth(label, im.Rect.Dx())
	barH := 26
	out := newCanvas(w, im.Rect.Dy()+barH, color.NRGBA{255, 255, 255, 255})
	fillRect(out, 0, 0, w, barH, bar)
	drawText(out, 8, 17, label, color.NRGBA{255, 255, 255, 255})
	paste(out, im, (w-im.Rect.Dx())/2, barH)
	return out
}

// hstack은 패널들을 가로로 이어 붙인다(간격 gap).
func hstack(gap int, bg color.NRGBA, panels ...*image.NRGBA) *image.NRGBA {
	w, h := 0, 0
	for i, p := range panels {
		w += p.Rect.Dx()
		if i > 0 {
			w += gap
		}
		if p.Rect.Dy() > h {
			h = p.Rect.Dy()
		}
	}
	out := newCanvas(w, h, bg)
	x := 0
	for _, p := range panels {
		paste(out, p, x, 0)
		x += p.Rect.Dx() + gap
	}
	return out
}

// vstack은 패널들을 세로로 이어 붙인다.
func vstack(gap int, bg color.NRGBA, panels ...*image.NRGBA) *image.NRGBA {
	w, h := 0, 0
	for i, p := range panels {
		h += p.Rect.Dy()
		if i > 0 {
			h += gap
		}
		if p.Rect.Dx() > w {
			w = p.Rect.Dx()
		}
	}
	out := newCanvas(w, h, bg)
	y := 0
	for _, p := range panels {
		paste(out, p, 0, y)
		y += p.Rect.Dy() + gap
	}
	return out
}

// textWidth는 basicfont(7px 고정폭) 기준 텍스트 픽셀 폭(여백 포함)을 추정한다.
func textWidth(s string, minW int) int {
	w := len(s)*7 + 16
	if w < minW {
		return minW
	}
	return w
}

// titleBar는 제목 텍스트 한 줄 패널을 만든다(텍스트가 안 잘리게 폭 자동 조정).
func titleBar(w int, s string) *image.NRGBA {
	im := newCanvas(textWidth(s, w), 30, color.NRGBA{255, 255, 255, 255})
	drawText(im, 8, 20, s, colInk)
	return im
}

func captionBar(w int, s string, c color.NRGBA) *image.NRGBA {
	im := newCanvas(textWidth(s, w), 22, color.NRGBA{255, 255, 255, 255})
	drawText(im, 8, 15, s, c)
	return im
}

// frameRow는 프레임들을 가로로 배치한 행을 만든다(셀 경계선 포함).
func frameRow(frames []*image.NRGBA, over *image.NRGBA, centerLine bool) *image.NRGBA {
	if len(frames) == 0 {
		return newCanvas(64, 64, color.NRGBA{255, 255, 255, 255})
	}
	cw, ch := frames[0].Rect.Dx(), frames[0].Rect.Dy()
	row := newCanvas(cw*len(frames), ch, color.NRGBA{255, 255, 255, 255})
	for i, f := range frames {
		cell := overOn(over, f)
		paste(row, cell, i*cw, 0)
		// 셀 경계
		fillRect(row, i*cw, 0, i*cw+1, ch, color.NRGBA{180, 184, 190, 255})
		if centerLine {
			cx := i*cw + cw/2
			for y := 0; y < ch; y += 4 {
				setPx(row, cx, y, color.NRGBA{220, 40, 40, 255})
			}
		}
	}
	return row
}

func main() {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "scan" {
		scanSamples()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "scan2" {
		scanBodyExtent()
		return
	}
	fmt.Println("== PerfectPixel 기술 비교 이미지 생성 (실제 AI 파이프라인) ==")
	// 매팅·픽셀화: 새로 생성한 마젠타 베이스 캐릭터(실제 AI)
	base, err := buildRealBase()
	if err != nil {
		fmt.Println("  [경고] 실제 생성 실패 → 합성 폴백:", err)
		base = synthBase()
	}
	// 분할·정렬: sample/의 실제 AI 매팅 스트립 중 대비가 큰 케이스(스캔으로 선정)
	segMatte, segN := pickSampleStrip("sample/fire-mage/kick-south-east", 9)
	cenMatte, cenN := pickSampleStrip("sample/fire-mage/dash", 5)

	demoMatting(base)
	demoSegmentation(segMatte, segN)
	demoCentroid(cenMatte, cenN)
	demoPixelize(base)
	demoBodyExtent()
	demoOverlapRecovery()
	buildOverview()
	fmt.Println("완료. report-images/ 확인.")
}

// pickSampleStrip은 sample/<char>/<state>/_strip.png(실제 AI 매팅 스트립)를 읽는다.
// 없으면 합성 스트립을 매팅해 폴백한다. (n = 디렉토리의 frame 개수)
func pickSampleStrip(dir string, fallbackN int) (*image.NRGBA, int) {
	p := filepath.Join(dir, "_strip.png")
	if im, err := loadPNG(p); err == nil {
		n := countFrames(dir)
		if n < 2 {
			n = fallbackN
		}
		fmt.Printf("  사용 스트립: %s (n=%d)\n", p, n)
		return im, n
	}
	fmt.Printf("  [경고] %s 없음 → 합성 매팅 폴백\n", p)
	return sprite.RemoveBackground(synthStrip(fallbackN)), fallbackN
}

// ---- 합성 폴백 입력 (실제 생성 실패 시에만 사용) ----

func synthBase() *image.NRGBA {
	im := magentaCanvas(cell, cell)
	drawChar(im, cell/2, cell-28, 1.0, idlePose())
	shadeGradient(im)
	return jpegRoundTrip(im, 88)
}

func synthStrip(n int) *image.NRGBA {
	W := cell * n
	im := magentaCanvas(W, cell)
	for i := 0; i < n; i++ {
		p := idlePose()
		p.armLen = 70
		swing := float64((i%2)*2-1) * 55
		p.rArm, p.lArm = swing, -swing/2
		p.lLeg, p.rLeg = swing/4, -swing/4
		drawChar(im, cell*i+cell/2, cell-28, 1.0, p)
	}
	return jpegRoundTrip(im, 86)
}

// ensure sprite import used.
var _ = sprite.RemoveBackground
