package sprite

import (
	"fmt"
	"strings"
)

// StylePresets는 선택 가능한 스타일 계약 모음입니다.
var StylePresets = map[string]string{
	"pixel": "true low-resolution pixel-art game sprite, like a 32-64px sprite enlarged on the canvas, " +
		"chunky readable silhouette, clean dark 1px outline, visible square pixel blocks, " +
		"grid-aligned hard pixel edges, limited shared palette, solid tone clusters, " +
		"flat color shading with at most one highlight step and one shadow step, " +
		"simple readable face and clearly separated limbs. " +
		"Never use painterly rendering, smooth gradients, airbrush shading, glossy lighting, " +
		"anti-aliased fine detail, high-definition pixel art, fine-grained pixel art, anime illustration, concept art, or 3D rendering.",
	"chibi": "cute chibi game sprite with oversized head and small body, " +
		"bold dark outline, flat bright colors, minimal shading, large expressive eyes, " +
		"clean cartoon shapes readable at small size. " +
		"Never use realistic proportions, gradients, or painterly detail.",
	"cartoon": "clean 2D cartoon game sprite, bold uniform outline, flat vivid colors, " +
		"simple two-tone cel shading, smooth rounded shapes, expressive but simple face. " +
		"Never use pixelation, gradients, photo textures, or 3D rendering.",
	"retro16": "16-bit retro console era game sprite, restrained palette of 16-24 colors, " +
		"dark outline, dithering only where needed, compact proportions, " +
		"crisp hard pixel edges like a classic arcade fighter sprite. " +
		"Never use modern smooth shading or high-resolution detail.",
}

// keyColorPhrase는 키잉 배경 묘사 문구입니다 (매팅이 분리하는 색).
const keyColorPhrase = "pure keying magenta (#FF00FF), perfectly uniform edge to edge"

// ResolveStyle은 프리셋 키 또는 커스텀 스타일 텍스트를 스타일 계약으로 변환합니다.
func ResolveStyle(presetKey, custom string) string {
	if strings.TrimSpace(custom) != "" {
		return strings.TrimSpace(custom)
	}
	if s, ok := StylePresets[presetKey]; ok {
		return s
	}
	return StylePresets["pixel"]
}

// canvasContract는 키잉 캔버스 규칙을 반환합니다 (매팅 단계가 의존하는 핵심 계약).
func canvasContract() string {
	var b strings.Builder
	b.WriteString("Keying canvas (the renderer mattes this away — obey exactly):\n")
	b.WriteString("- Fill the ENTIRE background, edge to edge, with " + keyColorPhrase + " — a single flat color touching all four image borders. No gradient, texture, scenery, floor, panel, frame, or border of any kind.\n")
	b.WriteString("- The subject must avoid magenta, pink and purple entirely — clothing, props, highlights and effects included — so the keyer never eats part of the character.\n")
	b.WriteString("- Drop every shadow and contact patch; the ground is implied, never painted.\n")
	return b.String()
}

// spriteDesignContract는 기본 픽셀 스타일에서 요구하는 게임 스프라이트 구조를 고정합니다.
func spriteDesignContract() string {
	var b strings.Builder
	b.WriteString("Game-sprite design contract:\n")
	b.WriteString("- Interpret the subject as a game-ready character sprite, not an illustration, poster, sticker, mascot logo, or concept-art render.\n")
	b.WriteString("- Preserve the subject's identity through a strong silhouette, hairstyle, outfit shapes, accessories, weapon or signature prop, and dominant color blocks.\n")
	b.WriteString("- Simplify anatomy into readable sprite shapes: compact torso, clear head shape, simple arms and legs, minimal joint detail, no tiny anatomy rendering.\n")
	b.WriteString("- Hair, clothing layers, capes, hats, weapons and accessories should read as distinct hard-edged pixel shapes, not detailed painted textures.\n")
	b.WriteString("- Keep the face simple at sprite scale: readable eyes and mouth, minimal facial detail, no realistic nose or painted skin texture.\n")
	return b.String()
}

