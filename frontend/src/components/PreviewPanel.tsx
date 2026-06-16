import { useEffect, useRef, useState } from "react";
import { Check, ChevronLeft, ChevronRight, Clapperboard, FlipHorizontal2, LayoutGrid, Package, RefreshCw, Wand2 } from "lucide-react";
import { DirectionInfo, StateDef, selectedFrames } from "../types";
import { useI18n } from "../i18n";
import { composeStateLabel, directionName } from "../i18n/catalog";
import AnimPlayer from "./AnimPlayer";
import DirectionGrid from "./DirectionGrid";
import { Button } from "./ui/button";
import { Tabs, TabsList, TabsTrigger } from "./ui/tabs";
import { Textarea } from "./ui/textarea";

type ViewTab = "play" | "frames" | "atlas";

interface IProps {
  state: StateDef | null;
  allStates: StateDef[];
  directions: DirectionInfo[];
  cellSize: number;
  busy: boolean;
  onUpdateState: (id: string, patch: Partial<StateDef>) => void;
  onSelect: (id: string) => void;
  onRegenerate: (id: string, feedback: string) => void;
  onExport: () => void;
}

// 우측 메인 프리뷰 영역: 재생 / 프레임 관리 / 아틀라스
export default function PreviewPanel({ state, allStates, directions, cellSize, busy, onUpdateState, onSelect, onRegenerate, onExport }: IProps) {
  const { t, lang } = useI18n();
  const [tab, setTab] = useState<ViewTab>("play");
  const [feedback, setFeedback] = useState("");

  useEffect(() => setFeedback(state?.feedback ?? ""), [state?.id]);

  const doneStates = allStates.filter((s) => s.status === "done" && selectedFrames(s).length > 0);

  if (!state) {
    return (
      <>
        <div className="panel-head">
          <span className="step-badge">3</span>
          <span className="panel-title">{t("preview_export")}</span>
        </div>
        <div className="preview-body">
          <EmptyHero hasResults={doneStates.length > 0} />
        </div>
      </>
    );
  }

  const frames = selectedFrames(state).map((f) => f.png);

  const toggleFrame = (fid: string) => {
    onUpdateState(state.id, {
      items: state.items.map((f) => (f.id === fid ? { ...f, selected: !f.selected } : f)),
    });
  };

  const moveFrame = (idx: number, dir: -1 | 1) => {
    const items = [...state.items];
    const j = idx + dir;
    if (j < 0 || j >= items.length) return;
    [items[idx], items[j]] = [items[j], items[idx]];
    onUpdateState(state.id, { items });
  };

  return (
    <>
      <div className="panel-head">
        <span className="step-badge">3</span>
        <span className="panel-title">{composeStateLabel(state, lang, t("custom"))}</span>
        {state.status === "done" && state.scores && (
          <div className="score-bar" title={t("score_tooltip")}>
            <span className="score-pill">{t("score")} {(state.scores.overall * 100).toFixed(0)}</span>
            <div className="score-segments">
              <span style={{ width: `${state.scores.identity * 100}%` }} title={`${t("score_identity")}: ${(state.scores.identity * 100).toFixed(0)}`} />
              <span style={{ width: `${state.scores.motion * 100}%` }} title={`${t("score_motion")}: ${(state.scores.motion * 100).toFixed(0)}`} />
              <span style={{ width: `${state.scores.contact * 100}%` }} title={`${t("score_contact")}: ${(state.scores.contact * 100).toFixed(0)}`} />
            </div>
          </div>
        )}
        <span className="hint">{state.name}</span>
        <span className="spacer" />
        <Tabs value={tab} onValueChange={(v) => setTab(v as ViewTab)}>
          <TabsList className="h-7 w-auto">
            <TabsTrigger value="play" className="flex-none px-3">
              {t("preview")}
            </TabsTrigger>
            <TabsTrigger value="frames" className="flex-none px-3">
              {t("frames")}{state.items.length > 0 ? ` ${frames.length}/${state.items.length}` : ""}
            </TabsTrigger>
            <TabsTrigger value="atlas" className="flex-none px-3">
              {t("spritesheet")}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      <div className="preview-body">
        {state.status === "done" && state.warnings.length > 0 && tab !== "atlas" && (
          <div className="preview-warn">
            {state.warnings.map((w, i) => (
              <div key={i}>{w}</div>
            ))}
          </div>
        )}

        {state.status === "generating" && (
          <div className="empty-hero">
            <div className="empty-icon">
              <span className="spinner" style={{ width: 26, height: 26, borderWidth: 3 }} />
            </div>
            <h2>{t("generating_state", { label: composeStateLabel(state, lang, t("custom")) })}</h2>
            <p>{t("generating_state_desc")}</p>
          </div>
        )}

        {state.status !== "generating" && state.items.length === 0 && tab !== "atlas" && (
          <div className="empty-hero">
            <div className="empty-icon">
              <Clapperboard size={26} />
            </div>
            <h2>{t("no_frames_yet")}</h2>
            <p>{t("no_frames_hint")}</p>
          </div>
        )}

        {state.status === "done" && tab === "play" && frames.length > 0 && (
          <AnimPlayer frames={frames} fps={state.fps} loop={state.loop} cellSize={cellSize} />
        )}

        {tab === "play" && state.dirBase && (
          <>
            <div className="hint" style={{ display: "flex", alignItems: "center", gap: 4 }}>
              <LayoutGrid size={11} /> {t("dir8_hint")}
            </div>
            <DirectionGrid
              states={allStates.filter((s) => s.dirBase === state.dirBase)}
              directions={directions}
              selectedId={state.id}
              onSelect={onSelect}
            />
          </>
        )}

        {state.status === "done" && tab === "frames" && state.items.length > 0 && (
          <>
            <div className="frame-grid">
              {state.items.map((f, i) => (
                <div
                  key={f.id}
                  className={`frame-cell checker ${f.selected ? "selected" : "deselected"}`}
                  onClick={() => toggleFrame(f.id)}
                  title={f.selected ? t("click_exclude") : t("click_include")}
                >
                  <img src={f.png} className="pixelated" alt={`frame ${i + 1}`} />
                  <span className="fc-num">#{i + 1}</span>
                  <span className="fc-check">{f.selected ? <Check size={11} /> : null}</span>
                  <span className="fc-move" onClick={(e) => e.stopPropagation()}>
                    <button onClick={() => moveFrame(i, -1)} title={t("move_earlier")}>
                      <ChevronLeft size={10} />
                    </button>
                    <button onClick={() => moveFrame(i, 1)} title={t("move_later")}>
                      <ChevronRight size={10} />
                    </button>
                  </span>
                </div>
              ))}
            </div>

            {state.mirrorOf ? (
              <div className="refine-box">
                <span className="rb-title" style={{ display: "flex", alignItems: "center", gap: 4 }}>
                  <FlipHorizontal2 size={11} /> {t("mirror_result", { dir: directionName(state.mirrorOf, lang) })}
                </span>
                <div className="row">
                  <span className="hint">{t("mirror_regen_hint", { dir: directionName(state.mirrorOf, lang) })}</span>
                  <span className="spacer" />
                  <Button size="sm" disabled={busy} onClick={() => onRegenerate(state.id, "")}>
                    <RefreshCw size={12} /> {t("remirror")}
                  </Button>
                </div>
              </div>
            ) : (
              <div className="refine-box">
                <span className="rb-title">{t("refine_title")}</span>
                <Textarea
                  className="min-h-[52px] bg-white"
                  placeholder={t("feedback_ph")}
                  value={feedback}
                  onChange={(e) => setFeedback(e.target.value)}
                />
                <div className="row">
                  <span className="hint">{t("frame_exclude_hint")}</span>
                  <span className="spacer" />
                  <Button size="sm" disabled={busy} onClick={() => onRegenerate(state.id, feedback)}>
                    <RefreshCw size={12} /> {t("regen_feedback")}
                  </Button>
                </div>
              </div>
            )}

            {state.rawStrip && (
              <div className="raw-strip checker">
                <img src={state.rawStrip} className="pixelated" alt={t("raw_strip_alt")} />
              </div>
            )}
          </>
        )}

        {tab === "atlas" && <AtlasView states={doneStates} cellSize={cellSize} onExport={onExport} />}
      </div>
    </>
  );
}

