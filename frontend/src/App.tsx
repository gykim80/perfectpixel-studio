import { useEffect, useRef, useState } from "react";
import { Images, Package, Plus, Settings, X } from "lucide-react";
import { CancelGeneration, ClearSession, ExportProject, GenerateState, GetSettings, ListDirections, ListPresets, LoadSession, MirrorFrames, RevealInFinder, SaveSession } from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime/runtime";
import CharacterPanel from "./components/CharacterPanel";
import GalleryModal from "./components/GalleryModal";
import PreviewPanel from "./components/PreviewPanel";
import SettingsModal, { ISettings } from "./components/SettingsModal";
import StatesPanel from "./components/StatesPanel";
import { Button } from "./components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "./components/ui/dialog";
import { CharacterDef, DirectionInfo, FALLBACK_PRESETS, FrameItem, PresetInfo, StateDef, selectedFrames, uid } from "./types";
import { useI18n } from "./i18n";
import { directionName } from "./i18n/catalog";
import logoUrl from "./assets/logo.svg";

interface IToast {
  id: string;
  kind: "info" | "error" | "success";
  text: string;
}

// 활성 프로바이더에 키가 있는지 확인
const hasActiveKey = (s: ISettings | null) => !!s?.providers?.[s.provider]?.hasKey;

const PROVIDER_LABELS: Record<string, string> = {
  gemini: "Gemini",
  openai: "OpenAI",
  openrouter: "OpenRouter",
  fal: "fal.ai",
  byteplus: "BytePlus",
};

