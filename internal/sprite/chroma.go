package sprite

import (
	"image"
	"image/draw"
	"math"
)

// 색차 평면(CbCr) 기반 적응형 매팅 파라미터.
// 루마(Y)를 무시하고 채도/색상만으로 키를 분리하므로 JPEG 4:2:0 압축에 강건합니다.
const (
	chromaIn     = 24.0  // CbCr 거리 이 이하 → 완전 투명 (키 색)
	chromaOut    = 72.0  // CbCr 거리 이 이상 → 완전 불투명 (피사체)
	despillBand  = 100.0 // 이 거리 안쪽 픽셀은 키 색조 번짐(despill) 보정 대상
	despillScale = 0.92  // despill 강도 (키 방향 채도 억제율)
	floodTol     = 88.0  // 테두리 시드 플러드필이 배경으로 간주하는 CbCr 거리(관대)
)

// ycc는 BT.601 YCbCr 좌표입니다 (8bit, 128 중심).
type ycc struct{ y, cb, cr float64 }

func toYCC(r, g, b uint8) ycc {
	fr, fg, fb := float64(r), float64(g), float64(b)
	y := 0.299*fr + 0.587*fg + 0.114*fb
	return ycc{
		y:  y,
		cb: (fb-y)*0.564 + 128,
		cr: (fr-y)*0.713 + 128,
	}
}

func fromYCC(c ycc) (uint8, uint8, uint8) {
	r := c.y + 1.402*(c.cr-128)
	g := c.y - 0.344136*(c.cb-128) - 0.714136*(c.cr-128)
	b := c.y + 1.772*(c.cb-128)
	return u8(r), u8(g), u8(b)
}

func u8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// smoothstep은 Hermite 보간으로 부드러운 0→1 전이를 만듭니다 (가장자리 페더링).
func smoothstep(edge0, edge1, x float64) float64 {
	if edge1 <= edge0 {
		return 0
	}
	t := (x - edge0) / (edge1 - edge0)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}

// ToNRGBA는 임의 이미지를 좌상단 원점 NRGBA로 변환합니다.
func ToNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	dst := image.NewNRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

// DetectBackground는 테두리 픽셀의 색차(CbCr) 최빈 클러스터로 배경 키 색을 추정합니다.
// 단순 RGB 평균이 아니라 CbCr 평면 양자화 히스토그램의 모드를 찾으므로
// 그라데이션/압축 노이즈가 섞인 배경에서도 안정적으로 지배 색상을 잡습니다.
func DetectBackground(img *image.NRGBA) [3]uint8 {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w == 0 || h == 0 {
		return [3]uint8{255, 0, 255}
	}
	type acc struct {
		n          int
		sr, sg, sb int
	}
	bins := map[int]*acc{}
	total := 0
	var magN, magR, magG, magB int // 마젠타 계열(R·B 강, G 약) 누적
	visit := func(x, y int) {
		i := img.PixOffset(x, y)
		r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
		total++
		if r > 150 && b > 150 && g < 120 { // 마젠타 계열
			magN++
			magR += int(r)
			magG += int(g)
			magB += int(b)
		}
		c := toYCC(r, g, b)
		key := int(c.cb)>>3<<6 | int(c.cr)>>3 // 8단위 CbCr 양자화
		a := bins[key]
		if a == nil {
			a = &acc{}
			bins[key] = a
		}
		a.n++
		a.sr += int(r)
		a.sg += int(g)
		a.sb += int(b)
	}
	// 넓은 포즈(걷기 등)는 테두리 전체에 닿아 캐릭터 색이 키 추정을 오염시킨다.
	// 모서리(코너) 사각 패치는 거의 항상 배경이므로 코너를 중심으로 샘플링한다.
	cw, ch := w/5, h/5
	if cw < 2 {
		cw = w
	}
	if ch < 2 {
		ch = h
	}
	corner := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				visit(x, y)
			}
		}
	}
	corner(0, 0, cw, ch)
	corner(w-cw, 0, w, ch)
	corner(0, h-ch, cw, h)
	corner(w-cw, h-ch, w, h)
	// 얇은 테두리도 보조로 (코너가 캐릭터에 가린 드문 경우 대비)
	for x := 0; x < w; x++ {
		visit(x, 0)
		visit(x, h-1)
	}
	for y := 0; y < h; y++ {
		visit(0, y)
		visit(w-1, y)
	}
	// 마젠타 바이어스: 이 파이프라인은 항상 마젠타 키를 의도하므로, 테두리/코너에
	// 마젠타 계열이 충분히(샘플의 12%+) 존재하면 — 넓은/누운 포즈가 코너를 채워
	// 전체 최빈색이 캐릭터 색이 되더라도 — 마젠타 클러스터를 키로 확정한다.
	if total > 0 && magN >= total*12/100 {
		return [3]uint8{uint8(magR / magN), uint8(magG / magN), uint8(magB / magN)}
	}
	var best *acc
	for _, a := range bins {
		if best == nil || a.n > best.n {
			best = a
		}
	}
	if best == nil || best.n == 0 {
		return [3]uint8{255, 0, 255}
	}
	return [3]uint8{uint8(best.sr / best.n), uint8(best.sg / best.n), uint8(best.sb / best.n)}
}

