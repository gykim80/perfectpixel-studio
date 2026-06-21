package sprite

import (
	"fmt"
	"strings"
)

// 지면 보행(좌우 다리가 교대하는 동작) 상태 집합.
// 이들은 "같은 발만 반복" 실패가 잦아 프레임별 다리/팔 안무를 강하게 주입한다.
func IsGaitState(name string) bool {
	switch stripDirectionSuffix(strings.ToLower(strings.TrimSpace(name))) {
	case "walk", "run", "sprint", "carry", "push", "pull":
		return true
	}
	return false
}

// gaitPhase는 한 프레임의 보행 위상입니다.
type gaitPhase struct {
	lead  string // 앞으로 나간 다리: "RIGHT"/"LEFT"
	trail string
	sub   string // "contact" | "passing" | "push"
}

// gaitPhases는 n프레임을 2스텝(오른발 리드 → 왼발 리드) 사이클로 매핑합니다.
// 한 스텝은 contact(앞발 착지) → weight(하중 이동) → passing(뒷발 통과) → push(뒷발 밀기) 순.
func gaitPhases(n int) []gaitPhase {
	if n < 2 {
		n = 2
	}
	out := make([]gaitPhase, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n) * 2.0 // 0..2 (두 스텝)
		step := int(t) % 2                 // 0: 오른발 리드, 1: 왼발 리드
		sub := t - float64(int(t))         // 0..1 (스텝 내 진행)
		lead, trail := "RIGHT", "LEFT"
		if step == 1 {
			lead, trail = "LEFT", "RIGHT"
		}
		// 임계값: contact<0.17, weight<0.30, push≥0.65.
		// n=4: [contact, passing] per step (sub=0, 0.5).
		// n=6: [contact, passing, push] per step (sub=0, 0.333, 0.667).
		// n=8: [contact, weight, passing, push] per step (sub=0, 0.25, 0.5, 0.75) — 각 프레임 고유.
		phase := "push"
		switch {
		case sub < 0.17:
			phase = "contact"
		case sub < 0.30:
			phase = "weight" // 하중 이동: 앞발이 평발이 되고 몸무게가 이동
		case sub < 0.65:
			phase = "passing"
		}
		out[i] = gaitPhase{lead: lead, trail: trail, sub: phase}
	}
	return out
}

// gaitChoreography는 방향별 보행 스트립에 프레임별 다리/팔 위치를 명시하는
// 안무 지시문을 만듭니다. 핵심 목적: contact 포즈에서 앞발을 좌우 교대시켜
// "같은 발만 앞에 두고 위아래로만 흔드는" 실패를 제거한다.
// facing이 비어있거나 "east"이면 순수 측면(사이드) 보행 안무를 주입한다.
func gaitChoreography(spec StateSpec) string {
	if !IsGaitState(spec.Name) {
		return ""
	}
	n := spec.Frames
	if n < 2 {
		return ""
	}
	phases := gaitPhases(n)
	half := n / 2
	var b strings.Builder

	facing := strings.ToLower(strings.TrimSpace(spec.Facing))
	switch facing {
	case "", "east", "west":
		// ── 순수 측면(2D 프로파일) 보행 ──────────────────────────────────────
		b.WriteString("Side-view gait lock (this is a profile locomotion cycle — obey ALL rules exactly):\n\n")
		gaitHeadFaceLock(&b, "east")
		gaitScaleLock(&b, n)
		gaitLegIdentityLock(&b, "east")
		gaitGroundLineLock(&b, n)
		gaitTailContainmentLock(&b, "east")
		gaitAlternationRules(&b, half, n, "east")
	case "south", "north":
		// ── 정면/후면 보행 ──────────────────────────────────────────────────
		view := "front"
		if facing == "north" {
			view = "back"
		}
		fmt.Fprintf(&b, "%s-view gait lock (this is a %s-view locomotion cycle — obey ALL rules exactly):\n\n", strings.ToUpper(view[:1])+view[1:], view)
		gaitHeadFaceLock(&b, facing)
		gaitScaleLock(&b, n)
		gaitLegIdentityLock(&b, facing)
		gaitGroundLineLock(&b, n)
		gaitTailContainmentLock(&b, facing)
		gaitAlternationRules(&b, half, n, facing)
	case "south-east", "north-east", "south-west", "north-west":
		// ── 3/4 대각선 보행 ──────────────────────────────────────────────────
		b.WriteString("Three-quarter view gait lock (diagonal locomotion cycle — obey ALL rules exactly):\n\n")
		gaitHeadFaceLock(&b, facing)
		gaitScaleLock(&b, n)
		gaitLegIdentityLock(&b, facing)
		gaitGroundLineLock(&b, n)
		gaitTailContainmentLock(&b, facing)
		gaitAlternationRules(&b, half, n, facing)
	default:
		// 알 수 없는 방향은 측면 기본값으로 폴백
		b.WriteString("Gait lock (profile locomotion cycle):\n\n")
		gaitHeadFaceLock(&b, "east")
		gaitScaleLock(&b, n)
		gaitLegIdentityLock(&b, "east")
		gaitGroundLineLock(&b, n)
		gaitTailContainmentLock(&b, "east")
		gaitAlternationRules(&b, half, n, "east")
	}

	// Contact summary: show the two contact frames explicitly before the full per-frame list.
	switch facing {
	case "", "east", "west":
		contactFrames := []int{}
		for i, p := range phases {
			if p.sub == "contact" {
				contactFrames = append(contactFrames, i+1)
			}
		}
		if len(contactFrames) >= 2 {
			fmt.Fprintf(&b, "CONTACT FRAME SUMMARY (the two most important frames):\n")
			fmt.Fprintf(&b, "  Frame %d (step 1 contact): RIGHT leg FORWARD + lighter; LEFT leg BACK + darker.\n", contactFrames[0])
			fmt.Fprintf(&b, "  Frame %d (step 2 contact): LEFT leg FORWARD + darker; RIGHT leg BACK + still lighter. ← OPPOSITE of frame %d!\n", contactFrames[1], contactFrames[0])
			fmt.Fprintf(&b, "  These two frames must be leg-position MIRRORS. Same shading (RIGHT=light, LEFT=dark), opposite positions.\n\n")
		}
	}
	b.WriteString("Exact per-frame poses (left to right):\n")
	for i, p := range phases {
		fmt.Fprintf(&b, "- Frame %d — %s: %s\n", i+1, phaseTitle(p), phaseDescForFacing(p, facing))
	}
	b.WriteString("Read the row left to right and confirm the leg lead switches at least once; if two neighbouring contact poses show the same leg forward, the cycle is wrong.\n")
	b.WriteString("\n")
	gaitPerFrameVerification(&b, n, facing)
	return b.String()
}

