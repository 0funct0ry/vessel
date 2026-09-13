import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import CodeMirror from "@uiw/react-codemirror";
import { yaml as yamlLang } from "@codemirror/lang-yaml";
import { Copy, Eye, FolderOpen, Layers, Pencil, Play, Plus, RotateCw, ScrollText, Square, Trash2, Upload } from "lucide-react";
import { Can } from "../auth/Can";
import { Button } from "../components/ui/Button";
import { EmptyState } from "../components/ui/EmptyState";
import { Modal } from "../components/ui/Modal";
import { useToast } from "../components/ui/Toast";
import { EnvVarRow, type EnvRow } from "../components/EnvVarRow";
import { StackGraph } from "./stackGraph";
import { api } from "../lib/api";
import { detectVariables, parseEnvContent, serializeEnvContent } from "../lib/composeVars";
import { streamSSE } from "../lib/pullStream";
import { useSSE } from "../lib/sse";
import type { Stack, StackEvent, StackLogLine, StackStatus, StackWarning } from "../types/api";

const inputClass = "mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 text-[13px]";
const iconAction = "rounded p-1 text-muted hover:bg-paper hover:text-text disabled:cursor-not-allowed disabled:opacity-45";
const dangerIconAction = `${iconAction} hover:bg-fail/10 hover:text-fail`;
const stackName = /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$/;
const headerIconBtn = "rounded p-1.5 text-muted hover:bg-paper hover:text-text";

const STARTER_COMPOSE = `services:
  web:
    image: nginx:1.27-alpine
    ports:
      - "8080:80"
`;

const STATUS_STYLE: Record<StackStatus, string> = {
  running: "border-[#BFD5CE] text-run",
  partial: "border-[#E2D3A8] text-pause",
  stopped: "border-line text-muted",
  not_deployed: "border-line text-muted",
};

function StatusPill({ status }: { status: StackStatus }) {
  return <span className={`rounded-sm border px-1.5 py-0.5 font-mono text-[11px] ${STATUS_STYLE[status] ?? STATUS_STYLE.stopped}`}>{status.replace("_", " ")}</span>;
}

async function copyToClipboard(text: string, label: string, push: (msg: string, kind?: "error") => void) {
  try { await navigator.clipboard.writeText(text); push(`Copied ${label}.`); }
  catch { push(`Could not copy ${label}. Select it and copy manually.`, "error"); }
}

function Warnings({ items, parseError }: { items: StackWarning[]; parseError?: string }) {
  if (parseError) return <p className="mt-2 mb-0 border-l-2 border-fail bg-paper p-2 text-[12px] text-fail">{parseError}</p>;
  if (!items.length) return null;
  return <ul className="mt-2 mb-0 list-none border-l-2 border-pause bg-paper p-2 pl-3 text-[12px]">{items.map((warning, index) => <li key={index}><span className="font-mono text-muted">{warning.kind}</span> — {warning.message}</li>)}</ul>;
}

function readFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error ?? new Error("could not read file"));
    reader.readAsText(file);
  });
}

/** Create/edit modal: a full-size two-pane editor — the compose file on the
 * left, a structured environment-variables panel on the right that tracks
 * every ${VAR} the file references as the user types. A second "Graph" tab
 * (M17.6.1) offers a two-way node-graph view of the same compose file. */
