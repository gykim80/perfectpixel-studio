import { useEffect, useRef, useState } from "react";
import { Download, RefreshCw } from "lucide-react";
import { ApplyUpdate, CheckUpdate, OpenDownloadPage, Version } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { useI18n } from "../i18n";
import { Button } from "./ui/button";

interface IUpdateInfo {
  available: boolean;
  current: string;
  latest: string;
  notes: string;
  canSelfUpdate: boolean;
  downloadUrl: string;
  assetSize: number;
  err?: string;
}

type Phase = "idle" | "downloading" | "verifying" | "applying" | "done" | "error";

interface IProgress {
  phase: Phase;
  received: number;
  total: number;
  err?: string;
}

// UpdateBadge는 토픽바에서 현재 버전을 표시하고, 새 빌드가 있으면 클릭 한 번으로
// 다운로드·서명검증·설치·재시작까지 수행하는 자동 업데이트 진입점입니다.
export default function UpdateBadge() {
  const { t } = useI18n();
  const [version, setVersion] = useState("");
  const [info, setInfo] = useState<IUpdateInfo | null>(null);
  const [prog, setProg] = useState<IProgress | null>(null);
  const [checking, setChecking] = useState(false);
  const [checkMsg, setCheckMsg] = useState(""); // "최신 버전" 같은 일시적 결과 메시지
  const busy = useRef(false);

  // 버전 배지 클릭: 수동으로 최신 빌드를 다시 확인한다.
  const onCheck = async () => {
    if (checking || busy.current) return;
    setChecking(true);
    setCheckMsg("");
    try {
      const u = (await CheckUpdate()) as IUpdateInfo;
      setInfo(u);
      // 업데이트가 없으면 잠깐 "최신 버전" 안내를 보여준다.
      if (!u?.available) {
        setCheckMsg(t("update_none"));
        setTimeout(() => setCheckMsg(""), 3000);
      }
    } catch {
      setCheckMsg(t("update_error", { err: "" }));
      setTimeout(() => setCheckMsg(""), 3000);
    } finally {
      setChecking(false);
    }
  };

  useEffect(() => {
    Version()
      .then((v) => setVersion(v ?? ""))
      .catch(() => {});
    // 시작 시 조용히 한 번 확인 — 실패해도 UI 는 버전만 표시한다.
    CheckUpdate()
      .then((u) => setInfo(u as IUpdateInfo))
      .catch(() => {});
    const off = EventsOn("updater:progress", (d: any) => setProg(d as IProgress));
    return () => off();
  }, []);

  const onApply = async () => {
    if (busy.current || !info?.available) return;
    busy.current = true;
    setProg({ phase: "downloading", received: 0, total: info.assetSize });
    try {
      // 미지원 플랫폼이면 백엔드가 다운로드 페이지를 대신 연다.
      if (!info.canSelfUpdate) {
        await OpenDownloadPage();
        busy.current = false;
        setProg(null);
        return;
      }
      await ApplyUpdate(); // 성공 시 프로세스가 종료되므로 보통 반환되지 않는다.
    } catch (e) {
      setProg({ phase: "error", received: 0, total: 0, err: String(e) });
      busy.current = false;
    }
  };

  const progressText = (): string => {
    if (!prog) return "";
    switch (prog.phase) {
      case "downloading": {
        const pct = prog.total > 0 ? Math.min(100, Math.round((prog.received / prog.total) * 100)) : 0;
        return t("update_downloading", { percent: pct });
      }
      case "verifying":
        return t("update_verifying");
      case "applying":
        return t("update_applying");
      case "done":
        return t("update_done");
      case "error":
        return t("update_error", { err: prog.err ?? "" });
      default:
        return "";
    }
  };

  // 진행 중일 때는 상태 텍스트를 보여준다.
  if (prog && prog.phase !== "idle") {
    return (
      <span className="update-badge update-badge-progress" title={progressText()}>
        {prog.phase !== "error" && <RefreshCw size={12} className="update-spin" />}
        {progressText()}
      </span>
    );
  }

  // 업데이트가 있으면 강조 버튼, 없으면 조용한 버전 표시.
  if (info?.available) {
    return (
      <Button
        variant="default"
        size="sm"
        onClick={onApply}
        title={t("update_available_tip", { version: info.latest })}
      >
        <Download size={13} /> {t("update_available", { version: info.latest })}
      </Button>
    );
  }

  // 일시적 결과 메시지(최신 버전 등)
  if (checkMsg) {
    return <span className="update-badge">{checkMsg}</span>;
  }

  return (
    <button
      type="button"
      className="update-badge update-badge-btn"
      onClick={onCheck}
      disabled={checking}
      title={t("update_check_tip", { version: version || info?.current || "" })}
    >
      {checking ? (
        <>
          <RefreshCw size={12} className="update-spin" />
          {t("update_checking")}
        </>
      ) : (
        <>v{version || info?.current || "—"}</>
      )}
    </button>
  );
}