// gaitHeadFaceLock은 방향별 머리/얼굴 고정 지시문을 씁니다.
func gaitHeadFaceLock(b *strings.Builder, facing string) {
	switch facing {
	case "", "east", "west":
		b.WriteString("HEAD & FACE LOCK — check EACH frame individually before drawing the next:\n")
		b.WriteString("- STRICT PURE SIDE PROFILE throughout the entire row. The camera is fixed at the character's right side and NEVER moves. The character NEVER rotates between frames — not even a slight turn.\n")
		b.WriteString("- CRITICAL: The head does NOT rotate during walking. A common mistake is to turn the head at the passing pose to 'look ahead' — DO NOT do this. Rigid side-profile in every frame, no exceptions.\n")
		b.WriteString("- PASSING-POSE HEAD TRAP (extremely common error): the passing frame (foot swinging under body) tempts artists to rotate the head slightly forward to show 'energy' or 'looking ahead'. This is WRONG. The head stays in the EXACT same side-profile angle as frame 1. If the passing frame head looks different from frame 1, redraw it.\n")
		b.WriteString("- Exactly ONE eye visible in every frame (the near/right-side eye). Count the eyes: if you see TWO eyes in any frame, the head has rotated wrong — STOP and redraw that frame. Zero eyes also means the character is facing the wrong way.\n")
		b.WriteString("- Exactly ONE ear visible (the near/right ear); the far ear is completely hidden behind the head in every frame.\n")
		b.WriteString("- The nose, snout, muzzle, or beak always points toward the RIGHT edge in every frame. Tiny head bob (up/down only) is fine; any horizontal rotation is forbidden.\n\n")
	case "south":
		b.WriteString("HEAD & FACE LOCK (front view) — check each frame:\n")
		b.WriteString("- STRICT FRONT-FACING VIEW throughout the entire row. The camera is directly in front; the character faces the viewer in every frame.\n")
		b.WriteString("- Both eyes visible and symmetric in every frame. The face must NEVER rotate to a profile or 3/4 angle — not even slightly. If you see only one eye, the head has turned wrong.\n")
		b.WriteString("- The body and shoulders remain forward-facing. A slight body lean during the walk is fine, but TWISTING the torso to show a 3/4 view is not.\n")
		b.WriteString("- Natural head bob (up/down 1-2px) and slight side-tilt is fine; horizontal rotation of the face is forbidden.\n\n")
	case "north":
		b.WriteString("HEAD & FACE LOCK (back view) — check each frame:\n")
		b.WriteString("- STRICT BACK-FACING VIEW throughout the entire row. The camera is directly behind; the character faces completely away in every frame.\n")
		b.WriteString("- No face visible — only the back of the head, hair and back of the outfit. If you can see the face or an eye, the head has rotated wrong.\n")
		b.WriteString("- The body and shoulders remain backward-facing; lean and head bob are fine, but twisting to show the face is never allowed.\n\n")
	default: // 3/4 views
		b.WriteString("HEAD & FACE LOCK (three-quarter view) — check each frame:\n")
		fmt.Fprintf(b, "- STRICT THREE-QUARTER VIEW throughout the entire row for the %s direction. The camera angle is fixed — it must not drift toward a full side profile or full front view between frames.\n", facing)
		b.WriteString("- Head orientation must remain IDENTICAL across all frames — the same partial-face angle in every pose. If you notice the head angle changing between frames, it is wrong.\n")
		b.WriteString("- Verify: the same number of features (eyes, nose, chin) are visible in every frame. If ANY frame shows more face than frame 1, the head has rotated — fix it.\n\n")
	}
}