// RemoveBackground는 배경 키를 자동 감지해 색차 평면 매팅으로 투명 처리합니다.
// (1) CbCr 거리 기반 소프트 알파 램프 → 가장자리 페더링, (2) 색차공간 despill로
// 키 색조 번짐 제거, (3) 고립 점/핀홀 형태학적 정리.
func RemoveBackground(src image.Image) *image.NRGBA {
	img := ToNRGBA(src)
	key := DetectBackground(img)
	out, frac := matteWith(img, key)

	// 안전장치: 넓은 포즈가 테두리를 채워 키를 캐릭터 색으로 오인하면 매팅이
	// 배경 대신 캐릭터를 지우거나(불투명 비율 급증) 마젠타 배경을 부분만 지운다
	// (마젠타 잔여 급증). 이 파이프라인은 항상 마젠타 키를 의도하므로, 두 증상 중
	// 하나라도 보이면 순수 마젠타(#FF00FF)로 폴백 재매팅해 더 나은 쪽을 택한다.
	if frac > 0.60 || magentaResidueFrac(out) > 0.025 {
		out2, frac2 := matteWith(img, [3]uint8{255, 0, 255})
		betterFrac := frac2 < frac-0.03 && frac2 > 0.02
		lessResidue := magentaResidueFrac(out2) < magentaResidueFrac(out)
		if (betterFrac || lessResidue) && frac2 > 0.02 {
			out = out2
		}
	}
	// 검색/다크 배경 fallback: 어두운/무채색 키가 감지되면 pure magenta 매팅도
	// 시도해 더 나은 쪽(마젠타 잔여가 적은 쪽)을 택한다.
	if !isMagentaKey(key) {
		out2, frac2 := matteWith(img, [3]uint8{255, 0, 255})
		if frac2 > 0.02 && magentaResidueFrac(out2) < magentaResidueFrac(out) {
			out = out2
		}
	}
	cleanupAlpha(out)
	return out
}

// magentaResidueFrac은 순수 마젠타에 가까운(CbCr<55) 불투명 픽셀의 전체 대비 비율입니다.
// 매팅이 마젠타 배경을 다 지웠는지 판정하는 증상 지표입니다.
func magentaResidueFrac(img *image.NRGBA) float64 {
	mk := toYCC(255, 0, 255)
	n := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if img.Pix[i+3] <= alphaThreshold {
			continue
		}
		c := toYCC(img.Pix[i], img.Pix[i+1], img.Pix[i+2])
		if math.Hypot(c.cb-mk.cb, c.cr-mk.cr) < 55 {
			n++
		}
	}
	total := len(img.Pix) / 4
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total)
}

// isMagentaKey는 키 색이 마젠타 계열(R·B 강, G 약)인지 판정합니다.
func isMagentaKey(k [3]uint8) bool {
	return k[0] > 150 && k[2] > 150 && k[1] < 120
}

