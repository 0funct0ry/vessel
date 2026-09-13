import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";

export type KVRow = { a: string; b: string };

const rowInput = "min-w-0 flex-1 rounded border border-line bg-panel px-2 py-1 font-mono text-[12px]";

/** A structured list of two-column rows (Key/Value, Host/Container, …) with
 * a per-row delete button and a single add-row control — the pattern
 * EnvVarRow already established for environment variables, generalized so
 * Ports/Labels/Driver-options/Volumes share one implementation instead of
 * each being a freeform textarea a user has to hand-format.
 *
 * Row state is owned locally (seeded from `rows` only on mount), not
 * recomputed from props on every keystroke: callers commit a *filtered*
 * map/array upstream (dropping blank rows) so the compose file never gets
 * an empty key, but that filtered echo must not be allowed to immediately
 * erase the blank row a user just added via "Add" before they've typed
 * anything into it. Callers should pass a `key` (e.g. the selected node's
 * id) so switching context remounts this with a fresh initial value
 * instead of stale rows leaking across selections. */
export function KeyValueRows({ rows: initialRows, labelA, labelB, disabled, onChange, addLabel }: {
  rows: KVRow[];
  labelA: string;
  labelB: string;
  disabled: boolean;
  onChange: (rows: KVRow[]) => void;
  addLabel?: string;
}) {
  const [rows, setRows] = useState<KVRow[]>(initialRows);

  function commit(next: KVRow[]) {
    setRows(next);
    onChange(next);
  }
  function update(i: number, patch: Partial<KVRow>) {
    commit(rows.map((row, idx) => (idx === i ? { ...row, ...patch } : row)));
  }
  function remove(i: number) {
    commit(rows.filter((_, idx) => idx !== i));
  }
  function add() {
    commit([...rows, { a: "", b: "" }]);
  }

  return <div>
    {rows.length === 0 && <p className="m-0 text-muted">None defined.</p>}
    {rows.map((row, i) => <div key={i} className="flex items-center gap-2 border-b border-linesoft py-1.5 last:border-0">
      <input aria-label={labelA} disabled={disabled} value={row.a} onChange={(e) => update(i, { a: e.target.value })} placeholder={labelA} className={rowInput} />
      <input aria-label={labelB} disabled={disabled} value={row.b} onChange={(e) => update(i, { b: e.target.value })} placeholder={labelB} className={rowInput} />
      <button type="button" title="Remove" aria-label={`Remove ${labelA.toLowerCase()} row`} disabled={disabled} onClick={() => remove(i)} className="rounded p-1 text-muted hover:bg-paper hover:text-fail disabled:opacity-45"><Trash2 size={13} /></button>
    </div>)}
    <button type="button" disabled={disabled} className="mt-1 text-hull hover:underline" onClick={add}><Plus size={12} className="mr-1 inline" />{addLabel ?? "Add"}</button>
  </div>;
}

export function mapToRows(m: Record<string, string> | undefined): KVRow[] {
  return Object.entries(m ?? {}).map(([a, b]) => ({ a, b }));
}

export function rowsToMap(rows: KVRow[]): Record<string, string> {
  return Object.fromEntries(rows.filter((r) => r.a).map((r) => [r.a, r.b]));
}