function EmptyHero({ hasResults }: { hasResults: boolean }) {
  const { t } = useI18n();
  return (
    <div className="empty-hero">
      <div className="empty-icon">
        <Wand2 size={26} />
      </div>
      <h2>{hasResults ? t("select_anim") : t("hero_title")}</h2>
      <p>{t("hero_desc")}</p>
      <div className="steps">
        <div className="step-item">
          <span className="n">1</span>
          <span className="hint">{t("step_char")}</span>
        </div>
        <ChevronRight size={14} className="step-arrow" />
        <div className="step-item">
          <span className="n">2</span>
          <span className="hint">{t("step_anim")}</span>
        </div>
        <ChevronRight size={14} className="step-arrow" />
        <div className="step-item">
          <span className="n">3</span>
          <span className="hint">{t("step_gen")}</span>
        </div>
        <ChevronRight size={14} className="step-arrow" />
        <div className="step-item">
          <span className="n">4</span>
          <span className="hint">{t("step_export")}</span>
        </div>
      </div>
    </div>
  );
}

// 클라이언트 사이드 아틀라스 미리보기
function AtlasView({ states, cellSize, onExport }: { states: StateDef[]; cellSize: number; onExport: () => void }) {
  const { t } = useI18n();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [dims, setDims] = useState({ w: 0, h: 0 });

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    if (states.length === 0) {
      setDims({ w: 0, h: 0 });
      return;
    }
    let cancelled = false;

    (async () => {
      const rows = await Promise.all(
        states.map(async (s) => {
          const imgs = await Promise.all(
            selectedFrames(s).map(
              (f) =>
                new Promise<HTMLImageElement>((resolve) => {
                  const img = new Image();
                  img.onload = () => resolve(img);
                  img.onerror = () => resolve(img);
                  img.src = f.png;
                })
            )
          );
          return { name: s.name, imgs };
        })
      );
      if (cancelled) return;

      const maxFrames = Math.max(1, ...rows.map((r) => r.imgs.length));
      const w = maxFrames * cellSize;
      const h = rows.length * cellSize;
      canvas.width = w;
      canvas.height = h;
      setDims({ w, h });
      const ctx = canvas.getContext("2d")!;
      ctx.imageSmoothingEnabled = false;
      ctx.clearRect(0, 0, w, h);
      rows.forEach((row, ri) => {
        row.imgs.forEach((img, ci) => {
          if (img.naturalWidth) ctx.drawImage(img, ci * cellSize, ri * cellSize, cellSize, cellSize);
        });
      });
    })();

    return () => {
      cancelled = true;
    };
  }, [states, cellSize]);

  if (states.length === 0) {
    return (
      <div className="empty-hero">
        <div className="empty-icon">
          <LayoutGrid size={26} />
        </div>
        <h2>{t("no_done_anim")}</h2>
        <p>{t("atlas_empty_hint")}</p>
      </div>
    );
  }

  return (
    <>
      <div className="row">
        <div className="atlas-meta">
          <span>{t("sheet_size", { w: dims.w, h: dims.h })}</span>
          <span>{t("cell", { n: cellSize })}</span>
          <span>{t("rows_states", { n: states.length })}</span>
        </div>
        <span className="spacer" />
        <Button onClick={onExport}>
          <Package size={13} /> {t("export_full")}
        </Button>
      </div>
      <div className="atlas-wrap checker">
        <canvas ref={canvasRef} />
      </div>
      <p className="hint">{t("atlas_hint")}</p>
    </>
  );
}