// gaitScaleLock은 방향 무관한 스케일 고정 지시문을 씁니다.
func gaitScaleLock(b *strings.Builder, n int) {
	contactFrame := n/2 + 1 // 두 번째 스텝 첫 프레임 (두 번째 contact)
	b.WriteString("SCALE LOCK — the most common squishing failure:\n")
	b.WriteString("- Measure the character's HEAD HEIGHT (top of ears/hair to chin) in frame 1. This must be IDENTICAL (within 2px) in every other frame.\n")
	b.WriteString("- Measure the character's TOTAL HEIGHT (top of head to bottom of feet) in frame 1. Same rule: must not change.\n")
	b.WriteString("- Measure the TORSO WIDTH at shoulder level in frame 1. Must stay within 3px in every frame. A narrowing torso in wide-stride poses = scale failure.\n")
	fmt.Fprintf(b, "- THE SQUISHING SYMPTOM: if frame 1 shows a tall figure but frame %d (wide-stride contact) shows a shorter, slightly squashed figure — that is scale failure. Every pose should look like the SAME PERSON with only the leg position changed.\n", contactFrame)
	b.WriteString("- Wide-legged contact poses tempt the artist to shrink the whole figure to fit splayed feet — RESIST this. Widen the stride ONLY by moving the feet further apart along the ground line. The torso, head, and TOTAL HEIGHT stay exactly the same. If the feet feel too spread for the cell, REDUCE the stride angle — never shrink the figure.\n")
	b.WriteString("- DRAW ORDER: draw the TORSO and HEAD first (matching frame 1 scale exactly), THEN attach the legs and feet at the correct stride width. Never draw the feet first and then squeeze the body to fit.\n")
	fmt.Fprintf(b, "- If you notice the head dropping lower between frame 1 and frame %d, that is scale failure. Raise the head back to the same height as frame 1 before continuing.\n\n", contactFrame)
}

// gaitLegIdentityLock은 방향별 다리 정체성 음영 지시문을 씁니다.
func gaitLegIdentityLock(b *strings.Builder, facing string) {
	switch facing {
	case "", "east", "west":
		b.WriteString("LEG IDENTITY LOCK — shading is the visual proof of correct alternation:\n")
		b.WriteString("- The NEAR leg (camera-side = the character's RIGHT leg for a rightward walk) must be drawn NOTICEABLY LIGHTER than the FAR leg (occluded = character's LEFT leg).\n")
		b.WriteString("- Minimum contrast: the far leg must be AT LEAST 25-30% darker (≥60 luma units difference on a 0-255 scale). This difference must be OBVIOUS even at thumbnail size.\n")
		b.WriteString("- SQUINT TEST: blur your vision or reduce the image to 32px wide. Can you immediately identify 'one bright leg' and 'one dark leg'? If not, the contrast is insufficient — make the far leg darker.\n")
		b.WriteString("- ALTERNATION PROOF (the near/RIGHT leg alternates between FRONT and BACK):\n")
		b.WriteString("  • Step 1 contact (RIGHT foot FORWARD): the LIGHTER leg (RIGHT) is in FRONT; the darker leg (LEFT) is behind pushing off.\n")
		b.WriteString("  • Step 2 contact (LEFT foot FORWARD): the LIGHTER leg (RIGHT) is now in BACK; the darker leg (LEFT) is in front.\n")
		b.WriteString("  • The LIGHTER leg MUST visibly switch from FRONT-position to BACK-position between the two contact poses. If the lighter leg is always in front OR always in back, the alternation is WRONG.\n")
		b.WriteString("- CAPED/ROBED characters: legs and feet MUST emerge clearly below the garment hem so both feet are individually visible. Raise the hem or shorten the robe so feet are unambiguous.\n\n")
	case "south", "north":
		b.WriteString("LEG IDENTITY LOCK (front/back view):\n")
		b.WriteString("- Both legs are fully visible. Left and right legs alternate clearly — the legs must NEVER be in identical positions in two consecutive frames.\n")
		b.WriteString("- Draw the swing leg slightly LIGHTER (it's lifting, catching light from above) and the planted leg slightly DARKER (grounded). This shading SWAPS each step.\n")
		b.WriteString("- Minimum contrast: 20-25% brightness difference between planted and swing leg. The difference should be visible at thumbnail size.\n")
		b.WriteString("- The leg that is RAISED/swinging forward should be clearly lighter; the leg that is PLANTED should be clearly darker.\n")
		b.WriteString("- Foot ground contact: the planted foot is flat on the ground line; the swing foot lifts VISIBLY above the ground between the two contact poses.\n")
		b.WriteString("- CAPED/ROBED characters: legs and feet MUST emerge clearly below the garment hem.\n\n")
	default: // 3/4 views
		b.WriteString("LEG IDENTITY LOCK (three-quarter view):\n")
		b.WriteString("- In 3/4 view, the NEAR (foreground) leg is prominent and fully visible; the FAR (background) leg is partially occluded behind the near leg.\n")
		b.WriteString("- The near leg must be AT LEAST 25% lighter than the far leg (receiving more direct light, less depth shadow).\n")
		b.WriteString("- ALTERNATION PROOF: the near leg alternates between FORWARD and BACKWARD positions across the two steps. Step 1: near leg reaches forward; Step 2: near leg pushes off from behind.\n")
		b.WriteString("- The LIGHTER leg must visibly switch from FRONT-position in step 1 to BACK-position in step 2. If the lighter leg is always in the same position, alternation is WRONG.\n")
		b.WriteString("- Foot ground contact: landing foot on ground line; push-off foot 1-3 pixels above.\n")
		b.WriteString("- CAPED/ROBED characters: legs and feet MUST emerge clearly below the garment hem.\n\n")
	}
}

