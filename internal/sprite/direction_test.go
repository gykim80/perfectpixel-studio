package sprite

import (
	"image"
	"strings"
	"testing"
)

func TestDirectionsMetadata(t *testing.T) {
	if len(Directions) != 8 {
		t.Fatalf("8방향이어야 함, got %d", len(Directions))
	}
	gen, mirrored := 0, 0
	seen := map[string]bool{}
	grid := map[[2]int]bool{}
	for _, d := range Directions {
		if seen[d.Key] {
			t.Errorf("중복 방향 키: %s", d.Key)
		}
		seen[d.Key] = true
		if d.Row < 0 || d.Row > 2 || d.Col < 0 || d.Col > 2 {
			t.Errorf("%s: 그리드 좌표 범위 초과 (%d,%d)", d.Key, d.Row, d.Col)
		}
		if grid[[2]int{d.Row, d.Col}] {
			t.Errorf("%s: 그리드 좌표 중복 (%d,%d)", d.Key, d.Row, d.Col)
		}
		grid[[2]int{d.Row, d.Col}] = true
		if d.MirrorOf == "" {
			gen++
			if FacingPromptSection(d.Key) == "" {
				t.Errorf("AI 생성 방향 %s에 프롬프트 지시문이 없음", d.Key)
			}
		} else {
			mirrored++
			src, ok := DirectionByKey(d.MirrorOf)
			if !ok {
				t.Errorf("%s: 미러 소스 %s가 존재하지 않음", d.Key, d.MirrorOf)
			} else if src.MirrorOf != "" {
				t.Errorf("%s: 미러 소스 %s도 미러 방향임", d.Key, d.MirrorOf)
			}
		}
	}
	if gen != 5 || mirrored != 3 {
		t.Errorf("AI 생성 5 + 미러 3이어야 함, got gen=%d mirrored=%d", gen, mirrored)
	}
	if len(GeneratedDirections) != 5 || GeneratedDirections[0] != "south" {
		t.Errorf("GeneratedDirections는 south가 첫 번째인 5개여야 함: %v", GeneratedDirections)
	}
}

func TestIsBackFacing(t *testing.T) {
	for _, k := range []string{"north", "north-east", "north-west"} {
		if !IsBackFacing(k) {
			t.Errorf("%s는 뒷면이어야 함", k)
		}
	}
	for _, k := range []string{"", "south", "east", "west", "south-east", "south-west"} {
		if IsBackFacing(k) {
			t.Errorf("%s는 뒷면이 아니어야 함", k)
		}
	}
}

func TestFacingPromptSection(t *testing.T) {
	if FacingPromptSection("") != "" {
		t.Error("빈 방향은 빈 지시문이어야 함")
	}
	if FacingPromptSection("west") != "" {
		t.Error("미러 방향(west)은 AI 프롬프트가 없어야 함")
	}
	sec := FacingPromptSection("north")
	if !strings.Contains(sec, "back view") || !strings.Contains(sec, "Facing direction lock") {
		t.Errorf("north 지시문에 back view 잠금이 없음: %q", sec)
	}
}

func TestBuildStripPromptIncludesFacing(t *testing.T) {
	spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Action: "walking", Facing: "east"}
	p := BuildStripPrompt("a knight", StylePresets["pixel"], spec, "", 256)
	if !strings.Contains(p, "Facing direction lock") || !strings.Contains(p, "right-side profile") {
		t.Error("스트립 프롬프트에 방향 잠금 섹션이 포함되어야 함")
	}
	spec.Facing = ""
	p = BuildStripPrompt("a knight", StylePresets["pixel"], spec, "", 256)
	if strings.Contains(p, "Facing direction lock") {
		t.Error("방향 미지정 시 방향 잠금 섹션이 없어야 함")
	}
}

func TestMirrorNRGBA(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	set := func(x, y int, r uint8) {
		i := img.PixOffset(x, y)
		img.Pix[i], img.Pix[i+3] = r, 255
	}
	set(0, 0, 10)
	set(1, 0, 20)
	set(2, 0, 30)
	set(0, 1, 40)

	m := MirrorNRGBA(img)
	get := func(x, y int) uint8 { return m.Pix[m.PixOffset(x, y)] }
	if get(2, 0) != 10 || get(1, 0) != 20 || get(0, 0) != 30 || get(2, 1) != 40 {
		t.Error("좌우 반전 결과가 올바르지 않음")
	}
	// 원본 불변
	if img.Pix[img.PixOffset(0, 0)] != 10 {
		t.Error("원본 이미지가 변형됨")
	}
	// 이중 반전 = 원본
	mm := MirrorNRGBA(m)
	for i := range img.Pix {
		if mm.Pix[i] != img.Pix[i] {
			t.Fatal("이중 반전이 원본과 다름")
		}
	}
}