// lowResPixelContract는 모델이 HD 일러스트로 도망가지 않게 렌더링 해상도 감각을 고정합니다.
func lowResPixelContract() string {
	var b strings.Builder
	b.WriteString("Pixel rendering contract:\n")
	b.WriteString("- The image must look like a 32-64px game sprite enlarged to the canvas, not newly painted at high resolution.\n")
	b.WriteString("- Use chunky square pixel blocks, clean 1px outline, solid tone clusters, limited palette, minimal two-step flat shading.\n")
	b.WriteString("- No dithering, no smooth gradients, no soft shadow, no blur, no airbrush, no texture, no fine hair strands, no tiny jewelry detail that would vanish at 64px.\n")
	b.WriteString("- Every important shape must remain readable when shrunk to a thumbnail: silhouette first, details second.\n")
	return b.String()
}

func pixelStyleContracts(style string) string {
	s := strings.ToLower(style)
	if !strings.Contains(s, "pixel") && !strings.Contains(s, "sprite") && !strings.Contains(s, "mmorpg") {
		return ""
	}
	return spriteDesignContract() + "\n" + lowResPixelContract()
}

// rejectClause는 추출을 방해하는 요소를 거부하는 간결한 계약입니다.
func rejectClause() string {
	var b strings.Builder
	b.WriteString("Reject (these break automatic extraction):\n")
	b.WriteString("- ANY frame, border, or decoration around the image or around a pose: no film strip, no sprocket holes or perforations, no photo/polaroid frame, no panel dividers, no outline box, no vignette. The background reaches every edge unbroken.\n")
	b.WriteString("- Motion garnish — streaks, speed lines, blur, after-images, arcs, swooshes, trails.\n")
	b.WriteString("- Free-floating bits — sparkles, stars, dust, smoke puffs, icons, symbols, or any mark not fused to the body.\n")
	b.WriteString("- Text, numbers, captions, grids, rulers, speech or thought bubbles, UI, watermarks.\n")
	b.WriteString("- Any pose that is clipped by the edge, or whose pixels bridge into the neighbouring pose.\n")
	return b.String()
}

// BuildCharacterPrompt는 텍스트 설명 → 베이스 캐릭터 이미지 생성 프롬프트를 만듭니다.
func BuildCharacterPrompt(description, style string) string {
	var b strings.Builder
	b.WriteString("Produce one complete game-character reference sprite in a relaxed player-avatar standing pose.\n\n")
	fmt.Fprintf(&b, "Subject: %s.\n\n", strings.TrimSpace(description))
	b.WriteString("Feature audit before drawing (do this internally, then render): identify and preserve the subject's hairstyle, hair color, eye color, outfit layers, accessories, weapon or signature prop, symbolic motifs, and dominant colors.\n\n")
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)
	if extra := pixelStyleContracts(style); extra != "" {
		b.WriteString(extra)
		b.WriteString("\n")
	}
	b.WriteString("Framing:\n")
	b.WriteString("- A single figure, head to feet, vertically centered, occupying about three quarters of the canvas height with generous breathing room on every side.\n")
	b.WriteString("- Idle standing sprite pose: feet level, weight balanced, arms relaxed but readable.\n")
	b.WriteString("- Almost flat 2D game-sprite view; avoid dramatic perspective, foreshortening, cinematic camera angles, and illustration-style posing.\n")
	b.WriteString("- One continuous silhouette — nothing detached, no trailing accessories or particles.\n\n")
	b.WriteString(canvasContract())
	b.WriteString("\n")
	b.WriteString(rejectClause())
	return b.String()
}

