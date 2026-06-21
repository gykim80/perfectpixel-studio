package sprite

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"testing"
	"time"
)

func loadNRGBA(t testing.TB, path string) *image.NRGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("sample not available: %v", err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	b := src.Bounds()
	img := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(img, img.Bounds(), src, b.Min, draw.Src)
	return img
}

// uniqueColors는 불투명 픽셀의 고유 색 수를 셉니다(낮을수록 깔끔한 픽셀아트).
func uniqueColors(img *image.NRGBA) int {
	seen := make(map[rgb]struct{})
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if img.Pix[i+3] <= alphaThreshold {
			continue
		}
		seen[rgb{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}] = struct{}{}
	}
	return len(seen)
}

// blockEdgeJaggedness는 셀 경계를 가로지르는 인접 픽셀 색 변화량의 합입니다.
// 그리드가 잘 정렬되면(셀 내부가 평탄) 작아집니다.
func gridAlignmentError(img *image.NRGBA, cols, rows int) float64 {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if cols < 1 || rows < 1 {
		return -1
	}
	var withinCellChange float64
	var n int
	for ry := 0; ry < rows; ry++ {
		y0, y1 := ry*h/rows, (ry+1)*h/rows
		for cx := 0; cx < cols; cx++ {
			x0, x1 := cx*w/cols, (cx+1)*w/cols
			for py := y0; py < y1; py++ {
				for px := x0; px+1 < x1; px++ {
					a := img.PixOffset(px, py)
					b := img.PixOffset(px+1, py)
					withinCellChange += float64(absInt(int(img.Pix[a])-int(img.Pix[b])) +
						absInt(int(img.Pix[a+1])-int(img.Pix[b+1])) +
						absInt(int(img.Pix[a+2])-int(img.Pix[b+2])))
					n++
				}
			}
		}
	}
	if n == 0 {
		return 0
	}
	return withinCellChange / float64(n)
}

func TestSnapPerfComparison(t *testing.T) {
	for _, name := range []string{"knight", "ranger", "slime"} {
		path := "../../sample/" + name + "/base.png"
		img := loadNRGBA(t, path)

		oldImg := image.NewNRGBA(img.Bounds())
		copy(oldImg.Pix, img.Pix)
		t0 := time.Now()
		scale := DetectPixelScale(oldImg)
		oldOut := Pixelize(oldImg, scale)
		oldDur := time.Since(t0)
		oldCols := 0
		if scale >= 2 {
			oldCols = img.Rect.Dx() / scale
		}

		newImg := image.NewNRGBA(img.Bounds())
		copy(newImg.Pix, img.Pix)
		t1 := time.Now()
		cols, rows, ok := detectGridCounts(newImg)
		var newOut *image.NRGBA
		if ok {
			newOut = SnapToGrid(newImg, cols, rows)
		} else {
			newOut = newImg
		}
		newDur := time.Since(t1)

		t.Logf("[%s] OLD scale=%d (=%d cols) %v colors=%d align=%.1f | NEW grid=%dx%d ok=%v %v colors=%d align=%.1f",
			name, scale, oldCols, oldDur.Round(time.Millisecond), uniqueColors(oldOut), gridAlignmentError(oldOut, max(oldCols, 1), max(oldCols, 1)),
			cols, rows, ok, newDur.Round(time.Millisecond), uniqueColors(newOut), gridAlignmentError(newOut, max(cols, 1), max(rows, 1)))
	}
}

func BenchmarkDetectGridCounts(b *testing.B) {
	img := loadNRGBA(b, "../../sample/knight/base.png")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detectGridCounts(img)
	}
}

func BenchmarkDetectPixelScale(b *testing.B) {
	img := loadNRGBA(b, "../../sample/knight/base.png")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectPixelScale(img)
	}
}
