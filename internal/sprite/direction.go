package sprite

import "image"

// 8방향 캐릭터 스프라이트 지원.
// 5방향(south/north/east/south-east/north-east)은 AI로 생성하고
// 나머지 3방향(west/south-west/north-west)은 좌우 미러링으로 만듭니다.

// DirectionInfo는 프론트엔드에 노출되는 방향 메타데이터입니다.
type DirectionInfo struct {
	Key      string `json:"key"`      // "south", "north-east" 등
	Label    string `json:"label"`    // 한국어 표시명
	Short    string `json:"short"`    // 그리드용 약칭 (S, NE 등)
	MirrorOf string `json:"mirrorOf"` // 미러링 소스 방향 (빈 값이면 AI 생성)
	Row      int    `json:"row"`      // 3x3 그리드 행 (0~2)
	Col      int    `json:"col"`      // 3x3 그리드 열 (0~2)
}

// Directions는 8방향 전체 목록입니다 (3x3 그리드 순서).
var Directions = []DirectionInfo{
	{Key: "north-west", Label: "¾ 뒤·좌", Short: "NW", MirrorOf: "north-east", Row: 0, Col: 0},
	{Key: "north", Label: "뒷면", Short: "N", Row: 0, Col: 1},
	{Key: "north-east", Label: "¾ 뒤·우", Short: "NE", Row: 0, Col: 2},
	{Key: "west", Label: "좌측면", Short: "W", MirrorOf: "east", Row: 1, Col: 0},
	{Key: "east", Label: "우측면", Short: "E", Row: 1, Col: 2},
	{Key: "south-west", Label: "¾ 앞·좌", Short: "SW", MirrorOf: "south-east", Row: 2, Col: 0},
	{Key: "south", Label: "정면", Short: "S", Row: 2, Col: 1},
	{Key: "south-east", Label: "¾ 앞·우", Short: "SE", Row: 2, Col: 2},
}

// GeneratedDirections는 AI로 생성하는 방향 순서입니다 (south가 정면 레퍼런스라 첫 번째).
var GeneratedDirections = []string{"south", "east", "north", "south-east", "north-east"}

// ListDirections는 방향 메타데이터 목록을 반환합니다.
func ListDirections() []DirectionInfo {
	return Directions
}

// DirectionByKey는 키에 해당하는 방향 정보를 반환합니다.
func DirectionByKey(key string) (DirectionInfo, bool) {
	for _, d := range Directions {
		if d.Key == key {
			return d, true
		}
	}
	return DirectionInfo{}, false
}

// IsBackFacing은 얼굴이 보이지 않는 뒷면 계열 방향인지 판정합니다.
// (베이스 정면 이미지 대비 색 히스토그램 정체성 검사가 오탐하기 쉬운 방향)
func IsBackFacing(key string) bool {
	switch key {
	case "north", "north-east", "north-west":
		return true
	}
	return false
}

// facingDesc는 방향별 카메라/신체 묘사입니다.
type facingDesc struct {
	view       string // 뷰 타입 요약
	camera     string // 카메라 앵글
	body       string // 신체 방향
	visibility string // 보이는 신체 부위
}

var facingDescs = map[string]facingDesc{
	"south": {
		view:       "front view",
		camera:     "camera directly in front, at eye level",
		body:       "the character faces the viewer directly",
		visibility: "full face visible (eyes and mouth, minimal nose detail); both arms and both legs fully visible, symmetric",
	},
	"north": {
		view:       "back view",
		camera:     "camera positioned directly behind the character",
		body:       "the character faces away from the viewer",
		visibility: "face completely hidden, only the back of the head and hair visible; back of the outfit, both arms and legs seen from behind",
	},
	"east": {
		view:       "right-side profile view",
		camera:     "camera locked at the character's right side, perfectly perpendicular — strictly 2D flat profile with zero perspective rotation at any point",
		body:       "the character faces and moves toward the RIGHT edge of the canvas in every single frame",
		visibility: "EXACTLY ONE eye (the right/near-side eye) visible — if two eyes appear the head has rotated wrong; EXACTLY ONE ear (the right/near ear) visible; right arm and right leg prominent in the foreground; the entire left side of the body (left arm, left leg, left eye, left ear) is hidden behind the body in every frame; the nose/snout always points RIGHT",
	},
	"south-east": {
		view:       "three-quarter front-right view",
		camera:     "camera at front-right, rotated about 45 degrees from straight ahead",
		body:       "the character is turned about 45 degrees to the right, mostly facing the viewer",
		visibility: "3/4 face with both eyes visible, right side emphasized; right arm and leg fully visible, left side partially visible",
	},
	"north-east": {
		view:       "three-quarter back-right view",
		camera:     "camera behind and to the right, rotated about 45 degrees",
		body:       "the character is turned away from the viewer, showing the back-right side",
		visibility: "face hidden except a hint of the right jaw; back and right shoulder prominent, right arm and leg visible from behind",
	},
}

// FacingPromptSection은 스트립 프롬프트에 삽입할 방향 잠금 지시문을 만듭니다.
// 알 수 없는 방향이면 빈 문자열을 반환합니다.
func FacingPromptSection(key string) string {
	d, ok := facingDescs[key]
	if !ok {
		return ""
	}
	return "Facing direction lock (overrides any other facing or view instruction in this prompt):\n" +
		"- Required view: " + d.view + " — " + d.camera + ".\n" +
		"- Body orientation: " + d.body + ".\n" +
		"- Visibility: " + d.visibility + ".\n" +
		"- The attached reference image shows this character from the front; redraw the IDENTICAL character (same hair, outfit, colors, proportions) rotated to this view.\n" +
		"- Every frame in the strip must use this exact same viewing angle. Never drift back toward a front view and never mirror the character between frames.\n"
}

// MirrorNRGBA는 이미지를 좌우 반전한 새 이미지를 반환합니다 (8방향 미러 페어 생성용).
func MirrorNRGBA(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		srcRow := src.PixOffset(b.Min.X, b.Min.Y+y)
		dstRow := dst.PixOffset(0, y)
		for x := 0; x < w; x++ {
			s := srcRow + x*4
			d := dstRow + (w-1-x)*4
			copy(dst.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return dst
}