// BuildStripPrompt는 상태별 가로 스트립 생성 프롬프트를 만듭니다.
func BuildStripPrompt(description, style string, spec StateSpec, feedback string) string {
	var b strings.Builder
	n := spec.Frames

	fmt.Fprintf(&b, "Draw a single horizontal row of exactly %d game-sprite poses of one character for the \"%s\" animation, ordered left to right. This is raw sprite art, not a photo or a film — draw only the character poses on a flat background.\n\n", n, spec.Name)

	b.WriteString("Subject lock (top priority):\n")
	b.WriteString("- The attached image is the canonical character. Match it exactly across every pose: face, hairstyle, build, outfit, accessories.\n")
	b.WriteString("- Palette is binding. Re-sample each region's hue, saturation and value from the reference — skin, hair, every garment, every piece of gear. Do not re-tint, re-light, brighten, darken, or substitute a similar shade.\n")
	b.WriteString("- Hold one fixed camera and facing. The figure never rotates, mirrors, ages, or restyles between poses — only the body moves.\n\n")

	if d := strings.TrimSpace(description); d != "" {
		fmt.Fprintf(&b, "Subject notes: %s.\n\n", d)
	}
	fmt.Fprintf(&b, "Render contract (obey strictly): %s\n\n", style)
	if extra := pixelStyleContracts(style); extra != "" {
		b.WriteString(extra)
		b.WriteString("\n")
	}

	if sec := FacingPromptSection(spec.Facing); sec != "" {
		b.WriteString(sec)
		b.WriteString("\n")
	}

	action := strings.TrimSpace(spec.Action)
	if action == "" {
		action = spec.Name
	}
	fmt.Fprintf(&b, "Movement: %s.\n", action)
	if hint := MotionHint(spec.Name); hint != "" {
		fmt.Fprintf(&b, "Choreography: %s\n", hint)
	}
	fmt.Fprintf(&b, "Treat the %d poses as evenly timed beats of one continuous motion — pose k is phase k of %d, and neighbours read as smooth in-betweens, never unrelated stances.\n", n, n)
	if spec.Loop {
		b.WriteString("It loops: the final pose must hand off cleanly into the first.\n\n")
	} else {
		b.WriteString("It plays once: give it a clear start, peak, and settle.\n\n")
	}

	b.WriteString("Row layout:\n")
	fmt.Fprintf(&b, "- Place exactly %d poses in one horizontal row, evenly spaced left to right — %d poses, no more and no fewer. Count them before finishing.\n", n, n)
	b.WriteString("- Every pose is the SAME size at one shared scale, each filling about 70-85% of the canvas height. No pose may be noticeably smaller, larger, or set further back than the others.\n")
	b.WriteString("- Leave a generous band of the flat keying background between every pair of poses. The gap must be wide enough that a human can easily see each pose is separate — never touching, overlapping, or bridging.\n")
	b.WriteString("- Each pose is ONE whole, connected body. Never split a body into separate pieces, and never let two poses touch, overlap, or merge.\n")
	b.WriteString("- Center each pose's torso horizontally in its share of the row; arms, legs and head move, but the torso stays put and no body part is cut off by the canvas edge.\n")
	b.WriteString("- Keep all poses standing on one common ground line, unless the action leaves the ground (a jump).\n")
	b.WriteString("- When the body leans or reaches far to one side, keep the torso/hips within the pose's column so that poses do not bridge into the next gap.\n\n")

	b.WriteString(canvasContract())
	b.WriteString("\n")
	b.WriteString(rejectClause())
	b.WriteString("- Favor changes of pose, weight and expression over decoration; any effect must be opaque, hard-edged, and fused to the body.\n")
	b.WriteString("- Keep every pose legible at thumbnail size: bold silhouette, clear limbs, no detail that vanishes when shrunk.\n")

	if f := strings.TrimSpace(feedback); f != "" {
		fmt.Fprintf(&b, "\nArtist revision (apply over everything above): %s\n", f)
	}
	return b.String()
}

// AspectForFrames는 프레임 수에 맞는 생성 종횡비를 고릅니다.
func AspectForFrames(frames int) string {
	switch {
	case frames <= 1:
		return "1:1"
	case frames <= 3:
		return "16:9"
	default:
		return "21:9"
	}
}
