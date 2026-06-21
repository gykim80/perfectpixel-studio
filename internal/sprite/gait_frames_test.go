package sprite

import (
	"strings"
	"testing"
)

// TestGaitPerFrameShading는 contact 포즈의 SHADING 지시가 올바른지 확인한다.
// step1 contact(RIGHT leads): near leg = RIGHT(front) = lighter.
// step2 contact(LEFT leads): near leg = RIGHT(back) = lighter.
// → RIGHT 다리가 항상 near/lighter, FRONT↔BACK 교대로 lumaLeadFlips 생성.
func TestGaitPerFrameShading(t *testing.T) {
	phases := gaitPhases(6)
	for i, p := range phases {
		desc := phaseDescForFacing(p, "east")
		if p.sub != "contact" {
			continue
		}
		if !strings.Contains(desc, "SHADING") {
			t.Errorf("frame %d contact: missing SHADING instruction in: %s", i+1, desc)
		}
		if !strings.Contains(strings.ToLower(desc), "lighter") {
			t.Errorf("frame %d contact: missing 'lighter' in: %s", i+1, desc)
		}
		// near leg must reference RIGHT (camera-side for eastward walk)
		if !strings.Contains(desc, "RIGHT leg") {
			t.Errorf("frame %d contact: SHADING must reference RIGHT as near/lighter leg, got: %s", i+1, desc)
		}
	}
}

// TestGaitNearLegIsRight는 동쪽 보행에서 항상 RIGHT 다리가 near/lighter 임을 확인한다.
// step1(RIGHT leads): nearLeg = p.lead = RIGHT.
// step2(LEFT leads): nearLeg = p.trail = RIGHT.
func TestGaitNearLegIsRight(t *testing.T) {
	phases := gaitPhases(6)
	for i, p := range phases {
		if p.sub != "contact" {
			continue
		}
		desc := phaseDescForFacing(p, "east")
		if !strings.Contains(desc, "RIGHT leg (near/camera-side RIGHT leg)") && !strings.Contains(desc, "RIGHT leg)") {
			// Should always mention RIGHT as near leg
			if !strings.Contains(desc, "RIGHT leg") {
				t.Errorf("frame %d (lead=%s): near leg should be RIGHT, desc: %s", i+1, p.lead, desc)
			}
		}
	}
}

// TestGaitChoreographyHasNewSections는 새로 추가된 섹션들이 출력에 포함됨을 확인한다.
func TestGaitChoreographyHasNewSections(t *testing.T) {
	spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Facing: "east"}
	out := gaitChoreography(spec)
	required := []string{
		"GROUND LINE LOCK",
		"TAIL & APPENDAGE",
		"FINAL FRAME-BY-FRAME VERIFICATION",
		"SCALE LOCK",
		"HEAD & FACE LOCK",
		"LEG IDENTITY LOCK",
		"SQUINT",
		"25%",
		"PASSING-POSE HEAD TRAP",
		"TAIL TIP",
		"HEAD LOCK",
	}
	for _, r := range required {
		if !strings.Contains(out, r) {
			t.Errorf("east choreography missing required section/phrase: %q", r)
		}
	}
}

// TestGaitStep2AlternationWarning는 step2 contact 프레임에 명시적 교대 경고가 있는지 확인한다.
// "같은 발 반복" 실패의 주원인인 step2에서 RIGHT leg가 여전히 앞에 오는 오류를 방지한다.
func TestGaitStep2AlternationWarning(t *testing.T) {
	phases := gaitPhases(6)
	for i, p := range phases {
		if p.sub != "contact" {
			continue
		}
		desc := phaseDescForFacing(p, "east")
		if p.lead == "RIGHT" {
			// step 1: should say STEP 1 CONTACT
			if !strings.Contains(desc, "STEP 1 CONTACT") {
				t.Errorf("frame %d (step1 contact): missing 'STEP 1 CONTACT' label in: %s", i+1, desc)
			}
		} else {
			// step 2: must have alternation warning
			if !strings.Contains(desc, "STEP 2 CONTACT") {
				t.Errorf("frame %d (step2 contact): missing 'STEP 2 CONTACT' label in: %s", i+1, desc)
			}
			if !strings.Contains(desc, "ALTERNATION REQUIRED") {
				t.Errorf("frame %d (step2 contact): missing ALTERNATION REQUIRED warning in: %s", i+1, desc)
			}
			if !strings.Contains(desc, "BROKEN") {
				t.Errorf("frame %d (step2 contact): missing BROKEN failure warning in: %s", i+1, desc)
			}
		}
	}
}

// TestGaitContactFrameSummary는 east 보행 안무에 contact frame summary가 있는지 확인한다.
func TestGaitContactFrameSummary(t *testing.T) {
	spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Facing: "east"}
	out := gaitChoreography(spec)
	if !strings.Contains(out, "CONTACT FRAME SUMMARY") {
		t.Error("east choreography missing CONTACT FRAME SUMMARY section")
	}
	// Summary should explicitly state frame 1 and frame 4 (half+1 for n=6)
	if !strings.Contains(out, "Frame 1 (step 1 contact)") {
		t.Error("east choreography CONTACT FRAME SUMMARY missing 'Frame 1 (step 1 contact)'")
	}
	if !strings.Contains(out, "Frame 4 (step 2 contact)") {
		t.Error("east choreography CONTACT FRAME SUMMARY missing 'Frame 4 (step 2 contact)'")
	}
	if !strings.Contains(out, "OPPOSITE") {
		t.Error("east choreography CONTACT FRAME SUMMARY missing OPPOSITE keyword")
	}
}

// TestGaitAllDirectionsHaveVerification는 모든 방향에 최종 검증 체크리스트가 있음을 확인한다.
func TestGaitAllDirectionsHaveVerification(t *testing.T) {
	dirs := []string{"east", "west", "south", "north", "south-east", "north-east"}
	for _, dir := range dirs {
		spec := StateSpec{Name: "walk", Frames: 6, FPS: 10, Loop: true, Facing: dir}
		out := gaitChoreography(spec)
		if !strings.Contains(out, "FINAL FRAME-BY-FRAME VERIFICATION") {
			t.Errorf("direction %q: missing FINAL FRAME-BY-FRAME VERIFICATION", dir)
		}
		if !strings.Contains(out, "GROUND LINE LOCK") {
			t.Errorf("direction %q: missing GROUND LINE LOCK", dir)
		}
		if !strings.Contains(out, "TAIL") {
			t.Errorf("direction %q: missing TAIL containment rules", dir)
		}
	}
}
