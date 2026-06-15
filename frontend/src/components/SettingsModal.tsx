import { useState } from "react";
import { Loader2 } from "lucide-react";
import { SaveProviderKey, SaveProviderModel, SetProvider } from "../../wailsjs/go/main/App";
import { useI18n, LANGUAGES, Lang } from "../i18n";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";
import { Tabs, TabsList, TabsTrigger } from "./ui/tabs";

export interface IProviderInfo {
  hasKey: boolean;
  keyPreview: string;
  model: string;
  models?: string[]; // 선택 가능한 모델 목록 (최신 모델이 맨 앞)
}

export interface ISettings {
  provider: string;
  providers: Record<string, IProviderInfo>;
}

interface IProps {
  settings: ISettings;
  onClose: () => void;
  onSaved: (msg: string, keepOpen?: boolean) => void;
}

const PROVIDERS: { key: string; label: string; placeholder: string }[] = [
  { key: "gemini", label: "Gemini", placeholder: "AIza..." },
  { key: "openai", label: "OpenAI", placeholder: "sk-..." },
  { key: "openrouter", label: "OpenRouter", placeholder: "sk-or-..." },
  { key: "fal", label: "fal.ai", placeholder: "key_id:key_secret" },
  { key: "byteplus", label: "BytePlus", placeholder: "ark-..." },
];

// 멀티 프로바이더 설정 모달 (shadcn Dialog)
export default function SettingsModal({ settings, onClose, onSaved }: IProps) {
  const { t, lang, setLang } = useI18n();
  const activeHasKey = !!settings.providers?.[settings.provider]?.hasKey;
  const [tab, setTab] = useState(settings.provider || "gemini");
  const [key, setKey] = useState("");
  const [model, setModel] = useState(settings.providers?.[settings.provider || "gemini"]?.model ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const info = settings.providers?.[tab];
  const meta = PROVIDERS.find((p) => p.key === tab)!;
  const isActive = settings.provider === tab;

  const switchTab = (k: string) => {
    setTab(k);
    setKey("");
    setModel(settings.providers?.[k]?.model ?? "");
    setError("");
  };

  const saveKey = async () => {
    if (!key.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      await SaveProviderKey(tab, key.trim());
      setKey("");
      onSaved(t("toast_key_saved", { provider: meta.label }));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const saveModel = async (explicit?: string) => {
    if (busy) return;
    const next = (explicit ?? model).trim();
    if (explicit !== undefined) setModel(next);
    setBusy(true);
    setError("");
    try {
      await SaveProviderModel(tab, next);
      onSaved(t("toast_model_saved", { provider: meta.label }), true);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const activate = async () => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await SetProvider(tab);
      onSaved(t("toast_provider_active", { provider: meta.label }));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && activeHasKey && onClose()}>
      <DialogContent hideClose={!activeHasKey}>
        <DialogTitle>{t("settings")}</DialogTitle>

        <div className="field">
          <Label>{t("language")}</Label>
          <Select value={lang} onValueChange={(v) => setLang(v as Lang)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LANGUAGES.map((l) => (
                <SelectItem key={l.code} value={l.code}>
                  {l.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <Tabs value={tab} onValueChange={switchTab}>
          <TabsList>
            {PROVIDERS.map((p) => (
              <TabsTrigger key={p.key} value={p.key}>
                {p.label}
                {settings.providers?.[p.key]?.hasKey && <span className="ml-1 text-success">●</span>}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <div className="field">
          <Label>
            {t("api_key", { provider: meta.label })} {isActive && <em className="not-italic text-primary">{t("in_use")}</em>}
          </Label>
          <Input
            type="password"
            placeholder={info?.hasKey ? t("saved_as", { preview: info.keyPreview }) : meta.placeholder}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && saveKey()}
            autoFocus
          />
        </div>

        <div className="field">
          <Label>{t("image_model")}</Label>
          {(info?.models?.length ?? 0) > 0 && (
            <Select
              value={info!.models!.includes(model.trim()) ? model.trim() : undefined}
              onValueChange={(v) => v !== model.trim() && saveModel(v)}
            >
              <SelectTrigger disabled={busy}>
                <SelectValue placeholder={t("custom_model_inuse")} />
              </SelectTrigger>
              <SelectContent>
                {info!.models!.map((m, i) => (
                  <SelectItem key={m} value={m}>
                    {m}
                    {i === 0 && t("latest")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <div className="row">
            <Input
              className="flex-1"
              placeholder={info?.model ?? ""}
              value={model}
              onChange={(e) => setModel(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && model.trim() !== (info?.model ?? "") && saveModel()}
            />
            <Button
              variant="outline"
              size="sm"
              onClick={() => saveModel()}
              disabled={busy || model.trim() === (info?.model ?? "")}
              title={t("model_apply_tip")}
            >
              {t("apply")}
            </Button>
          </div>
        </div>

        <p className="hint">
          {t(`help_${tab}`)}
          <br />{t("key_local_note")}
        </p>

        {error && <p className="hint" style={{ color: "hsl(var(--destructive))" }}>{error}</p>}

        <div className="row justify-end">
          {activeHasKey && (
            <Button variant="ghost" onClick={onClose}>
              {t("close")}
            </Button>
          )}
          {!isActive && info?.hasKey && (
            <Button variant="outline" onClick={activate} disabled={busy}>
              {t("use_provider")}
            </Button>
          )}
          <Button onClick={saveKey} disabled={busy || !key.trim()}>
            {busy && <Loader2 size={13} className="animate-spin" />}
            {busy ? t("checking") : t("save_key")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
