// 프론트엔드 도메인 타입 정의

export interface FrameItem {
  id: string;
  png: string; // dataURL
  selected: boolean;
}

export interface FrameScores {
  identity: number;
  motion: number;
  contact: number;
  overall: number;
}

export type StateStatus = "idle" | "generating" | "done" | "error";

export interface StateDef {
  id: string;
  name: string; // 영문 상태명 (export용)
  label: string; // 한글 표시명
  frames: number;
  fps: number;
  loop: boolean;
  action: string;
  status: StateStatus;
  error?: string;
  rawStrip?: string;
  items: FrameItem[];
  warnings: string[];
  feedback: string;
  scores?: FrameScores;
  facing?: string; // 8방향 키 (south 등, 미지정 시 방향 지시 없음)
  dirBase?: string; // 8방향 세트의 베이스 상태명 (세트 소속일 때만)
  mirrorOf?: string; // 미러링 소스 방향 키 (west→east 등, AI 생성 안 함)
}

// 백엔드 sprite.DirectionInfo와 동일 구조 (ListDirections 응답)
export interface DirectionInfo {
  key: string;
  label: string;
  short: string;
  mirrorOf: string;
  row: number;
  col: number;
}

// 방향 키 → 한글 라벨 변환 (목록에 없으면 키 그대로)
export function directionLabel(directions: DirectionInfo[], key: string | undefined): string {
  if (!key) return "";
  return directions.find((d) => d.key === key)?.label ?? key;
}

export interface CharacterDef {
  image: string | null; // dataURL
  name: string; // 내보내기 파일 프리픽스로 사용
  description: string;
  styleKey: string;
  styleCustom: string;
}

export interface StatePreset {
  name: string;
  label: string;
  frames: number;
  fps: number;
  loop: boolean;
  action: string;
}

// 백엔드 sprite.PresetInfo와 동일 구조 (ListPresets 응답)
export interface PresetInfo {
  name: string;
  label: string;
  category: string;
  action: string;
  frames: number;
  fps: number;
  loop: boolean;
}

export const STATE_PRESETS: StatePreset[] = [
  { name: "idle", label: "대기", frames: 4, fps: 6, loop: true, action: "subtle breathing idle in place" },
  { name: "walk", label: "걷기", frames: 6, fps: 10, loop: true, action: "side-view walking cycle facing right" },
  { name: "run", label: "달리기", frames: 6, fps: 12, loop: true, action: "fast side-view running cycle facing right" },
  { name: "jump", label: "점프", frames: 5, fps: 10, loop: false, action: "crouch, take off, airborne peak, land" },
  { name: "attack", label: "공격", frames: 5, fps: 12, loop: false, action: "melee attack with wind-up, strike, recovery" },
  { name: "hurt", label: "피격", frames: 3, fps: 10, loop: false, action: "recoil from being hit" },
  { name: "death", label: "사망", frames: 5, fps: 8, loop: false, action: "stagger, collapse, lie flat on the ground" },
  { name: "wave", label: "인사", frames: 4, fps: 8, loop: true, action: "friendly hand wave, body still" },
];

export const STYLE_OPTIONS = [
  { key: "pixel", label: "픽셀 아트" },
  { key: "chibi", label: "치비" },
  { key: "cartoon", label: "카툰" },
  { key: "retro16", label: "16비트 레트로" },
  { key: "custom", label: "직접 입력" },
];

export const CELL_SIZES = [128, 256, 512];

let seq = 0;
export function uid(prefix: string): string {
  seq += 1;
  return `${prefix}-${Date.now().toString(36)}-${seq}`;
}

export function presetToState(p: StatePreset): StateDef {
  return {
    id: uid("st"),
    name: p.name,
    label: p.label,
    frames: p.frames,
    fps: p.fps,
    loop: p.loop,
    action: p.action,
    status: "idle",
    items: [],
    warnings: [],
    feedback: "",
  };
}

// ListPresets 카탈로그 항목을 애니메이션 상태로 변환합니다.
export function presetInfoToState(p: PresetInfo): StateDef {
  return {
    id: uid("st"),
    name: p.name,
    label: p.label,
    frames: p.frames,
    fps: p.fps,
    loop: p.loop,
    action: p.action,
    status: "idle",
    items: [],
    warnings: [],
    feedback: "",
  };
}

// STATE_PRESETS를 PresetInfo 형태로 변환한 폴백 카탈로그 (ListPresets 실패 시 사용)
export const FALLBACK_PRESETS: PresetInfo[] = STATE_PRESETS.map((p) => ({
  ...p,
  category: "기본 동작",
}));

export function selectedFrames(s: StateDef): FrameItem[] {
  return s.items.filter((f) => f.selected);
}