// gaitGroundLineLock은 발 지면선 고정 지시문을 씁니다.
func gaitGroundLineLock(b *strings.Builder, n int) {
	b.WriteString("GROUND LINE LOCK — feet must stay anchored:\n")
	fmt.Fprintf(b, "- All %d frames share ONE fixed horizontal ground line. The landing/planted feet must all touch this same Y coordinate (within ±3px).\n", n)
	b.WriteString("- The swing (passing) foot lifts AT MOST 6-8px above this ground line — not 20px, not 15px. A floating foot that rises far above the ground looks broken.\n")
	b.WriteString("- The CHARACTER bobs slightly: torso rises 1-2px at push-off, drops 1-2px at passing. The ground line itself does NOT move — only the torso bobs.\n")
	b.WriteString("- FLOATING SYMPTOM: if one frame shows the character drifting upward (all feet far above the normal ground level), that frame has the ground line wrong. Anchor the feet back down.\n\n")
}

// gaitTailContainmentLock은 꼬리(와 기타 부속지) 열 경계 지시문을 씁니다.
func gaitTailContainmentLock(b *strings.Builder, facing string) {
	switch facing {
	case "", "east", "west":
		b.WriteString("TAIL & APPENDAGE CONTAINMENT (applies to tailed characters: cat, fox, dog, wolf, lizard, dragon, etc.):\n")
		b.WriteString("- The tail exits from the character's back (left side of the body for a rightward walk). It may sway during the gait — but it MUST curve back toward the body before reaching the column's left edge.\n")
		b.WriteString("- Tail shape rule: think of the tail as a fishhook or reverse question-mark. The SHAFT points left (away from body), but the TIP hooks rightward back toward the character's spine or legs. The tip must point RIGHT or DOWNWARD — never further LEFT.\n")
		b.WriteString("- Maximum leftward reach: the tail may extend left by AT MOST 15% of the column width before it MUST curve back. A tail that runs straight left and hits the column boundary is WRONG — shorten it or increase the curl.\n")
		b.WriteString("- A long tail (fox, dragon): start the curl EARLIER. The base exits left, the middle turns, the tip points right. If the full length would exit the column, curl the tip more sharply.\n")
		b.WriteString("- ALL other appendages (ear tips, wing feathers, weapon tips, scarf ends, hair strands): they must also stay INSIDE the pose column. Curve or shorten any appendage that would cross the column edge.\n\n")
	case "south", "north":
		b.WriteString("TAIL & APPENDAGE CONTAINMENT (front/back view):\n")
		b.WriteString("- The tail is seen from the front/back; it typically hangs below the body or rises behind it. It may sway left/right with the walk gait.\n")
		b.WriteString("- The tail must stay within the pose column. If it sways left, it must curve back toward center before reaching the column left edge, and vice versa for rightward swings.\n")
		b.WriteString("- All appendages (ears, wings, weapon tips, hair) must stay inside the column.\n\n")
	default: // 3/4 views
		b.WriteString("TAIL & APPENDAGE CONTAINMENT (three-quarter view):\n")
		b.WriteString("- In 3/4 view, the tail is partially visible behind the body, extending toward one side. It may sway gently with the gait — but the TIP must ALWAYS curve back toward the character's spine before reaching the column edge.\n")
		b.WriteString("- Tail shape rule: the shaft exits the body, but the tip hooks inward (toward the body center), like a fishhook curling back. The tail tip must NEVER point away from the character toward the column edge.\n")
		b.WriteString("- Maximum sideward reach: the tail shaft may extend sideways by AT MOST 15% of the column width before the tip must hook back. A tail that runs straight to the edge is WRONG — curl it sooner.\n")
		b.WriteString("- All appendages (ear tips, wing feathers, weapon tips, scarf ends, hair strands) must stay INSIDE the pose column — curve or shorten any appendage that would cross the edge.\n\n")
	}
}