export default function App() {
  const { t, lang } = useI18n();
  const [settings, setSettings] = useState<ISettings | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [showGallery, setShowGallery] = useState(false);
  const [confirmNew, setConfirmNew] = useState(false);
  const [character, setCharacter] = useState<CharacterDef>({
    image: null,
    name: "",
    description: "",
    styleKey: "pixel",
    styleCustom: "",
  });
  const [cellSize, setCellSize] = useState(256);
  const [states, setStates] = useState<StateDef[]>([]);
  const [directions, setDirections] = useState<DirectionInfo[]>([]);
  const [presets, setPresets] = useState<PresetInfo[]>(FALLBACK_PRESETS);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState("");
  const [toasts, setToasts] = useState<IToast[]>([]);

  // 최신 상태 참조 (비동기 루프에서 사용)
  const statesRef = useRef(states);
  statesRef.current = states;
  const charRef = useRef(character);
  charRef.current = character;
  const cellRef = useRef(cellSize);
  cellRef.current = cellSize;
  const cancelRef = useRef(false); // 전체 생성 루프 중단 플래그
  const busyRef = useRef(busy);
  busyRef.current = busy;

  const restoredRef = useRef(false); // 세션 복원 완료 전 자동 저장 방지

  useEffect(() => {
    refreshSettings();
    ListDirections()
      .then((list: any) => setDirections(list ?? []))
      .catch(() => {});
    ListPresets()
      .then((list: any) => {
        if (Array.isArray(list) && list.length > 0) setPresets(list);
      })
      .catch(() => {}); // 실패 시 FALLBACK_PRESETS 유지
    const off = EventsOn("progress", (data: any) => {
      const st = data?.state ? `[${data.state}] ` : "";
      setProgress(`${st}${data?.message ?? ""}`);
    });

    // 이전 작업 세션 복원
    (async () => {
      try {
        const raw = await LoadSession();
        if (raw) {
          const s = JSON.parse(raw);
          if (s?.character) {
            setCharacter({
              image: s.character.image ?? null,
              name: s.character.name ?? "",
              description: s.character.description ?? "",
              styleKey: s.character.styleKey ?? "pixel",
              styleCustom: s.character.styleCustom ?? "",
            });
          }
          if (typeof s?.cellSize === "number") setCellSize(s.cellSize);
          if (Array.isArray(s?.states)) {
            // 생성 도중 종료된 상태는 안전하게 정리, 구버전 절차 애니메이션 상태는 제외
            const states: StateDef[] = s.states
              .filter((st: any) => st?.mode !== "procedural")
              .map((st: StateDef) => ({
                ...st,
                status: st.status === "generating" ? (st.items?.length > 0 ? "done" : "idle") : st.status,
              }));
            setStates(states);
            if (s.selectedId && states.some((x) => x.id === s.selectedId)) setSelectedId(s.selectedId);
          }
        }
      } catch {
        // 손상된 세션은 무시
      } finally {
        restoredRef.current = true;
      }
    })();

    return off;
  }, []);

  // 작업 세션 자동 저장 (디바운스)
  useEffect(() => {
    if (!restoredRef.current) return;
    const t = setTimeout(() => {
      SaveSession(JSON.stringify({ v: 1, character, cellSize, states, selectedId })).catch(() => {});
    }, 1200);
    return () => clearTimeout(t);
  }, [character, cellSize, states, selectedId]);

  // 전역 단축키: ⌘, 설정 / ⌘E 내보내기 / ⌘G 갤러리
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      if (e.key === ",") {
        e.preventDefault();
        setShowSettings(true);
      } else if (e.key.toLowerCase() === "e") {
        e.preventDefault();
        if (!busyRef.current) handleExport();
      } else if (e.key.toLowerCase() === "g") {
        e.preventDefault();
        setShowGallery((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const refreshSettings = async (): Promise<ISettings | null> => {
    try {
      const s = (await GetSettings()) as unknown as ISettings;
      setSettings(s);
      if (!hasActiveKey(s)) setShowSettings(true);
      return s;
    } catch {
      return null;
    }
  };

  const toast = (kind: IToast["kind"], text: string) => {
    const t: IToast = { id: uid("toast"), kind, text };
    setToasts((prev) => [...prev, t]);
    setTimeout(() => setToasts((prev) => prev.filter((x) => x.id !== t.id)), 5000);
  };

  const updateState = (id: string, patch: Partial<StateDef>) => {
    // statesRef를 즉시 갱신: 방향 세트 순차 생성 루프가 리렌더 전에
    // 직전 방향의 결과(정면 스트립, 미러 소스)를 읽을 수 있어야 함
    statesRef.current = statesRef.current.map((s) => (s.id === id ? { ...s, ...patch } : s));
    setStates(statesRef.current);
  };

  const generateOne = async (id: string, feedback = ""): Promise<boolean> => {
    const st = statesRef.current.find((s) => s.id === id);
    const ch = charRef.current;
    if (!st || !ch.image) return false;

    const prevItems = st.items;
    updateState(id, { status: "generating", error: undefined, warnings: [], feedback });
    try {
      // 미러 방향: AI 호출 없이 소스 방향 프레임을 좌우 반전
      if (st.mirrorOf) {
        const src = statesRef.current.find(
          (s) => s.dirBase === st.dirBase && s.facing === st.mirrorOf && s.status === "done" && s.items.length > 0
        );
        if (!src) {
          updateState(id, {
            status: "error",
            items: prevItems,
            error: t("err_mirror_source", { dir: directionName(st.mirrorOf, lang) }),
          });
          return false;
        }
        const mirrored: string[] = (await MirrorFrames(src.items.map((f) => f.png))) ?? [];
        const items: FrameItem[] = mirrored.map((png, i) => ({
          id: uid("fr"),
          png,
          selected: src.items[i]?.selected ?? true,
        }));
        updateState(id, {
          status: items.length > 0 ? "done" : "error",
          error: items.length > 0 ? undefined : t("err_mirror_fail"),
          items,
          warnings: [],
        });
        return items.length > 0;
      }

      // 방향 세트 소속이면 정면(south) 스트립을 모션 참조로 전달
      let refStrip = "";
      if (st.dirBase && st.facing && st.facing !== "south") {
        const south = statesRef.current.find((s) => s.dirBase === st.dirBase && s.facing === "south");
        refStrip = south?.rawStrip ?? "";
      }

      const res: any = await GenerateState({
        baseImage: ch.image,
        description: ch.description,
        styleKey: ch.styleKey,
        styleCustom: ch.styleCustom,
        cellSize: cellRef.current,
        safeMargin: 0,
        feedback,
        refStrip,
        state: { name: st.name, frames: st.frames, fps: st.fps, loop: st.loop, action: st.action, facing: st.facing ?? "" },
      } as any);

      const items: FrameItem[] = (res.frames ?? []).map((png: string) => ({
        id: uid("fr"),
        png,
        selected: true,
      }));
      updateState(id, {
        status: items.length > 0 ? "done" : "error",
        error: items.length > 0 ? undefined : t("err_no_frames"),
        items,
        rawStrip: res.rawStrip || undefined,
        warnings: res.warnings ?? [],
      });
      return items.length > 0;
    } catch (e) {
      const msg = String(e);
      if (msg.includes("취소")) {
        // 취소: 이전 결과를 보존하고 조용히 복귀
        updateState(id, { status: prevItems.length > 0 ? "done" : "idle", items: prevItems });
        toast("info", t("toast_gen_canceled"));
        return false;
      }
      updateState(id, { status: "error", error: msg });
      return false;
    }
  };

  // 여러 상태를 동시성 제한 하에 병렬 생성하고 성공 개수를 반환한다.
  // (fal 등 일부 프로바이더의 rate-limit 대비 — 8방향 세트와 동일한 기본값 3)
  const GEN_CONCURRENCY = 3;
  const generateBatch = async (ids: string[]): Promise<number> => {
    const queue = [...ids];
    let ok = 0;
    const worker = async () => {
      while (queue.length > 0) {
        if (cancelRef.current) return;
        const id = queue.shift()!;
        setSelectedId(id);
        if (await generateOne(id)) ok += 1;
      }
    };
    await Promise.all(Array.from({ length: Math.min(GEN_CONCURRENCY, queue.length) }, worker));
    return ok;
  };

  const handleGenerate = async (id: string) => {
    if (busy) return;
    setBusy(true);
    cancelRef.current = false;
    setSelectedId(id);
    await generateOne(id);
    setBusy(false);
    setProgress("");
  };

  const handleRegenerate = async (id: string, feedback: string) => {
    if (busy) return;
    setBusy(true);
    cancelRef.current = false;
    await generateOne(id, feedback);
    setBusy(false);
    setProgress("");
  };

  // 8방향 세트: 5방향 AI 생성(south가 정면 레퍼런스) + 3방향 좌우 미러링
  const handleGenerateDirectionSet = async (id: string) => {
    if (busy || directions.length === 0) return;
    const origin = statesRef.current.find((s) => s.id === id);
    if (!origin || !charRef.current.image) return;

    setBusy(true);
    cancelRef.current = false;

    const base = origin.dirBase ?? origin.name;
    const labelBase = origin.dirBase ? origin.label.split("·")[0] : origin.label;

    // 세트 상태 보장: 클릭한 상태를 south로 전환하고 누락 방향만 추가
    let next = [...statesRef.current];
    if (!origin.dirBase) {
      // 다른 방향으로 생성된 기존 프레임은 south로 재사용할 수 없으므로 초기화
      const keepItems = !origin.facing || origin.facing === "south";
      next = next.map((s) =>
        s.id === id
          ? {
              ...s,
              name: `${base}-south`,
              label: `${labelBase}·정면`,
              dirBase: base,
              facing: "south",
              ...(keepItems ? {} : { items: [], status: "idle" as const, rawStrip: undefined, warnings: [] }),
            }
          : s
      );
    }
    const inSet = (key: string) => next.some((s) => s.dirBase === base && s.facing === key);
    for (const d of directions) {
      if (inSet(d.key)) continue;
      next.push({
        id: uid("st"),
        name: `${base}-${d.key}`,
        label: `${labelBase}·${d.label}`,
        frames: origin.frames,
        fps: origin.fps,
        loop: origin.loop,
        action: origin.action,
        status: "idle",
        items: [],
        warnings: [],
        feedback: "",
        facing: d.key,
        dirBase: base,
        mirrorOf: d.mirrorOf || undefined,
      });
    }
    statesRef.current = next;
    setStates(next);

    // 생성 순서: south 먼저(정면 레퍼런스, 나머지 방향의 모션 참조) →
    // 나머지 AI 방향은 동시성 제한 병렬 생성 → 미러 3종(로컬, 빠름)
    setSelectedId(id); // 세트 미리보기(방향 그리드)가 보이도록 south 선택
    let ok = 0;
    let failed = 0;
    const genKey = async (key: string): Promise<boolean | null> => {
      const st = statesRef.current.find((s) => s.dirBase === base && s.facing === key);
      if (!st) return null;
      if (st.status === "done" && st.items.length > 0) return true; // 이미 완성된 방향 재사용
      return generateOne(st.id);
    };
    const tally = (r: boolean | null) => {
      if (r === true) ok += 1;
      else if (r === false) failed += 1;
    };

    // 1) south (모션 참조용) — 반드시 먼저 완료
    if (!cancelRef.current) tally(await genKey("south"));

    // 2) 나머지 AI 방향 병렬 (fal rate-limit 대비 동시성 3개로 제한)
    const aiRest = ["east", "north", "south-east", "north-east"];
    const queue = [...aiRest];
    const CONCURRENCY = 3;
    const worker = async () => {
      while (queue.length > 0) {
        if (cancelRef.current) return;
        const key = queue.shift()!;
        tally(await genKey(key));
      }
    };
    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, queue.length) }, worker));

    // 3) 미러 방향 (east/se/ne 완료 후, AI 호출 없이 좌우 반전)
    const mirrorKeys = directions.filter((d) => d.mirrorOf).map((d) => d.key);
    for (const key of mirrorKeys) {
      if (cancelRef.current) break;
      tally(await genKey(key));
    }
    setBusy(false);
    setProgress("");
    if (!cancelRef.current) {
      toast(
        failed === 0 ? "success" : "info",
        failed === 0 ? t("toast_dirset", { ok }) : t("toast_dirset_failed", { ok, failed })
      );
    }
  };

  const handleGenerateAll = async () => {
    if (busy) return;
    setBusy(true);
    cancelRef.current = false;
    const pending = statesRef.current.filter((s) => s.status !== "done");
    // 미러 방향은 소스 방향이 먼저 생성되어야 하므로 비미러를 먼저 병렬 생성하고,
    // 그 다음 미러를 병렬 생성한다.
    const nonMirror = pending.filter((s) => !s.mirrorOf).map((s) => s.id);
    const mirror = pending.filter((s) => s.mirrorOf).map((s) => s.id);
    const total = nonMirror.length + mirror.length;
    let ok = 0;
    ok += await generateBatch(nonMirror);
    if (!cancelRef.current && mirror.length > 0) {
      ok += await generateBatch(mirror);
    }
    setBusy(false);
    setProgress("");
    if (total > 0 && !cancelRef.current) {
      toast(ok === total ? "success" : "info", t("toast_genall", { ok, total }));
    }
  };

  // 커스텀 N개를 한 번에 추가하고 곧바로 순차 생성
  const handleAddCustomBatch = async (count: number) => {
    if (busy) return;
    const n = Math.max(1, Math.min(10, count));
    const base = statesRef.current.length;
    const created: StateDef[] = Array.from({ length: n }, (_, i) => ({
      id: uid("st"),
      name: `custom${base + 1 + i}`,
      label: "커스텀",
      frames: 4,
      fps: 8,
      loop: true,
      action: "",
      status: "idle",
      items: [],
      warnings: [],
      feedback: "",
    }));
    // statesRef를 즉시 갱신: generateOne이 리렌더 전에 새 상태를 찾을 수 있도록
    statesRef.current = [...statesRef.current, ...created];
    setStates(statesRef.current);
    const ids = created.map((s) => s.id);
    setSelectedId(ids[ids.length - 1]);

    if (!charRef.current.image || !hasActiveKey(settings)) return;

    setBusy(true);
    cancelRef.current = false;
    // 배치 전체를 동시성 제한 하에 병렬 생성
    const ok = await generateBatch(ids);
    setBusy(false);
    setProgress("");
    if (!cancelRef.current) {
      toast(ok === ids.length ? "success" : "info", t("toast_custom", { ok, total: ids.length }));
    }
  };

  const handleCancel = () => {
    cancelRef.current = true;
    CancelGeneration();
  };

  // 새 프로젝트: 작업 내용이 있으면 인앱 확인 모달을 띄우고, 없으면 바로 초기화.
  // (window.confirm은 Wails WKWebView에서 동작하지 않으므로 사용하지 않음)
  const handleNewProject = () => {
    if (busy) return;
    const hasWork = !!charRef.current.image || statesRef.current.length > 0;
    if (hasWork) {
      setConfirmNew(true);
      return;
    }
    resetProject();
  };

  const resetProject = async () => {
    setConfirmNew(false);
    setCharacter({ image: null, name: "", description: "", styleKey: "pixel", styleCustom: "" });
    setStates([]);
    setSelectedId(null);
    setCellSize(256);
    try {
      await ClearSession();
    } catch {
      // 세션 파일 삭제 실패는 무시 (다음 자동 저장이 덮어씀)
    }
    toast("info", t("toast_new_project"));
  };

  const handleExport = async () => {
    const done = statesRef.current.filter((s) => s.status === "done" && selectedFrames(s).length > 0);
    if (done.length === 0) {
      toast("error", t("toast_no_export"));
      return;
    }
    try {
      const outDir: any = await ExportProject({
        character: charRef.current.name.trim() || "character",
        cellSize: cellRef.current,
        states: done.map((s) => ({
          name: s.name,
          fps: s.fps,
          loop: s.loop,
          frames: selectedFrames(s).map((f) => f.png),
        })),
      } as any);
      if (outDir) {
        toast("success", t("toast_export_done", { dir: outDir }));
        RevealInFinder(outDir);
      }
    } catch (e) {
      toast("error", String(e));
    }
  };

  const selectedState = states.find((s) => s.id === selectedId) ?? null;
  const exportable = states.some((s) => s.status === "done" && selectedFrames(s).length > 0);

  return (
    <div className="app">
      <header className="topbar">
        <div className="logo">
          <img className="logo-mark" src={logoUrl} alt={t("logo_alt")} />
        </div>
        <div className="topbar-status">
          {progress && (
            <span className="progress-pill">
              <span className="spinner" />
              {progress}
              <button className="pp-cancel" onClick={handleCancel} title={t("cancel_generation")}>
                <X size={10} />
              </button>
            </span>
          )}
        </div>
        <div className="topbar-actions">
          <Button variant="ghost" size="sm" disabled={busy} onClick={handleNewProject} title={t("new_project_tip")}>
            <Plus size={13} /> {t("new_project")}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setShowGallery(true)} title={t("gallery_tip")}>
            <Images size={13} /> {t("gallery")}
          </Button>
          <Button size="sm" disabled={!exportable} onClick={handleExport} title={t("export_tip")}>
            <Package size={13} /> {t("export")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            title={settings ? t("provider_tip", { provider: PROVIDER_LABELS[settings.provider] ?? settings.provider, model: settings.providers?.[settings.provider]?.model ?? "" }) : t("settings_tip")}
            onClick={() => setShowSettings(true)}
          >
            <Settings size={13} />
            {settings ? PROVIDER_LABELS[settings.provider] ?? settings.provider : t("settings")}
            {settings && !hasActiveKey(settings) && <span className="text-destructive font-bold">!</span>}
          </Button>
        </div>
      </header>

      <main className="workspace">
        <section className="panel panel-char">
          <CharacterPanel
            character={character}
            cellSize={cellSize}
            busy={busy}
            onChange={setCharacter}
            onCellSize={setCellSize}
            onError={(m) => (m.includes("취소") ? toast("info", t("toast_gen_canceled")) : toast("error", m))}
          />
        </section>

        <section className="panel panel-anim">
          <StatesPanel
            states={states}
            directions={directions}
            presets={presets}
            selectedId={selectedId}
            canGenerate={!!character.image && hasActiveKey(settings)}
            hasImage={!!character.image}
            busy={busy}
            onStates={setStates}
            onSelect={setSelectedId}
            onGenerate={handleGenerate}
            onGenerateAll={handleGenerateAll}
            onAddCustomBatch={handleAddCustomBatch}
            onGenerateDirectionSet={handleGenerateDirectionSet}
          />
        </section>

        <section className="panel panel-preview">
          <PreviewPanel
            state={selectedState}
            allStates={states}
            directions={directions}
            cellSize={cellSize}
            busy={busy}
            onUpdateState={updateState}
            onSelect={setSelectedId}
            onRegenerate={handleRegenerate}
            onExport={handleExport}
          />
        </section>
      </main>

      {showSettings && settings && (
        <SettingsModal
          settings={settings}
          onClose={() => setShowSettings(false)}
          onSaved={async (msg, keepOpen) => {
            const s = await refreshSettings();
            if (!keepOpen && hasActiveKey(s)) setShowSettings(false);
            toast("success", msg);
          }}
        />
      )}

      {showGallery && (
        <GalleryModal onClose={() => setShowGallery(false)} onError={(m) => toast("error", m)} />
      )}

      <Dialog open={confirmNew} onOpenChange={(o) => !o && setConfirmNew(false)}>
        <DialogContent className="w-[380px]">
          <DialogTitle>{t("confirm_new_title")}</DialogTitle>
          <DialogDescription>
            {t("confirm_new_desc")}
          </DialogDescription>
          <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 4 }}>
            <Button variant="ghost" size="sm" onClick={() => setConfirmNew(false)}>
              {t("cancel")}
            </Button>
            <Button variant="destructive" size="sm" onClick={resetProject}>
              {t("confirm_new_ok")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <div className="toasts">
        {toasts.map((t) => (
          <div key={t.id} className={`toast ${t.kind}`}>
            {t.text}
          </div>
        ))}
      </div>
    </div>
  );
}
