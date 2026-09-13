import { Trash2 } from "lucide-react";
import type { DetectedVar } from "../lib/composeVars";

export type EnvRow = { key: string; value: string; custom?: boolean };

const dangerIconAction = "rounded p-1 text-muted hover:bg-paper hover:text-text disabled:cursor-not-allowed disabled:opacity-45 hover:bg-fail/10 hover:text-fail";

const VAR_KIND_STYLE: Record<DetectedVar["kind"], string> = {
  required: "border-[#BFD5CE] text-run",
  optional: "border-line text-muted",
  error: "border-fail/40 text-fail",
};
const VAR_KIND_LABEL: Record<DetectedVar["kind"], string> = { required: "required", optional: "optional", error: "required w/ error" };

/** One row of an environment-variables panel: a detected ${VAR}'s value, or a
 * manually-added key the compose file doesn't reference (yet). Shared by the
 * stack Editor tab and the Graph tab's service property panel. */
export function EnvVarRow({ row, kind, onChange, onRemove, disabled }: { row: EnvRow; kind?: DetectedVar["kind"]; onChange: (row: EnvRow) => void; onRemove?: () => void; disabled: boolean }) {
  return <div className="flex items-center gap-2 border-b border-linesoft py-1.5 last:border-0">
    {row.custom
      ? <input aria-label="Variable name" disabled={disabled} value={row.key} onChange={(e) => onChange({ ...row, key: e.target.value })} placeholder="KEY" className="w-[42%] rounded border border-line bg-panel px-2 py-1 font-mono text-[12px]" />
      : <span className="w-[42%] truncate font-mono text-[12.5px]" title={row.key}>{row.key}</span>}
    <input aria-label={`Value for ${row.key || "variable"}`} disabled={disabled} value={row.value} onChange={(e) => onChange({ ...row, value: e.target.value })} placeholder="value" className="min-w-0 flex-1 rounded border border-line bg-panel px-2 py-1 font-mono text-[12px]" />
    {kind && <span className={`shrink-0 rounded-sm border px-1 py-0.5 font-mono text-[10px] ${VAR_KIND_STYLE[kind]}`}>{VAR_KIND_LABEL[kind]}</span>}
    {onRemove && <button type="button" title="Remove" aria-label={`Remove ${row.key || "variable"}`} disabled={disabled} onClick={onRemove} className={dangerIconAction}><Trash2 size={13} /></button>}
  </div>;
}