// gaitPerFrameVerification은 모든 프레임 제출 전 최종 검증 체크리스트를 씁니다.
func gaitPerFrameVerification(b *strings.Builder, n int, facing string) {
	b.WriteString("FINAL FRAME-BY-FRAME VERIFICATION (check ALL frames before finishing):\n")
	b.WriteString("Go through each frame and verify ALL of the following:\n")
	switch facing {
	case "", "east", "west":
		b.WriteString("1. HEAD HEIGHT: same as frame 1 within 2px. If the head is lower → scale failure, fix it.\n")
		b.WriteString("2. SIDE PROFILE: exactly 1 eye visible, nose pointing right. If 2 eyes → head rotated, fix it. If 0 eyes → character is facing wrong way.\n")
		b.WriteString("3. TORSO WIDTH: same as frame 1 within 3px. Narrower torso = squishing, fix it.\n")
		b.WriteString("4. FEET GROUNDED: both feet within ±3px of the ground line (swing foot ≤8px above).\n")
		b.WriteString("5. LEG SHADING: near/RIGHT leg is clearly LIGHTER than far/LEFT leg in EVERY frame. Squint test: can you see 'one bright leg' and 'one dark leg'? If not, increase contrast to at least 25% brightness difference.\n")
		b.WriteString("6. ALTERNATION: compare frame 1 and frame " + fmt.Sprintf("%d", n/2+1) + " side by side. Frame 1: RIGHT leg (lighter) is FORWARD. Frame " + fmt.Sprintf("%d", n/2+1) + ": RIGHT leg (still lighter) is BACKWARD, LEFT leg (darker) is now FORWARD. These two frames MUST be leg-position mirrors. If RIGHT is still in front in frame " + fmt.Sprintf("%d", n/2+1) + ", the alternation FAILED — redraw frame " + fmt.Sprintf("%d", n/2+1) + ".\n")
	case "south":
		b.WriteString("1. HEAD HEIGHT: same as frame 1 within 2px.\n")
		b.WriteString("2. FRONT-FACING: both eyes visible and symmetric. If only one eye → head rotated, fix it.\n")
		b.WriteString("3. TORSO WIDTH: same as frame 1.\n")
		b.WriteString("4. FEET GROUNDED: both feet within ±3px of ground line in contact poses.\n")
		b.WriteString("5. LEG SHADING: swing leg is lighter than planted leg. This SWAPS between steps.\n")
		b.WriteString("6. ALTERNATION: in the two contact poses, clearly different legs are leading.\n")
	case "north":
		b.WriteString("1. HEAD HEIGHT: same as frame 1 within 2px.\n")
		b.WriteString("2. BACK-FACING: no face visible, only the back of head/hair.\n")
		b.WriteString("3. TORSO WIDTH: same as frame 1.\n")
		b.WriteString("4. FEET GROUNDED: both feet within ±3px of ground line.\n")
		b.WriteString("5. ALTERNATION: in the two contact poses, clearly different legs are leading.\n")
	default:
		b.WriteString("1. HEAD HEIGHT: same as frame 1 within 2px.\n")
		b.WriteString("2. VIEW ANGLE: three-quarter angle is the same in every frame — not drifting toward profile or front.\n")
		b.WriteString("3. TORSO WIDTH: same as frame 1.\n")
		b.WriteString("4. FEET GROUNDED: both feet within ±3px of ground line.\n")
		b.WriteString("5. LEG SHADING: near (foreground) leg is lighter than far leg. This shading ALTERNATES between steps.\n")
		b.WriteString("6. ALTERNATION: the lighter (near) leg switches position between the two contact poses.\n")
	}
	b.WriteString("7. TAIL TIP: for tailed characters — in EVERY frame, trace the tail from base to tip. The tip must point RIGHT or DOWNWARD, NOT leftward. If the tail tip exits the left side of the pose column, it is WRONG — curl the tip back toward the body.\n")
	b.WriteString("8. APPENDAGES: no ear, wing feather, hair strand, scarf end, or weapon tip crosses the column boundary in any frame.\n")
	fmt.Fprintf(b, "If ANY check fails on ANY of the %d frames, FIX THAT FRAME before the final render.\n", n)
}