// matteWith는 주어진 키로 색차 평면 매팅 + despill + 플러드필을 수행하고,
// 결과 이미지와 불투명(알파>임계) 픽셀 비율을 반환합니다.
func matteWith(img *image.NRGBA, key [3]uint8) (*image.NRGBA, float64) {
	kc := toYCC(key[0], key[1], key[2])
	kvb, kvr := kc.cb-128, kc.cr-128
	klen := math.Hypot(kvb, kvr)
	out := image.NewNRGBA(img.Rect)

	for i := 0; i+3 < len(img.Pix); i += 4 {
		r, g, b, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
		if a == 0 {
			continue
		}
		c := toYCC(r, g, b)
		dist := math.Hypot(c.cb-kc.cb, c.cr-kc.cr)
		alpha := smoothstep(chromaIn, chromaOut, dist)
		if alpha <= 0 {
			continue
		}
		if klen > 1 && dist < despillBand {
			pcb, pcr := c.cb-128, c.cr-128
			proj := (pcb*kvb + pcr*kvr) / klen
			if proj > 0 {
				wgt := smoothstep(0, 1, (despillBand-dist)/despillBand) * despillScale
				ub, ur := kvb/klen, kvr/klen
				c.cb = 128 + (pcb - ub*proj*wgt)
				c.cr = 128 + (pcr - ur*proj*wgt)
				r, g, b = fromYCC(c)
			}
		}
		out.Pix[i] = r
		out.Pix[i+1] = g
		out.Pix[i+2] = b
		out.Pix[i+3] = uint8(float64(a) * alpha)
	}

	floodClearBackground(out, img, kc)

	opaque := 0
	for i := 3; i < len(out.Pix); i += 4 {
		if out.Pix[i] > alphaThreshold {
			opaque++
		}
	}
	frac := float64(opaque) / float64(len(out.Pix)/4)
	return out, frac
}

// floodClearBackground는 테두리에서 출발해 키 색에 가까운(관대 허용) 픽셀을 따라
// 4방향 플러드필하며 알파를 0으로 만듭니다. 소프트 매팅만으로는 못 지우는
// 그라데이션/노이즈 배경(걷기처럼 넓은 포즈가 키를 흔든 경우)을 연결성 기준으로
// 확실히 제거하되, 테두리와 단절된 내부 캐릭터 픽셀(설령 키색이어도)은 보존합니다.
func floodClearBackground(out *image.NRGBA, orig *image.NRGBA, kc ycc) {
	w, h := orig.Rect.Dx(), orig.Rect.Dy()
	if w < 3 || h < 3 {
		return
	}
	isKey := func(x, y int) bool {
		i := orig.PixOffset(x, y)
		c := toYCC(orig.Pix[i], orig.Pix[i+1], orig.Pix[i+2])
		return math.Hypot(c.cb-kc.cb, c.cr-kc.cr) <= floodTol
	}
	visited := make([]bool, w*h)
	stack := make([]int, 0, 4096)
	push := func(x, y int) {
		p := y*w + x
		if !visited[p] && isKey(x, y) {
			visited[p] = true
			stack = append(stack, p)
		}
	}
	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := p%w, p/w
		out.Pix[p*4+3] = 0 // 배경 → 투명
		if x > 0 {
			push(x-1, y)
		}
		if x < w-1 {
			push(x+1, y)
		}
		if y > 0 {
			push(x, y-1)
		}
		if y < h-1 {
			push(x, y+1)
		}
	}
}

// cleanupAlpha는 고립된 불투명 점(JPEG 블록 잡티)을 제거하고 1px 핀홀을 메웁니다.
// 소프트 가장자리는 보존하기 위해 명확히 고립된/둘러싸인 픽셀만 손봅니다.
func cleanupAlpha(img *image.NRGBA) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w < 3 || h < 3 {
		return
	}
	orig := make([]uint8, w*h)
	for p := 0; p < w*h; p++ {
		orig[p] = img.Pix[p*4+3]
	}
	opaque := func(x, y int) int {
		if x < 0 || y < 0 || x >= w || y >= h {
			return 0
		}
		if orig[y*w+x] > alphaThreshold {
			return 1
		}
		return 0
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			nb := opaque(x-1, y) + opaque(x+1, y) + opaque(x, y-1) + opaque(x, y+1) +
				opaque(x-1, y-1) + opaque(x+1, y-1) + opaque(x-1, y+1) + opaque(x+1, y+1)
			if orig[y*w+x] > alphaThreshold {
				if nb == 0 { // 완전 고립된 점 → 제거
					img.Pix[i+3] = 0
				}
			} else if nb >= 7 { // 거의 둘러싸인 핀홀 → 채움
				img.Pix[i+3] = 255
			}
		}
	}
}

// colorDist는 RGB 유클리드 거리입니다 (inspect의 잔여 크로마 판정용).
func colorDist(r, g, b uint8, bg [3]uint8) float64 {
	dr := float64(r) - float64(bg[0])
	dg := float64(g) - float64(bg[1])
	db := float64(b) - float64(bg[2])
	return math.Sqrt(dr*dr + dg*dg + db*db)
}