function StackEditorModal({ existing, close, onDeploy }: { existing?: Stack; close: () => void; onDeploy: (name: string, action: "up" | "redeploy") => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const navigate = useNavigate();
  const [name, setName] = useState(existing?.name ?? "");
  const [composeYAML, setComposeYAML] = useState(existing?.compose_yaml ?? STARTER_COMPOSE);
  const [envRows, setEnvRows] = useState<EnvRow[]>(() => Object.entries(parseEnvContent(existing?.env_content ?? "")).map(([key, value]) => ({ key, value })));
  const [warnings, setWarnings] = useState<StackWarning[] | null>(existing ? existing.warnings : null);
  const [parseError, setParseError] = useState<string | undefined>(existing?.parse_error);
  const [busy, setBusy] = useState(false);
  const composeFileInput = useRef<HTMLInputElement>(null);
  const envFileInput = useRef<HTMLInputElement>(null);
  const valid = stackName.test(name) && composeYAML.trim().length > 0;
  const [modalTab, setModalTab] = useState<"editor" | "graph">("editor");

  const detected = useMemo(() => detectVariables(composeYAML), [composeYAML]);

  // As the user types, every newly-referenced ${VAR} gets its own row —
  // pre-filled with its ${VAR:-default} when it has one — without disturbing
  // values already typed for variables still present.
  useEffect(() => {
    setEnvRows((current) => {
      const known = new Set(current.map((row) => row.key));
      const additions = detected.filter((v) => !known.has(v.name)).map((v) => ({ key: v.name, value: v.fallback ?? "" }));
      return additions.length ? [...current, ...additions] : current;
    });
  }, [detected]);

  const kindByName = useMemo(() => new Map(detected.map((v) => [v.name, v.kind])), [detected]);
  const counts = useMemo(() => {
    const out = { required: 0, optional: 0, error: 0 };
    for (const v of detected) out[v.kind]++;
    return out;
  }, [detected]);

  const envContent = useMemo(() => serializeEnvContent(envRows), [envRows]);
  const dirty = composeYAML !== (existing?.compose_yaml ?? STARTER_COMPOSE) || envContent !== (existing?.env_content ?? "") || (!existing && name !== "");

  function updateRow(index: number, row: EnvRow) { setEnvRows((current) => current.map((r, i) => (i === index ? row : r))); }
  function removeRow(index: number) { setEnvRows((current) => current.filter((_, i) => i !== index)); }
  function addRow() { setEnvRows((current) => [...current, { key: "", value: "", custom: true }]); }
  function clearRows() { setEnvRows([]); }

  async function loadEnvFile(file: File) {
    const loaded = parseEnvContent(await readFileAsText(file));
    setEnvRows((current) => {
      const merged = new Map(current.map((row) => [row.key, row] as const));
      for (const [key, value] of Object.entries(loaded)) merged.set(key, { key, value, custom: !kindByName.has(key) });
      return [...merged.values()];
    });
    push(`Loaded ${Object.keys(loaded).length} variable(s) from ${file.name}.`);
  }

  async function loadComposeFile(file: File) { setComposeYAML(await readFileAsText(file)); push(`Loaded ${file.name}.`); }

  function validate() {
    const problems: string[] = [];
    if (!/^\s*services\s*:/m.test(composeYAML)) problems.push("no top-level `services:` key found");
    for (const v of detected) if (v.kind === "error" && !(envRows.find((row) => row.key === v.name)?.value)) problems.push(`${v.name} ${v.message}`);
    if (problems.length === 0) push(`Looks good — ${detected.length} variable(s) detected, ${counts.error} error(s).`);
    else push(problems.join("; "), "error");
  }

  // Save is also the authoritative validation path: create/update parse the
  // compose file server-side and return the warnings, so the editor and the
  // deploy engine can never disagree about what a file means.
  async function save(andDeploy: boolean) {
    if (!valid) return;
    setBusy(true); setParseError(undefined);
    try {
      const saved = existing
        ? await api.put<Stack>(`/stacks/${encodeURIComponent(existing.name)}`, { compose_yaml: composeYAML, env_content: envContent })
        : await api.post<Stack>("/stacks", { name, compose_yaml: composeYAML, env_content: envContent });
      setWarnings(saved.warnings);
      await queryClient.invalidateQueries({ queryKey: ["stacks"] });
      await queryClient.invalidateQueries({ queryKey: ["stack", saved.name] });
      const noted = saved.warnings.length ? ` (${saved.warnings.length} warning(s))` : "";
      push(`${existing ? "Saved" : "Created stack"} ${saved.name}${noted}.`);
      close();
      if (andDeploy) onDeploy(saved.name, existing ? "redeploy" : "up");
      else if (!existing) navigate(`/stacks/${encodeURIComponent(saved.name)}`);
    } catch (err) {
      setParseError(err instanceof Error ? err.message : "could not save");
      setBusy(false);
    }
  }

  return <Modal
    title={existing ? `Edit compose stack ${existing.name}` : "Create compose stack"}
    subtitle={existing ? "Update the stored compose file and environment" : "Create a new Docker Compose stack"}
    icon={<Layers size={20} className="text-hull" />}
    close={close} busy={busy} fullscreen maximizable
  >
    <form onSubmit={(e: FormEvent) => { e.preventDefault(); void save(false); }} className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-1 border-b border-line px-5">
        <button type="button" onClick={() => setModalTab("editor")} className={`px-3 py-2.5 text-[13px] ${modalTab === "editor" ? "border-b-2 border-hull font-medium text-text" : "text-muted"}`}>Editor</button>
        <button type="button" onClick={() => setModalTab("graph")} disabled={!valid} title={!valid ? "Enter a valid name and compose file first" : undefined} className={`px-3 py-2.5 text-[13px] disabled:opacity-50 ${modalTab === "graph" ? "border-b-2 border-hull font-medium text-text" : "text-muted"}`}>Graph</button>
      </div>

      {!existing && <div className="border-b border-line px-5 py-3">
        <label className="block text-[13px]">Stack name<input autoFocus disabled={busy} value={name} onChange={(e) => setName(e.target.value)} placeholder="my-stack" className={`${inputClass} max-w-xs`} /></label>
        {name && !stackName.test(name) && <p className="mb-0 mt-1 text-[12px] text-fail">Use 1–63 letters, digits, dots, underscores, or hyphens; start with a letter or digit.</p>}
      </div>}

      {modalTab === "graph" && <StackGraph stackKey={existing?.name ?? name} composeYAML={composeYAML} onComposeYAMLChange={setComposeYAML} busy={busy} />}

      <div className={`grid min-h-0 flex-1 grid-cols-2 divide-x divide-line ${modalTab === "graph" ? "hidden" : ""}`}>
        <div className="flex min-h-0 flex-col">
          <div className="flex items-center gap-2 border-b border-linesoft px-4 py-2 text-[12.5px]">
            <span className="text-muted">Compose file</span>
            <span className="flex-1" />
            <button type="button" title="Load from file" aria-label="Load compose file" onClick={() => composeFileInput.current?.click()} className={headerIconBtn}><FolderOpen size={14} /></button>
            <button type="button" title="Copy" aria-label="Copy compose file" onClick={() => void copyToClipboard(composeYAML, "the compose file", push)} className={headerIconBtn}><Copy size={14} /></button>
            <input ref={composeFileInput} type="file" accept=".yml,.yaml,text/yaml" hidden onChange={(e) => { const file = e.target.files?.[0]; if (file) void loadComposeFile(file); e.target.value = ""; }} />
          </div>
          <div className="flex items-center justify-end gap-2 border-b border-linesoft px-4 py-1.5">
            <Button type="button" onClick={validate}>Validate</Button>
          </div>
          <div className="min-h-0 flex-1 overflow-auto">
            <CodeMirror aria-label="Compose file" value={composeYAML} onChange={setComposeYAML} extensions={[yamlLang()]} theme="light" basicSetup={{ foldGutter: false }} height="100%" style={{ height: "100%", fontSize: "12.5px" }} editable={!busy} />
          </div>
        </div>

        <div className="flex min-h-0 flex-col">
          <div className="flex items-center gap-2 border-b border-linesoft px-4 py-2 text-[12.5px]">
            <span className="text-muted">Environment variables</span>
            {counts.required > 0 && <span className="rounded-sm border border-[#BFD5CE] px-1 py-0.5 font-mono text-[10px] text-run">{counts.required} required</span>}
            {counts.optional > 0 && <span className="rounded-sm border border-line px-1 py-0.5 font-mono text-[10px] text-muted">{counts.optional} optional</span>}
            {counts.error > 0 && <span className="rounded-sm border border-fail/40 px-1 py-0.5 font-mono text-[10px] text-fail">{counts.error} w/ error</span>}
            <span className="flex-1" />
            <button type="button" title="Load .env file" aria-label="Load environment file" onClick={() => envFileInput.current?.click()} className={headerIconBtn}><Upload size={14} /></button>
            <button type="button" title="Add variable" aria-label="Add variable" onClick={addRow} className={headerIconBtn}><Plus size={14} /></button>
            <button type="button" title="Clear all" aria-label="Clear all variables" disabled={envRows.length === 0} onClick={clearRows} className={dangerIconAction}><Trash2 size={14} /></button>
            <input ref={envFileInput} type="file" accept=".env,text/plain" hidden onChange={(e) => { const file = e.target.files?.[0]; if (file) void loadEnvFile(file); e.target.value = ""; }} />
          </div>
          <p className="m-0 border-b border-linesoft bg-paper px-4 py-1.5 text-[11px] text-muted">
            <code>{"${VAR}"}</code> required &nbsp; <code>{"${VAR:-default}"}</code> optional &nbsp; <code>{"${VAR:?error}"}</code> required w/ error
          </p>
          <div className="min-h-0 flex-1 overflow-auto px-4 py-2">
            {envRows.length === 0
              ? <div className="flex h-full flex-col items-center justify-center text-center text-[13px] text-muted">
                  <p className="m-0">No environment variables defined.</p>
                  <button type="button" onClick={addRow} className="mt-1 text-hull hover:underline">+ Add your first variable</button>
                </div>
              : envRows.map((row, index) => <EnvVarRow key={index} row={row} kind={kindByName.get(row.key)} disabled={busy} onChange={(next) => updateRow(index, next)} onRemove={() => removeRow(index)} />)}
          </div>
        </div>
      </div>

      {(warnings?.length || parseError) ? <div className="px-5 pt-2"><Warnings items={warnings ?? []} parseError={parseError} /></div> : null}

      <div className="flex items-center gap-2 border-t border-line px-5 py-3">
        <span className="text-[12px] text-muted">{dirty ? "Unsaved changes" : "No changes"}</span>
        <span className="flex-1" />
        <Button type="button" disabled={busy} onClick={close}>Cancel</Button>
        <Button type="submit" disabled={!valid || busy}>{existing ? "Save" : "Create"}</Button>
        <Button type="button" variant="primary" disabled={!valid || busy} onClick={() => void save(true)}>{existing ? "Save & redeploy" : "Create & Start"}</Button>
      </div>
    </form>
  </Modal>;
}

const PHASE_ICON: Record<string, string> = { creating: "·", created: "+", starting: "·", started: "✓", removing: "·", removed: "−", exists: "=", skipped: "~", error: "✗", done: "✓", pulling: "↓", pulled: "✓" };

/** Deploy console: one line per engine step, kept open when a step fails. */
function DeployConsoleModal({ name, action, volumes, close }: { name: string; action: "up" | "down" | "redeploy"; volumes?: boolean; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast();
  const [steps, setSteps] = useState<StackEvent[]>([]);
  const [failure, setFailure] = useState<string | null>(null);
  const [busy, setBusy] = useState(true);
  const log = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // StrictMode's dev-mode double-invoke mounts this effect, tears it down,
    // then mounts it again — all synchronously, before any timer fires. A
    // one-shot POST (deploy) can't just re-run on the second mount like a
    // GET can: firing it from the first (phantom) mount and guarding the
    // second with a ref means the phantom's own cleanup aborts the only
    // request that was ever made, and the guard stops the second, real mount
    // from retrying — the deploy silently never happens. Deferring the
    // actual start past a 0ms timeout sidesteps this: the phantom mount's
    // timer is cleared by its cleanup before it ever fires, so only the
    // surviving mount's timer goes off, exactly once.
    const ac = new AbortController();
    const path = `/stacks/${encodeURIComponent(name)}/${action}${action === "down" && volumes ? "?volumes=true" : ""}`;
    const timer = setTimeout(() => {
      void (async () => {
        try {
          await streamSSE(path, "", {}, (event, data) => {
            if (event === "step") setSteps((current) => [...current, data as StackEvent]);
            if (event === "error") setFailure(String((data as { message?: string }).message ?? "stream closed"));
          }, ac.signal);
        } catch (err) {
          if (ac.signal.aborted) return; // real unmount, not a failure to report
          setFailure(err instanceof Error ? err.message : "deploy failed");
        } finally {
          setBusy(false);
          void queryClient.invalidateQueries({ queryKey: ["stacks"] });
          void queryClient.invalidateQueries({ queryKey: ["stack", name] });
          void queryClient.invalidateQueries({ queryKey: ["containers"] });
        }
      })();
    }, 0);
    return () => { clearTimeout(timer); ac.abort(); };
  }, [name, action, volumes, queryClient]);

  useEffect(() => { log.current?.scrollTo({ top: log.current.scrollHeight }); }, [steps]);

  const errors = steps.filter((step) => step.phase === "error");
  const raw = [...errors.map((step) => `${step.name}: ${step.detail}`), ...(failure ? [failure] : [])].join("\n");

  return <Modal title={`${action === "up" ? "Deploy" : action === "down" ? "Stop" : "Redeploy"} ${name}`} close={close} busy={busy}>
    <div ref={log} className="mt-4 max-h-[50vh] overflow-auto rounded border border-line bg-ink p-3 font-mono text-[12px] text-[#D7E7EA]">
      {steps.length === 0 && !failure ? <div className="text-[#7F9BA6]">Contacting Docker…</div> : steps.map((step, index) => (
        <div key={index} className={step.phase === "error" ? "text-fail" : undefined}>
          <span className="inline-block w-4">{PHASE_ICON[step.phase] ?? "·"}</span>
          <span className="text-[#7F9BA6]">{step.kind}</span> {step.name} <span className="text-[#7F9BA6]">{step.phase}</span>
          {step.detail && <span className="text-[#9FB6BF]"> — {step.detail}</span>}
        </div>
      ))}
      {failure && <div className="text-fail">✗ {failure}</div>}
    </div>
    <div className="mt-4 flex items-center gap-2">
      {raw && <><span className="text-[12px] text-fail">{errors.length || 1} step(s) failed.</span><Button onClick={() => void copyToClipboard(raw, "the daemon error", push)}><Copy size={14} className="mr-1 inline" />Copy error</Button></>}
      <span className="flex-1" />
      <Button variant={raw ? undefined : "primary"} disabled={busy} onClick={close}>Close</Button>
    </div>
  </Modal>;
}