// gaitAlternationRules는 방향별 교대 보행 규칙을 씁니다.
func gaitAlternationRules(b *strings.Builder, half, n int, facing string) {
	switch facing {
	case "", "east", "west":
		b.WriteString("Gait alternation rules:\n")
		b.WriteString("- Pure 2D side profile, the character travels toward the RIGHT edge. Both legs must read clearly as a FRONT leg and a BACK leg in every contact pose; the body never turns toward the viewer.\n")
		fmt.Fprintf(b, "- The row is TWO separate steps. Frames 1..%d = FIRST step (RIGHT leg leads FORWARD), frames %d..%d = SECOND step (LEFT leg leads FORWARD). Each step opens wide at contact, closes at passing. The legs OPEN–CLOSE TWICE across the whole row.\n", half, half+1, n)
		fmt.Fprintf(b, "- CRITICAL alternation anchor: frame %d must be the LEFT–RIGHT MIRROR of frame 1 in leg position — the opposite foot is forward and the lighter leg has switched sides.\n", half+1)
		fmt.Fprintf(b, "- STRIDE OSCILLATION PROFILE (mandatory): the foot spread across the %d frames must follow a WIDE→narrow→WIDE→narrow (double-valley) pattern. Frame 1 = WIDE contact, frame %d = narrow passing, frame %d = WIDE contact again, final frame = narrow/push. If the stride stays narrow or only widens ONCE, the walk is incomplete.\n", n, n/4+1, half+1)
		b.WriteString("- STRIDE AMPLITUDE: at contact poses, the front foot should reach 30-40% of body width AHEAD of the hip center, and the back foot 30-40% BEHIND. A walk where the feet barely leave center is a shuffle — spread them.\n")
		b.WriteString("- The NEAR (lighter/RIGHT) leg sweeps: FRONT in step 1 contact → BACK in step 2 contact. This is the visual proof of correct alternation.\n")
		b.WriteString("- Arms swing OPPOSITE to the legs: right leg forward → left arm forward.\n")
		b.WriteString("- Body bob: torso LOWEST at passing pose (1-2px below frame 1), HIGHEST at push-off (1-2px above). This bob is subtle — never more than 3px total.\n")
		fmt.Fprintf(b, "- Garments (robes, skirts, coats): legs and feet emerge below the hem clearly in all %d frames; the hem lifts with each step.\n", n)
		b.WriteString("- Weapons (axe, sword, staff, shield): position so BOTH LEGS are clearly visible in every frame. Leg alternation takes priority over weapon drama.\n\n")
	case "south", "north":
		b.WriteString("Gait alternation rules (front/back view):\n")
		b.WriteString("- Both legs are symmetrically visible. The stride is visible as a HORIZONTAL SPREAD: the forward foot reaches ahead of the centerline, the back foot pushes off behind.\n")
		fmt.Fprintf(b, "- Frames 1..%d = FIRST step (right leg swings forward/ahead), frames %d..%d = SECOND step (left leg swings forward/ahead).\n", half, half+1, n)
		fmt.Fprintf(b, "- Frame %d must clearly show the OPPOSITE leg raised/forward compared to frame 1. The two contact poses should look like left-right mirrors of each other.\n", half+1)
		b.WriteString("- HIP SWAY: hips shift slightly left when the right leg leads, slightly right when the left leg leads. This natural hip sway (1-3px lateral movement) makes the front-view walk look alive.\n")
		b.WriteString("- STRIDE AMPLITUDE: the forward foot reaches at least 20-30% of hip width ahead of the centerline. A walk where the feet stay directly under the hips looks like a march.\n")
		b.WriteString("- Arms swing OPPOSITE: right leg forward → left arm swings forward.\n")
		b.WriteString("- Body bob: slight dip at passing (feet together, 1-2px below), rise at push-off (1-2px above).\n\n")
	default: // 3/4 views
		b.WriteString("Gait alternation rules (three-quarter view):\n")
		fmt.Fprintf(b, "- The row is TWO separate steps. Frames 1..%d = FIRST step, frames %d..%d = SECOND step with the OPPOSITE foot leading.\n", half, half+1, n)
		fmt.Fprintf(b, "- Frame %d must clearly show the opposite foot forward from frame 1 — these two contact poses must look like near-mirror images of each other.\n", half+1)
		b.WriteString("- DEPTH STRIDE: in 3/4 view, the forward leg reaches toward the VIEWER (downward-forward in perspective) and the back leg pushes AWAY (upward-back). The near (foreground) leg appears slightly LARGER due to perspective — use this to reinforce the depth illusion.\n")
		b.WriteString("- Near leg (foreground/lighter) sweeps FORWARD in step 1, BACKWARD in step 2 — and vice versa for the far leg. The lighter leg MUST switch position between the two contact poses.\n")
		b.WriteString("- STRIDE AMPLITUDE: the forward leg in 3/4 view should reach noticeably toward the camera. A stride where legs barely move looks flat — open the legs into a clear spread.\n")
		b.WriteString("- Arms swing OPPOSITE to legs. Body bobs: low at passing, high at push-off.\n")
		b.WriteString("- Keep the torso centered in its column — forward lean during the stride is fine, but the hips must not drift out of the pose column.\n\n")
	}
}

