package sprite

import (
	"strings"
	"testing"
)

// TestGaitChoreography_DirectionAware는 방향별 gait 안무가 올바른 섹션을 포함하는지 확인한다.
func TestGaitChoreography_DirectionAware(t *testing.T) {
	cases := []struct {
		facing      string
		mustHave    string
		mustNotHave string
	}{
		{"east", "STRICT PURE SIDE PROFILE", "STRICT FRONT-FACING"},
		{"", "STRICT PURE SIDE PROFILE", "STRICT FRONT-FACING"},
		{"south", "STRICT FRONT-FACING VIEW", "STRICT PURE SIDE PROFILE"},
		{"north", "STRICT BACK-FACING VIEW", "STRICT PURE SIDE PROFILE"},
		{"south-east", "three-quarter", "STRICT PURE SIDE PROFILE"},
		{"north-east", "three-quarter", "STRICT PURE SIDE PROFILE"},
	}
	for _, tc := range cases {
		spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Facing: tc.facing}
		out := gaitChoreography(spec)
		facing := tc.facing
		if facing == "" {
			facing = "(default)"
		}
		if !strings.Contains(strings.ToLower(out), strings.ToLower(tc.mustHave)) {
			t.Errorf("facing=%q: must contain %q\noutput snippet: %s", facing, tc.mustHave, out[:min(200, len(out))])
		}
		if tc.mustNotHave != "" && strings.Contains(strings.ToLower(out), strings.ToLower(tc.mustNotHave)) {
			t.Errorf("facing=%q: must NOT contain %q", facing, tc.mustNotHave)
		}
	}
}

// TestGaitChoreography_All8Directions는 8방향 모두에서 핵심 규칙이 포함됨을 확인한다.
func TestGaitChoreography_All8Directions(t *testing.T) {
	allDirs := []string{"east", "west", "south", "north", "south-east", "south-west", "north-east", "north-west"}
	commonRequired := []string{
		"GROUND LINE LOCK",
		"SCALE LOCK",
		"FINAL FRAME-BY-FRAME VERIFICATION",
		"TAIL",
		"ground line",
	}
	for _, dir := range allDirs {
		spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Facing: dir}
		out := gaitChoreography(spec)
		if len(out) == 0 {
			t.Errorf("facing=%q: empty output", dir)
			continue
		}
		for _, req := range commonRequired {
			if !strings.Contains(out, req) {
				t.Errorf("facing=%q: missing required phrase %q", dir, req)
			}
		}
	}
}

// TestGaitChoreography_EastSpecificRules는 동쪽(측면) 방향 전용 규칙을 확인한다.
func TestGaitChoreography_EastSpecificRules(t *testing.T) {
	spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Facing: "east"}
	out := gaitChoreography(spec)
	eastRequired := []string{
		"RIGHT leg",       // near leg must reference RIGHT
		"25%",             // minimum luma contrast
		"SQUINT",          // squint test
		"fishhook",        // tail shape rule
		"STRIDE AMPLITUDE", // stride width requirement
		"head does NOT rotate", // head rotation prevention
	}
	for _, req := range eastRequired {
		if !strings.Contains(out, req) {
			t.Errorf("east walk: missing required phrase %q", req)
		}
	}
}

// TestGaitChoreography_SouthSpecificRules는 정면(south) 방향 전용 규칙을 확인한다.
func TestGaitChoreography_SouthSpecificRules(t *testing.T) {
	spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Facing: "south"}
	out := gaitChoreography(spec)
	southRequired := []string{
		"Both eyes",    // front view = both eyes visible
		"HIP SWAY",     // front view hip movement
		"STRIDE AMPLITUDE",
	}
	for _, req := range southRequired {
		if !strings.Contains(out, req) {
			t.Errorf("south walk: missing required phrase %q", req)
		}
	}
}

// TestGaitChoreography_3QuarterRules는 3/4 방향 전용 규칙을 확인한다.
func TestGaitChoreography_3QuarterRules(t *testing.T) {
	diagonals := []string{"south-east", "north-east", "south-west", "north-west"}
	for _, dir := range diagonals {
		spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Facing: dir}
		out := gaitChoreography(spec)
		required := []string{
			"DEPTH STRIDE",  // 3/4 view depth
			"foreground",    // near/foreground leg
			"25%",           // contrast requirement
		}
		for _, req := range required {
			if !strings.Contains(out, req) {
				t.Errorf("3/4 view %q: missing required phrase %q", dir, req)
			}
		}
	}
}

// TestGaitChoreography_FrameCountVariants는 4/6/8프레임 모두에서 올바른 phase 설명이 나오는지 확인한다.
func TestGaitChoreography_FrameCountVariants(t *testing.T) {
	for _, n := range []int{4, 6, 8} {
		spec := StateSpec{Name: "walk", Frames: n, FPS: 10, Loop: true, Facing: "east"}
		out := gaitChoreography(spec)
		if len(out) == 0 {
			t.Errorf("frames=%d: empty output", n)
			continue
		}
		expected := strings.Count(out, "Frame ")
		if expected < n {
			t.Errorf("frames=%d: expected at least %d frame descriptions, got %d", n, n, expected)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