/** Confirms Down and lets the user opt into also removing the stack's named
 * volumes — mirrors the same checkbox the Remove modal already offers, since
 * Down is the more common point at which someone wants a clean slate (e.g.
 * before changing a database password that's already baked into a volume). */
function DownStackModal({ name, close, onConfirm }: { name: string; close: () => void; onConfirm: (volumes: boolean) => void }) {
  const [volumes, setVolumes] = useState(false);
  return <Modal title={`Stop ${name}?`} close={close}>
    <p className="mt-3 text-[13px] text-muted">Stops and removes every container in this stack. The network is removed too; named volumes are kept unless you ask below.</p>
    <label className="mt-4 flex items-center gap-2 text-[13px]"><input type="checkbox" checked={volumes} onChange={(e) => setVolumes(e.target.checked)} /> Also remove the stack&rsquo;s named volumes</label>
    <div className="mt-5 flex justify-end gap-2"><Button onClick={close}>Cancel</Button><Button variant="danger" onClick={() => onConfirm(volumes)}>Down</Button></div>
  </Modal>;
}

function RemoveStackModal({ stack, close }: { stack: Stack; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const navigate = useNavigate();
  const [volumes, setVolumes] = useState(false); const [busy, setBusy] = useState(false);
  async function remove() {
    setBusy(true);
    try {
      await api.delete(`/stacks/${encodeURIComponent(stack.name)}${volumes ? "?volumes=true" : ""}`);
      await queryClient.invalidateQueries({ queryKey: ["stacks"] });
      await queryClient.invalidateQueries({ queryKey: ["containers"] });
      push(`Removed stack ${stack.name}.`); close(); navigate("/stacks");
    } catch (err) { push(`Could not remove ${stack.name}: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); }
  }
  return <Modal title={`Remove ${stack.name}?`} close={close} busy={busy}>
    <p className="mt-3 text-[13px] text-muted">This deletes the stored compose file. Stop the stack first — a running stack cannot be removed.</p>
    <label className="mt-4 flex items-center gap-2 text-[13px]"><input type="checkbox" checked={volumes} onChange={(e) => setVolumes(e.target.checked)} /> Remove the stack&rsquo;s named volumes too</label>
    <div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void remove()}>Remove</Button></div>
  </Modal>;
}

type Deploy = { name: string; action: "up" | "down" | "redeploy"; volumes?: boolean };

function StackRowActions({ stack, onEdit, onLogs, onDeploy, onDown, onRemove }: { stack: Stack; onEdit: () => void; onLogs: () => void; onDeploy: (action: "up" | "redeploy") => void; onDown: () => void; onRemove: () => void }) {
  const navigate = useNavigate();
  const up = stack.status === "running" || stack.status === "partial";
  return <span className="flex items-center justify-end gap-0.5">
    <button type="button" title="View details" aria-label={`View ${stack.name}`} onClick={() => navigate(`/stacks/${encodeURIComponent(stack.name)}`)} className={iconAction}><Eye size={15} /></button>
    <Can do="stacks.update"><button type="button" title="Edit" aria-label={`Edit ${stack.name}`} onClick={onEdit} className={iconAction}><Pencil size={15} /></button></Can>
    <Can do="stacks.logs"><button type="button" title="View logs" aria-label={`Logs for ${stack.name}`} disabled={stack.container_count === 0} onClick={onLogs} className={iconAction}><ScrollText size={15} /></button></Can>
    <Can do="stacks.up"><button type="button" title="Start" aria-label={`Start ${stack.name}`} disabled={stack.status === "running"} onClick={() => onDeploy("up")} className={iconAction}><Play size={15} /></button></Can>
    <Can do="stacks.redeploy"><button type="button" title="Redeploy" aria-label={`Redeploy ${stack.name}`} onClick={() => onDeploy("redeploy")} className={iconAction}><RotateCw size={15} /></button></Can>
    <Can do="stacks.down"><button type="button" title="Down" aria-label={`Stop ${stack.name}`} disabled={stack.container_count === 0} onClick={onDown} className={iconAction}><Square size={15} /></button></Can>
    <Can do="stacks.remove"><button type="button" title={up ? "Stop the stack before removing it" : "Remove"} aria-label={`Remove ${stack.name}`} disabled={up} onClick={onRemove} className={dangerIconAction}><Trash2 size={15} /></button></Can>
  </span>;
}

export function StacksPage() {
  const [q, setQ] = useState("");
  const [create, setCreate] = useState(false);
  const [edit, setEdit] = useState<Stack | null>(null);
  const [remove, setRemove] = useState<Stack | null>(null);
  const [logs, setLogs] = useState<string | null>(null);
  const [deploy, setDeploy] = useState<Deploy | null>(null);
  const [downConfirm, setDownConfirm] = useState<string | null>(null);
  const stacks = useQuery({ queryKey: ["stacks", q], queryFn: () => api.get<Stack[]>(q.trim() ? `/stacks?q=${encodeURIComponent(q.trim())}` : "/stacks") });
  const rows = stacks.data ?? [];

  return <section>
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter stacks" aria-label="Filter stacks" className="min-w-[200px] rounded border border-line bg-panel px-2 py-1.5 text-[13px]" />
      <span className="flex-1" />
      <Button onClick={() => void stacks.refetch()}>Refresh</Button>
      <Can do="stacks.create"><Button variant="primary" onClick={() => setCreate(true)}>Create stack</Button></Can>
    </div>
    <div className="overflow-x-auto rounded border border-line bg-panel">
      {stacks.isLoading ? <EmptyState title="Loading stacks" action="Contacting Docker…" />
        : rows.length === 0 ? <EmptyState title="No stacks yet" action="Create a stack from a compose file, or broaden the current filter." />
        : <table className="w-full min-w-[820px] border-collapse text-[13px]">
          <thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted"><tr><th className="px-3">Name</th><th className="px-3">Status</th><th className="px-3 text-right">Services</th><th className="px-3 text-right">Containers</th><th className="px-3">Source</th><th className="w-[220px] px-3" /></tr></thead>
          <tbody>{rows.map((stack) => <tr key={stack.id} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper">
            <td className="px-3 font-medium"><Link to={`/stacks/${encodeURIComponent(stack.name)}`}>{stack.name}</Link></td>
            <td className="px-3"><StatusPill status={stack.status} /></td>
            <td className="px-3 text-right font-mono text-[12px]">{stack.service_count}</td>
            <td className="px-3 text-right font-mono text-[12px]">{stack.container_count}</td>
            <td className="px-3"><span className="rounded-sm border border-line px-1 font-mono text-[11px] text-muted">{stack.source}</span></td>
            <td className="px-3 text-right"><span className="opacity-0 group-hover:opacity-100 focus-within:opacity-100"><StackRowActions stack={stack} onEdit={() => setEdit(stack)} onLogs={() => setLogs(stack.name)} onDeploy={(action) => setDeploy({ name: stack.name, action })} onDown={() => setDownConfirm(stack.name)} onRemove={() => setRemove(stack)} /></span></td>
          </tr>)}</tbody>
        </table>}
    </div>
    {create && <StackEditorModal close={() => setCreate(false)} onDeploy={(name, action) => setDeploy({ name, action })} />}
    {edit && <StackEditorModal existing={edit} close={() => setEdit(null)} onDeploy={(name, action) => setDeploy({ name, action })} />}
    {remove && <RemoveStackModal stack={remove} close={() => setRemove(null)} />}
    {downConfirm && <DownStackModal name={downConfirm} close={() => setDownConfirm(null)} onConfirm={(volumes) => { setDeploy({ name: downConfirm, action: "down", volumes }); setDownConfirm(null); }} />}
    {deploy && <DeployConsoleModal name={deploy.name} action={deploy.action} volumes={deploy.volumes} close={() => setDeploy(null)} />}
    {logs && <StackLogsModal name={logs} close={() => setLogs(null)} />}
  </section>;
}

function StackLogsModal({ name, close }: { name: string; close: () => void }) {
  return <Modal title={`${name} logs`} close={close} wide><StackLogs name={name} /></Modal>;
}

/**
 * Merged per-service log stream. One SSE connection covers the whole stack;
 * the GET route means this can use the shared EventSource hook rather than the
 * POST-initiated stream the deploy console needs.
 */
function StackLogs({ name }: { name: string }) {
  const [lines, setLines] = useState<StackLogLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const box = useRef<HTMLDivElement>(null);

  const status = useSSE(`/stacks/${encodeURIComponent(name)}/logs`, {
    step: (data) => { try { const line = JSON.parse(data) as StackLogLine; setLines((current) => [...current.slice(-2000), line]); } catch { /* malformed frame */ } },
    error: (data) => { try { setError(String((JSON.parse(data) as { message?: string }).message ?? "stream closed")); } catch { setError("stream closed"); } },
  }, { reconnect: false });

  useEffect(() => { box.current?.scrollTo({ top: box.current.scrollHeight }); }, [lines]);

  return <div>
    <div ref={box} className="mt-4 max-h-[60vh] min-h-[200px] overflow-auto rounded border border-line bg-ink p-3 font-mono text-[12px] text-[#D7E7EA]">
      {lines.length === 0 && !error && <div className="text-[#7F9BA6]">{status === "closed" ? "Stream closed." : "Waiting for output…"}</div>}
      {lines.map((line, index) => <div key={index} className={line.stream === "stderr" ? "text-[#F0B9B0]" : undefined}><span className="text-[#7F9BA6]">{line.service}</span> | {line.line}</div>)}
    </div>
    {error && <p className="mt-2 mb-0 text-[12px] text-fail">{error}</p>}
  </div>;
}

const STACK_TABS = ["services", "compose", "logs"] as const;
type StackTab = typeof STACK_TABS[number];

export function StackDetailPage() {
  const { name = "" } = useParams();
  const { push } = useToast();
  const [tab, setTab] = useState<StackTab>("services");
  const [edit, setEdit] = useState(false);
  const [remove, setRemove] = useState(false);
  const [deploy, setDeploy] = useState<Deploy | null>(null);
  const [downConfirm, setDownConfirm] = useState(false);
  const stack = useQuery({ queryKey: ["stack", name], queryFn: () => api.get<Stack>(`/stacks/${encodeURIComponent(name)}`), refetchInterval: 5000 });
  const item = stack.data;

  if (stack.isLoading) return <EmptyState title="Loading stack" action="Contacting Docker…" />;
  if (!item) return <EmptyState title="Stack not found" action="Return to the stacks list and refresh." />;
  const up = item.status === "running" || item.status === "partial";

  return <section>
    <div className="mb-3 flex flex-wrap items-center gap-3">
      <h1 className="m-0 text-lg">{item.name}</h1><StatusPill status={item.status} />
      <span className="flex-1" />
      <Can do="stacks.up"><Button disabled={item.status === "running"} onClick={() => setDeploy({ name: item.name, action: "up" })}>Start</Button></Can>
      <Can do="stacks.redeploy"><Button onClick={() => setDeploy({ name: item.name, action: "redeploy" })}>Redeploy</Button></Can>
      <Can do="stacks.down"><Button disabled={item.container_count === 0} onClick={() => setDownConfirm(true)}>Down</Button></Can>
      <Can do="stacks.update"><Button onClick={() => setEdit(true)}>Edit</Button></Can>
      <Can do="stacks.remove"><Button variant="danger" disabled={up} title={up ? "Stop the stack before removing it." : undefined} onClick={() => setRemove(true)}>Remove</Button></Can>
    </div>
    <Warnings items={item.warnings} parseError={item.parse_error} />
    <div role="tablist" className="my-4 flex gap-1 border-b border-line">{STACK_TABS.map((key) => <button key={key} role="tab" aria-selected={tab === key} onClick={() => setTab(key)} className={`px-3 py-2 text-[13px] capitalize ${tab === key ? "border-b-2 border-hull font-medium text-text" : "text-muted"}`}>{key}</button>)}</div>

    {tab === "services" && <div className="overflow-x-auto rounded border border-line bg-panel">
      <table className="w-full min-w-[700px] border-collapse text-[13px]">
        <thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted"><tr><th className="px-3">Service</th><th className="px-3">Image</th><th className="px-3">Container</th><th className="px-3">State</th><th className="px-3">Status</th></tr></thead>
        <tbody>{item.services.map((service) => <tr key={service.name} className="h-row border-b border-linesoft last:border-0 hover:bg-paper">
          <td className="px-3 font-medium">{service.name}</td>
          <td className="max-w-[240px] truncate px-3 font-mono text-[12px]">{service.image}</td>
          <td className="px-3 font-mono text-[12px]">{service.container_id ? <Link to={`/containers/${encodeURIComponent(service.container_id)}`}>{service.container_name}</Link> : "—"}</td>
          <td className="px-3 font-mono text-[12px]">{service.state}</td>
          <td className="px-3 font-mono text-[12px] text-muted">{service.status || "—"}</td>
        </tr>)}</tbody>
      </table>
      {item.services.length === 0 && <EmptyState title="This compose file declares no services" action="Edit the stack and add a service." />}
    </div>}

    {tab === "compose" && <div>
      <div className="mb-2 flex items-center gap-2">
        <span className="text-[13px] text-muted">Stored compose file</span><span className="flex-1" />
        <Button onClick={() => void copyToClipboard(item.compose_yaml, "the compose file", push)}><Copy size={14} className="mr-1 inline" />Copy</Button>
        <Can do="stacks.update"><Button onClick={() => setEdit(true)}><Pencil size={14} className="mr-1 inline" />Edit</Button></Can>
      </div>
      <pre className="max-h-[60vh] overflow-auto rounded border border-line bg-ink p-4 text-[12px] text-[#D7E7EA]">{item.compose_yaml}</pre>
      {item.env_content && <><div className="mb-2 mt-4 text-[13px] text-muted">Environment</div><pre className="max-h-[30vh] overflow-auto rounded border border-line bg-ink p-4 text-[12px] text-[#D7E7EA]">{item.env_content}</pre></>}
    </div>}

    {tab === "logs" && (item.container_count === 0
      ? <EmptyState title="No containers to stream" action="Start the stack, then return to this tab." />
      : <StackLogs name={item.name} />)}

    {edit && <StackEditorModal existing={item} close={() => setEdit(false)} onDeploy={(stackName, action) => setDeploy({ name: stackName, action })} />}
    {remove && <RemoveStackModal stack={item} close={() => setRemove(false)} />}
    {downConfirm && <DownStackModal name={item.name} close={() => setDownConfirm(false)} onConfirm={(volumes) => { setDeploy({ name: item.name, action: "down", volumes }); setDownConfirm(false); }} />}
    {deploy && <DeployConsoleModal name={deploy.name} action={deploy.action} volumes={deploy.volumes} close={() => setDeploy(null)} />}
  </section>;
}
