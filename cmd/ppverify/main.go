// Command ppverify는 AI 호출 없이 sample/ 의 실제 스트립 100개에 대해
// 현재(신) 파이프라인을 그대로 재실행하여 품질을 정량 측정하고,
// 과거 베이스라인(sample/report.json)과 비교한다.
// 목적: 알고리즘 개선이 실데이터에서 실제 효과가 있는지 검증.
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"

	_ "image/gif"
	_ "image/jpeg"

	_ "golang.org/x/image/webp"

	"perfectpixel/internal/sprite"
)

type baseline struct {
	PassRate float64 `json:"passRate"`
	AvgScore float64 `json:"avgScore"`
	Results  []struct {
		Name     string  `json:"name"`
		Expected int     `json:"expected"`
		Found    int     `json:"found"`
		Score    int     `json:"score"`
		Identity float64 `json:"identity"`
		Motion   float64 `json:"motion"`
	} `json:"results"`
}

type row struct {
	path                               string
	expected, found                    int
	identity, motion, contact, overall float64
	errs                               int
}

func loadPNG(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return sprite.ToNRGBA(img), nil
}

func countFrames(dir string) int {
	m, _ := filepath.Glob(filepath.Join(dir, "frame-*.png"))
	return len(m)
}

func main() {
	strips, _ := filepath.Glob("sample/*/*/_strip.png")
	sort.Strings(strips)
	if len(strips) == 0 {
		fmt.Println("no sample strips found (run from repo root)")
		os.Exit(1)
	}

	var rows []row
	var sumId, sumMo, sumCo, sumOv float64
	hit := 0
	var regress []row

	for _, sp := range strips {
		dir := filepath.Dir(sp)
		expected := countFrames(dir)
		if expected == 0 {
			continue
		}
		nimg, err := loadPNG(sp)
		if err != nil {
			fmt.Printf("decode fail %s: %v\n", sp, err)
			continue
		}
		// _strip.png는 이미 배경 제거된 투명 스트립이므로 RemoveBackground를
		// 다시 돌리지 않고 그대로 ExtractFrames에 넣는다(ppsamples scan과 동일).
		ext := sprite.ExtractFrames(nimg, expected, 256, 256, 16)
		insp := sprite.InspectFrames(ext.Frames, [3]uint8{255, 0, 255}, nil)
		sc := sprite.ScoreFrames(ext.Frames)

		r := row{
			path: dir, expected: expected, found: ext.Found,
			identity: sc.Identity, motion: sc.Motion,
			contact: sc.Contact, overall: sc.Overall, errs: len(insp.Errors),
		}
		rows = append(rows, r)
		sumId += sc.Identity
		sumMo += sc.Motion
		sumCo += sc.Contact
		sumOv += sc.Overall
		if ext.Found == expected {
			hit++
		} else {
			regress = append(regress, r)
		}
	}

	n := float64(len(rows))
	fmt.Printf("\n=== NEW pipeline on %d real sample strips ===\n", len(rows))
	fmt.Printf("frame-count accuracy : %d/%d (%.1f%%)\n", hit, len(rows), 100*float64(hit)/n)
	fmt.Printf("mean identity        : %.3f\n", sumId/n)
	fmt.Printf("mean motion          : %.3f\n", sumMo/n)
	fmt.Printf("mean contact (new)   : %.3f\n", sumCo/n)
	fmt.Printf("mean overall (new)   : %.3f\n", sumOv/n)

	// 베이스라인 비교
	if bf, err := os.ReadFile("sample/report.json"); err == nil {
		var bl baseline
		if json.Unmarshal(bf, &bl) == nil && len(bl.Results) > 0 {
			var oId, oMo float64
			oHit := 0
			for _, r := range bl.Results {
				oId += r.Identity
				oMo += r.Motion
				if r.Found == r.Expected {
					oHit++
				}
			}
			bn := float64(len(bl.Results))
			fmt.Printf("\n=== OLD baseline (sample/report.json) ===\n")
			fmt.Printf("frame-count accuracy : %d/%d (%.1f%%)\n", oHit, len(bl.Results), 100*float64(oHit)/bn)
			fmt.Printf("mean identity (dHash): %.3f\n", oId/bn)
			fmt.Printf("mean motion          : %.3f\n", oMo/bn)
			fmt.Printf("avg score / passRate : %.1f / %.1f%%\n", bl.AvgScore, bl.PassRate)
		}
	}

	if len(regress) > 0 {
		fmt.Printf("\n=== frame-count mismatches (NEW) %d ===\n", len(regress))
		for _, r := range regress {
			fmt.Printf("  %-48s expected=%d found=%d errs=%d\n", r.path, r.expected, r.found, r.errs)
		}
	}

	// 회귀 가드: 새 파이프라인 프레임 정확도가 85% 미만이면 실패 종료
	if float64(hit)/n < 0.85 {
		fmt.Printf("\nFAIL: frame accuracy below 85%%\n")
		os.Exit(1)
	}
	fmt.Printf("\nPASS\n")
}