func phaseTitle(p gaitPhase) string {
	switch p.sub {
	case "contact":
		return p.lead + "-foot CONTACT (heel strike)"
	case "weight":
		return p.lead + "-foot WEIGHT TRANSFER (foot flat)"
	case "passing":
		return "PASSING (" + p.lead + " bears weight)"
	default:
		return "PUSH-OFF (" + p.trail + " drives)"
	}
}

func phaseDesc(p gaitPhase) string {
	return phaseDescForFacing(p, "east")
}

func phaseDescForFacing(p gaitPhase, facing string) string {
	leadArm := "left"
	if p.lead == "LEFT" {
		leadArm = "right"
	}
	switch facing {
	case "south", "north":
		// 정면/후면: 좌우 다리가 대칭으로 보임
		switch p.sub {
		case "contact":
			return fmt.Sprintf("the %s leg swings FORWARD — heel-strikes; the %s leg pushes OFF behind with heel lifting — legs at WIDEST spread. Hips shift slightly toward the %s side. The %s arm swings FORWARD, other arm BACK. Body at mid-height.",
				p.lead, p.trail, p.lead, leadArm)
		case "weight":
			return fmt.Sprintf("the %s foot is now FLAT on the ground (full contact); body shifts forward and weight transfers onto the %s leg; the %s leg begins to swing forward with knee bending; stride slightly narrower than contact; body still at mid-height; arms continuing their swing.",
				p.lead, p.lead, p.trail)
		case "passing":
			return fmt.Sprintf("the %s leg swings forward with BENT KNEE (passes close under the body near the planted %s leg); feet at their closest (near centerline); body at LOWEST (1-2px below contact); hips centered; arms near the body. HEAD LOCK: both eyes visible and symmetric, face directly forward — the face must NOT rotate to a profile angle here.",
				p.trail, p.lead)
		default:
			return fmt.Sprintf("the %s leg PUSHES OFF (heel lifting, toe pressing down) while the %s leg extends AHEAD ready to land; body rising toward its HIGHEST; hips shifting toward the %s side; arms crossing at mid-swing.",
				p.trail, p.lead, p.lead)
		}
	case "south-east", "south-west", "north-east", "north-west":
		// 3/4 사각: 한쪽 다리가 앞쪽으로 더 크게 보임
		switch p.sub {
		case "contact":
			if p.lead == "RIGHT" {
				return fmt.Sprintf("STEP 1 CONTACT (3/4 view) — the RIGHT leg reaches FORWARD toward the viewer (slightly LARGER due to perspective, heel landing); the LEFT leg is BACK and partially OCCLUDED (slightly smaller, heel lifting in push-off) — widest leg spread. The %s arm swings forward. Hips angled toward the right side. Body mid-height. SHADING: near (foreground) leg is LIGHTER than far (background) leg — 25%%+ brightness difference.",
					leadArm)
			}
			return fmt.Sprintf("STEP 2 CONTACT (3/4 view) — ⚠ ALTERNATION: the LEFT leg now reaches FORWARD toward the viewer; the RIGHT leg is BACK and partially OCCLUDED (pushing off). The %s arm swings forward. Body mid-height. SHADING: near (foreground) leg is LIGHTER. ⚠ CHECK: compare to frame 1 — leg positions must be SWAPPED (opposite foot forward). If the same leg is still forward, alternation FAILED.",
				leadArm)
		case "weight":
			return fmt.Sprintf("the %s foot is FLAT on the ground (full contact, toe down); weight transfers forward; the %s leg begins lifting from behind, knee starting to bend; stride slightly narrower than contact; body at mid-height; arms continuing their swing.",
				p.lead, p.trail)
		case "passing":
			return fmt.Sprintf("the %s leg swings forward with BENT KNEE (passes close under the body, swinging toward the viewer); the planted %s leg is flat on the ground; feet near each other in center; body at LOWEST (1-2px below contact); arms near the body. HEAD LOCK: three-quarter angle is UNCHANGED — same partial-face angle as frame 1, same number of visible features.",
				p.trail, p.lead)
		default:
			return fmt.Sprintf("the %s leg PUSHES OFF (foreshortened/smaller as it goes behind) while the %s leg swings TOWARD the viewer; body rising to its HIGHEST; legs transitioning — near leg about to land; arms mid-swap.",
				p.trail, p.lead)
		}
	default: // east/west: 순수 측면
		// near(light) leg 정체성: 오른쪽 방향 보행에서 near = 캐릭터 RIGHT 다리.
		// 카메라가 캐릭터 오른쪽에 있으므로 RIGHT 다리가 항상 카메라에 가까운(near) 다리.
		// step1(RIGHT leads): RIGHT 다리가 FRONT(앞) → near(light)=RIGHT=FRONT.
		// step2(LEFT leads): RIGHT 다리가 BACK(뒤) → near(light)=RIGHT=BACK.
		// 따라서 near/light 다리는 step1에서 FRONT, step2에서 BACK으로 교대한다.
		nearLeg := p.lead // step1: lead=RIGHT → nearLeg=RIGHT(front, lighter)
		if p.lead == "LEFT" {
			nearLeg = p.trail // step2: trail=RIGHT → nearLeg=RIGHT(back, lighter)
		}
		// nearLeg은 항상 RIGHT (east 보행 고정). farLeg은 항상 LEFT.
		// 앞/뒤 발 정보는 p.lead/p.trail로 표현.
		_ = nearLeg // nearLeg 선언 유지, 실제 문자열은 "RIGHT"/"LEFT" 고정
		switch p.sub {
		case "contact":
			if p.lead == "RIGHT" {
				// Step 1: RIGHT leg forward, RIGHT (near) leg = lighter and in FRONT.
				return fmt.Sprintf("STEP 1 CONTACT — the RIGHT leg is stretched FAR FORWARD (heel strikes, toes up); the LEFT leg is stretched BACK (toe down, heel LIFTING in push-off) — legs at WIDEST spread. RIGHT arm swings BACK, LEFT arm swings FORWARD. Body mid-height. SHADING: RIGHT leg (near/camera-side) is LIGHTER — 25%%+ brighter than the far LEFT leg (clearly visible at thumbnail size). LEFT leg is at least 25%% DARKER.",
					)
			}
			// Step 2: LEFT leg forward, RIGHT (near) leg = lighter but now in BACK.
			return "STEP 2 CONTACT — ⚠ ALTERNATION REQUIRED: the LEFT leg is now stretched FAR FORWARD (heel strikes, toes up); the RIGHT leg is now stretched BACK (toe down, heel LIFTING in push-off) — legs at WIDEST spread. LEFT arm swings BACK, RIGHT arm swings FORWARD. Body mid-height. SHADING: the RIGHT leg (near/camera-side) is STILL LIGHTER (25%+ brighter, same contrast requirement as step 1) — SAME shading as step 1, but OPPOSITE positions. In step 1 the RIGHT (lighter) leg was in FRONT; NOW the RIGHT (lighter) leg is in BACK and the DARK LEFT leg is in FRONT. ⚠ CRITICAL CHECK: place this frame next to frame 1 — the leg positions must be LEFT-RIGHT MIRRORS of each other. If the RIGHT leg is still in front here, the alternation is BROKEN and the walk is wrong. The KEY visual: dark LEFT leg heel-striking forward, light RIGHT leg pushing off behind."
		case "weight":
			return fmt.Sprintf("the %s foot is now FLAT on the ground (heel down, toe down); body weight fully over the %s leg; the %s leg begins swinging forward with a bending knee; stride slightly narrower than contact (feet less spread); body at mid-height; arms continuing swing. SHADING: RIGHT leg LIGHTER, LEFT leg DARKER (same as frame 1).",
				p.lead, p.lead, p.trail)
		case "passing":
			return fmt.Sprintf("the %s leg swings FORWARD with a BENT KNEE (knee is ahead of the hip as the foot lifts and swings through under the body); the planted %s leg is flat on the ground supporting the weight; feet pass CLOSE together (near centerline); body at its LOWEST (1-2px below contact height); arms swing toward center. HEAD LOCK: the head stays in STRICT side-profile — ONE eye, nose pointing RIGHT — exactly the same angle as frame 1. Do NOT turn the head forward or show two eyes here.",
				p.trail, p.lead)
		default:
			return fmt.Sprintf("the %s leg PUSHES OFF behind (heel rising, toe still pressing down) as the %s leg swings AHEAD and extends forward ready to heel-strike; body at its HIGHEST point (1-2px above contact height); arms crossing at mid-swing.",
				p.trail, p.lead)
		}
	}
}
