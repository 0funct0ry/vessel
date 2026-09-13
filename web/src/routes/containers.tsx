import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Pause, Pencil, Play, RotateCw, Square, Trash2 } from "lucide-react";
import { Can } from "../auth/Can";
import { LogViewer } from "../components/containers/LogViewer";
import { FileBrowser } from "../components/containers/FileBrowser";
import { CreateContainerModal } from "../components/containers/CreateContainerModal";
import { CommitContainerModal } from "../components/containers/CommitContainerModal";
import { Button } from "../components/ui/Button";
import { EmptyState } from "../components/ui/EmptyState";
import { Flag } from "../components/ui/Flag";
import { useToast } from "../components/ui/Toast";
import { api } from "../lib/api";
import { bytes, canStop, containerQuery, flagState, httpPort, uptime } from "../lib/containers";
import type { Container, ContainerDetail, Top } from "../types/api";

function RemoveDialog({ container, close, done }: { container: Container; close: () => void; done: () => void }) { const [volumes, setVolumes] = useState(false); const [typed, setTyped] = useState(""); const [busy, setBusy] = useState(false); const { push } = useToast(); const allowed = !volumes || typed === container.name; async function remove() { setBusy(true); try { await api.delete(`/containers/${encodeURIComponent(container.id)}?force=true${volumes ? "&volumes=true" : ""}`); push(`Removed ${container.name}.`); done(); close(); } catch (e) { push(`Could not remove ${container.name}: ${e instanceof Error ? e.message : "try again"}.`, "error"); setBusy(false); } } return <div role="dialog" aria-modal="true" aria-labelledby="remove-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4"><div className="w-full max-w-md rounded border border-line bg-panel p-5 shadow-lg"><h2 id="remove-title" className="m-0 text-lg">Remove {container.name}?</h2><p className="mt-2 text-[13px] text-muted">This stops and permanently removes the container.</p><label className="mt-4 flex items-center gap-2 text-[13px]"><input type="checkbox" checked={volumes} onChange={(e) => setVolumes(e.target.checked)} /> Remove anonymous volumes too</label>{volumes && <label className="mt-3 block text-[13px]">Type <b>{container.name}</b> to confirm<input autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} className="mt-1 block w-full rounded border border-line px-2 py-1" /></label>}<div className="mt-5 flex justify-end gap-2"><Button onClick={close}>Cancel</Button><Button variant="danger" disabled={!allowed || busy} onClick={() => void remove()}>Remove</Button></div></div></div>; }

function BulkRemoveDialog({ containers, close, done }: { containers: Container[]; close: () => void; done: () => void }) {
  const [volumes, setVolumes] = useState(false); const [typed, setTyped] = useState(""); const [busy, setBusy] = useState(false); const { push } = useToast();
  const allowed = typed.trim().toUpperCase() === "REMOVE";
  async function remove() {
    setBusy(true);
    const results: string[] = [];
    for (const c of containers) {
      try { await api.delete(`/containers/${encodeURIComponent(c.id)}?force=true${volumes ? "&volumes=true" : ""}`); results.push(`${c.name}: removed`); } catch { results.push(`${c.name}: failed`); }
    }
    push(results.join(" · ")); done(); close();
  }
  return <div role="dialog" aria-modal="true" aria-labelledby="bulk-remove-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4">
    <div className="w-full max-w-md rounded border border-line bg-panel p-5 shadow-lg">
      <h2 id="bulk-remove-title" className="m-0 text-lg">Remove {containers.length} containers?</h2>
      <p className="mt-2 text-[13px] text-muted">{containers.map((c) => c.name).join(", ")}</p>
      <label className="mt-4 flex items-center gap-2 text-[13px]"><input type="checkbox" checked={volumes} onChange={(e) => setVolumes(e.target.checked)} /> Remove anonymous volumes too</label>
      <label className="mt-3 block text-[13px]">This removes {containers.length} containers — type REMOVE to confirm<input autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} className="mt-1 block w-full rounded border border-line px-2 py-1" /></label>
      <div className="mt-5 flex justify-end gap-2"><Button onClick={close}>Cancel</Button><Button variant="danger" disabled={!allowed || busy} onClick={() => void remove()}>Remove</Button></div>
    </div>
  </div>;
}

/** Compose writes the owning project onto every container it creates. */
function stackOf(c: Container): string | undefined { return c.labels?.["com.docker.compose.project"] || undefined; }

type LifecycleAction = "start" | "stop" | "restart" | "pause" | "unpause";

const iconAction = "rounded p-1 text-muted hover:bg-paper hover:text-text disabled:cursor-not-allowed disabled:opacity-45";
const dangerIconAction = `${iconAction} hover:bg-fail/10 hover:text-fail`;

function ContainerRowActions({ container: c, onLifecycle, onEdit, onRemove }: { container: Container; onLifecycle: (c: Container, action: LifecycleAction) => void; onEdit: (c: Container) => void; onRemove: (c: Container) => void }) {
  return <span className="inline-flex items-center gap-0.5">
    <Can do="containers.start"><button type="button" title="Start" aria-label={`Start ${c.name}`} disabled={c.state === "running" || c.state === "restarting"} onClick={() => onLifecycle(c, "start")} className={iconAction}><Play size={15} /></button></Can>
    <Can do="containers.pause">{c.state === "paused" ? <button type="button" title="Resume" aria-label={`Resume ${c.name}`} onClick={() => onLifecycle(c, "unpause")} className={iconAction}><Play size={15} /></button> : <button type="button" title={c.state !== "running" ? "Only a running container can be paused" : "Pause"} aria-label={`Pause ${c.name}`} disabled={c.state !== "running"} onClick={() => onLifecycle(c, "pause")} className={iconAction}><Pause size={15} /></button>}</Can>
    <Can do="containers.stop"><button type="button" title="Stop" aria-label={`Stop ${c.name}`} disabled={!canStop(c)} onClick={() => onLifecycle(c, "stop")} className={iconAction}><Square size={15} /></button></Can>
    <Can do="containers.restart"><button type="button" title="Restart" aria-label={`Restart ${c.name}`} onClick={() => onLifecycle(c, "restart")} className={iconAction}><RotateCw size={15} /></button></Can>
    <Can do="containers.recreate"><button type="button" title="Edit" aria-label={`Edit ${c.name}`} onClick={() => onEdit(c)} className={iconAction}><Pencil size={15} /></button></Can>
    <Can do="containers.remove"><button type="button" title="Remove" aria-label={`Remove ${c.name}`} onClick={() => onRemove(c)} className={dangerIconAction}><Trash2 size={15} /></button></Can>
  </span>;
}

const CONTAINER_TABS = ["overview", "logs", "stats", "console", "files", "processes", "security", "resources", "inspect"] as const;
type ContainerTab = typeof CONTAINER_TABS[number];

export function ContainerDetailPage() {
  const { id = "" } = useParams();
  const { push } = useToast();
  const [params, setParams] = useSearchParams();
  const requested = params.get("tab");
  const [tab, setTab] = useState<ContainerTab>(CONTAINER_TABS.includes(requested as ContainerTab) ? (requested as ContainerTab) : "overview");
  const [revealed, setRevealed] = useState<Set<string>>(new Set());
  const [commitOpen, setCommitOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const queryClient = useQueryClient();
  const detail = useQuery({ queryKey: ["container", id], queryFn: () => api.get<ContainerDetail>(`/containers/${encodeURIComponent(id)}`) });
  const c = detail.data;
  const processes = useQuery({ queryKey: ["container", id, "top"], queryFn: () => api.get<Top>(`/containers/${encodeURIComponent(id)}/top`), enabled: tab === "processes" && c?.state === "running", refetchInterval: 3000 });
  if (detail.isLoading) return <EmptyState title="Loading container" action="Contacting Docker…" />;
  if (!c) return <EmptyState title="Container not found" action="Return to the containers list and refresh." />;
  const env = c.env.map((entry) => { const at = entry.indexOf("="); return [at < 0 ? entry : entry.slice(0, at), at < 0 ? "" : entry.slice(at + 1)] as const; });
  const copy = async () => { try { await navigator.clipboard.writeText(JSON.stringify(c.raw, null, 2)); push("Inspect JSON copied."); } catch { push("Could not copy JSON. Select it and copy manually.", "error"); } };
  async function lifecycle(action: LifecycleAction) { try { await api.post(`/containers/${encodeURIComponent(c!.id)}/${action}`); await queryClient.invalidateQueries({ queryKey: ["container", id] }); await queryClient.invalidateQueries({ queryKey: ["containers"] }); } catch (e) { push(`Could not ${action} ${c!.name}: ${e instanceof Error ? e.message : "try again"}.`, "error"); } }
  const selectTab = (name: ContainerTab) => { setTab(name); setParams(name === "overview" ? {} : { tab: name }, { replace: true }); };
  return <section>
    <div className="mb-3 flex items-center gap-3">
      <Flag state={flagState(c.state)} /><h1 className="m-0 text-lg">{c.name}</h1><span className="font-mono text-[12px] text-muted">{c.id.slice(0, 12)}</span>
      <span className="flex-1" />
      <Can do="containers.pause">{c.state === "paused" ? <Button onClick={() => void lifecycle("unpause")}>Resume</Button> : <Button disabled={c.state !== "running"} title={c.state !== "running" ? "Only a running container can be paused" : undefined} onClick={() => void lifecycle("pause")}>Pause</Button>}</Can>
      <Can do="containers.recreate"><Button onClick={() => setEditing(true)}>Edit</Button></Can>
      <Can do="containers.commit"><Button onClick={() => setCommitOpen(true)}>Commit to image</Button></Can>
    </div>
    <div role="tablist" className="mb-4 flex gap-1 border-b border-line">{CONTAINER_TABS.map((name) => <button key={name} role="tab" aria-selected={tab === name} onClick={() => selectTab(name)} className={`px-3 py-2 text-[13px] capitalize ${tab === name ? "border-b-2 border-hull font-medium text-text" : "text-muted"}`}>{name}</button>)}</div>
    {tab === "overview" && <div className="grid gap-3 md:grid-cols-2"><Info title="Configuration" rows={[["Image", c.image], ["Command", c.command.join(" ") || "—"], ["Created", c.created], ["Restart policy", c.restart_policy || "—"], ["Health", c.health || "not configured"], ["Exit code", String(c.exit_code)]]} /><div className="rounded border border-line bg-panel p-4"><h2 className="m-0 mb-3 text-[14px]">Environment</h2><dl className="grid grid-cols-[minmax(100px,auto)_1fr] gap-x-4 gap-y-2 text-[12px]">{env.map(([key, value]) => <><dt key={`${key}-k`} className="font-mono text-muted">{key}</dt><dd key={`${key}-v`} className="min-w-0 break-all font-mono">{revealed.has(key) ? value : "••••••••••••"} <button className="ml-1 text-link underline" onClick={() => setRevealed((set) => new Set(set).add(key))}>reveal</button></dd></>)}</dl><p className="mb-0 mt-3 text-[12px] text-muted">Revealing a value writes an audit log line.</p></div><Info title="Mounts" rows={c.mounts.map((m) => [m.destination, `${m.name || m.source} · ${m.rw ? "rw" : "ro"}`])} /><Info title="Networks" rows={Object.entries(c.networks).map(([name, n]) => [name, n.ip_address || "—"])} /></div>}
    {tab === "logs" && <LogViewer containerID={c.id} />}
    {tab === "files" && <FileBrowser containerID={c.id} />}
    {tab === "processes" && <Processes running={c.state === "running"} data={processes.data} loading={processes.isLoading} />}
    {["stats", "console"].includes(tab) && <EmptyState title={`${tab[0].toUpperCase() + tab.slice(1)} is not built yet`} action={tab === "console" ? "The interactive terminal has not shipped yet." : "The full stats experience has not shipped yet."} />}
    {tab === "security" && <Info title="Security" rows={[["Privileged", pill(c.security.privileged)], ["Read-only rootfs", pill(c.security.readonly_rootfs)], ["User", c.security.user || "default"], ["User namespace mode", c.security.userns_mode || "default"], ["AppArmor profile", c.security.apparmor_profile || "default"]]} />}
    {tab === "resources" && <Info title="Resources" rows={[["CPU shares", c.resources.cpu_shares ? String(c.resources.cpu_shares) : "unlimited"], ["CPUs", c.resources.cpus ? c.resources.cpus.toFixed(2) : "unlimited"], ["Memory", c.resources.memory ? bytes(c.resources.memory) : "unlimited"], ["Memory + swap", c.resources.memory_swap > 0 ? bytes(c.resources.memory_swap) : "unlimited"], ["Memory reservation", c.resources.memory_reservation ? bytes(c.resources.memory_reservation) : "unlimited"], ["PIDs limit", c.resources.pids_limit > 0 ? String(c.resources.pids_limit) : "unlimited"], ["OOM kill disabled", pill(c.resources.oom_kill_disable)], ["CPU period", c.resources.cpu_period ? String(c.resources.cpu_period) : "default"], ["CPU quota", c.resources.cpu_quota ? String(c.resources.cpu_quota) : "default"], ["Cgroup parent", c.resources.cgroup_parent || "default"], ["Cgroup namespace mode", c.resources.cgroupns_mode || "default"]]} />}
    {tab === "inspect" && <div><div className="mb-2 flex items-center"><span className="text-[13px] text-muted">Full engine response</span><Button className="ml-auto" onClick={() => void copy()}>Copy JSON</Button></div><pre className="max-h-[65vh] overflow-auto rounded border border-line bg-ink p-4 text-[12px] text-[#D7E7EA]">{JSON.stringify(c.raw, null, 2)}</pre></div>}
    {commitOpen && <CommitContainerModal containerID={c.id} close={() => setCommitOpen(false)} />}
    {editing && <CreateContainerModal existing={c} close={() => setEditing(false)} />}
  </section>;
}

function pill(value: boolean) { return <span className={`inline-block rounded-full px-2 py-0.5 text-[11px] ${value ? "bg-fail/10 text-fail" : "bg-linesoft text-muted"}`}>{value ? "yes" : "no"}</span>; }
function Info({ title, rows }: { title: string; rows: readonly (readonly [string, unknown])[] }) { return <div className="rounded border border-line bg-panel p-4"><h2 className="m-0 mb-3 text-[14px]">{title}</h2>{rows.length ? <dl className="grid grid-cols-[minmax(140px,auto)_1fr] gap-x-4 gap-y-2 text-[12px]">{rows.map(([key, value]) => <><dt key={`${key}-k`} className="text-muted">{key}</dt><dd key={`${key}-v`} className="min-w-0 break-all font-mono">{value as React.ReactNode}</dd></>)}</dl> : <p className="text-[12px] text-muted">None</p>}</div>; }
function Processes({ running, data, loading }: { running: boolean; data?: Top; loading: boolean }) { if (!running) return <EmptyState title="Process list is only available for running containers." action="Start the container, then return to this tab." />; if (loading) return <EmptyState title="Loading processes" action="Requesting a docker top snapshot…" />; return <div><div className="mb-2 text-[13px] text-muted">Snapshot from <span className="font-mono">docker top</span> · refreshes every few seconds</div><div className="overflow-x-auto rounded border border-line bg-panel"><table className="w-full min-w-max border-collapse text-[13px] font-mono"><thead className="border-b border-line bg-paper text-left text-[11.5px] font-sans uppercase tracking-wide text-muted"><tr>{(data?.titles ?? []).map((title) => <th key={title} className="px-3 py-[7px]">{title}</th>)}</tr></thead><tbody>{(data?.processes ?? []).map((row, i) => <tr key={i} className="h-row border-b border-linesoft last:border-0 hover:bg-paper">{row.map((cell, j) => <td key={j} className="whitespace-nowrap px-3">{cell}</td>)}</tr>)}</tbody></table>{(data?.processes.length ?? 0) === 0 && <div className="p-8 text-center text-[13px] text-muted">No processes found.</div>}</div></div>; }

/** Containers list: create, edit-in-place, and per-row/bulk lifecycle actions. */
export function ContainersPage() {
  const queryClient = useQueryClient(); const { push } = useToast();
  const [q, setQ] = useState(""); const [status, setStatus] = useState("");
  const [creating, setCreating] = useState(false); const [remove, setRemove] = useState<Container | null>(null);
  const [bulkRemove, setBulkRemove] = useState<Container[] | null>(null);
  const [editTarget, setEditTarget] = useState<ContainerDetail | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [summary, setSummary] = useState<string | null>(null);
  const path = containerQuery({ all: true, q, status, sort: "name" });
  const containers = useQuery({ queryKey: ["containers", true, q, status, "name"], queryFn: () => api.get<Container[]>(path), refetchInterval: 3000 });
  const rows = containers.data ?? [];
  const allSelected = rows.length > 0 && rows.every((c) => selected.has(c.id));
  const toggle = (id: string) => setSelected((old) => { const next = new Set(old); if (next.has(id)) next.delete(id); else next.add(id); return next; });

  async function lifecycle(c: Container, action: LifecycleAction) { try { await api.post(`/containers/${encodeURIComponent(c.id)}/${action}`); await queryClient.invalidateQueries({ queryKey: ["containers"] }); } catch (e) { push(`Could not ${action} ${c.name}: ${e instanceof Error ? e.message : "try again"}.`, "error"); } }

  async function bulk(action: "stop" | "pause" | "restart", eligible: (c: Container) => boolean) {
    const targets = rows.filter((c) => selected.has(c.id) && eligible(c));
    const results: string[] = [];
    for (const c of targets) {
      try { await api.post(`/containers/${encodeURIComponent(c.id)}/${action}`); results.push(`${c.name}: ${action === "stop" ? "stopped" : action === "pause" ? "paused" : "restarted"}`); } catch { results.push(`${c.name}: failed`); }
    }
    setSummary(results.join(" · ")); setSelected(new Set()); await queryClient.invalidateQueries({ queryKey: ["containers"] });
  }

  async function openEdit(c: Container) {
    try { setEditTarget(await api.get<ContainerDetail>(`/containers/${encodeURIComponent(c.id)}`)); } catch (e) { push(`Could not load ${c.name} for editing: ${e instanceof Error ? e.message : "try again"}.`, "error"); }
  }

  return <section>
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter by name, image or port" aria-label="Filter containers" className="min-w-[230px] rounded border border-line bg-panel px-2 py-1.5 text-[13px]" />
      <select value={status} onChange={(e) => setStatus(e.target.value)} aria-label="Filter state" className="rounded border border-line bg-panel px-2 py-1.5 text-[13px]"><option value="">All states</option><option value="running">Running</option><option value="paused">Paused</option><option value="exited">Stopped</option></select>
      <span className="flex-1" />
      <Can do="containers.create"><Button variant="primary" onClick={() => setCreating(true)}>Create container</Button></Can>
    </div>
    {selected.size > 0 && <div className="mb-3 flex flex-wrap items-center gap-2 border border-line bg-panel px-3 py-2 text-[13px]">
      <b>{selected.size} selected</b>
      <Can do="containers.stop"><Button onClick={() => void bulk("stop", canStop)}>Stop {selected.size} containers</Button></Can>
      <Can do="containers.pause"><Button onClick={() => void bulk("pause", (c) => c.state === "running")}>Pause {selected.size} containers</Button></Can>
      <Can do="containers.restart"><Button onClick={() => void bulk("restart", () => true)}>Restart {selected.size} containers</Button></Can>
      <Can do="containers.remove"><Button variant="danger" onClick={() => setBulkRemove(rows.filter((c) => selected.has(c.id)))}>Remove {selected.size} containers</Button></Can>
      <Button className="ml-auto" onClick={() => setSelected(new Set())}>Clear</Button>
    </div>}
    {summary && <p role="status" className="mb-3 border-l-2 border-pause bg-panel p-2 text-[12px]">{summary}</p>}
    <div className="overflow-x-auto rounded border border-line bg-panel">{containers.isLoading ? <EmptyState title="Loading containers" action="Contacting Docker…" /> : rows.length === 0 ? <EmptyState title="No containers found" action="Create a container or broaden the current filter." /> : <table className="w-full min-w-[980px] border-collapse text-[13px]"><thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted"><tr><th className="w-8 px-2"><input aria-label="Select all" type="checkbox" checked={allSelected} onChange={() => setSelected(allSelected ? new Set() : new Set(rows.map((c) => c.id)))} /></th><th className="px-2">State</th><th className="px-2">Name</th><th className="px-2">Stack</th><th className="px-2">Image</th><th className="px-2">Ports</th><th className="px-2">Uptime</th><th className="w-[240px] px-2" /></tr></thead><tbody>{rows.map((c) => <tr key={c.id} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper"><td className="px-2"><input aria-label={`Select ${c.name}`} type="checkbox" checked={selected.has(c.id)} onChange={() => toggle(c.id)} /></td><td className="px-2"><Flag state={flagState(c.state)} /></td><td className="px-2 font-medium"><Link to={`/containers/${encodeURIComponent(c.id)}`}>{c.name}</Link></td><td className="px-2 text-[12px]">{stackOf(c) ? <Link to={`/stacks/${encodeURIComponent(stackOf(c)!)}`}>{stackOf(c)}</Link> : <span className="text-muted">—</span>}</td><td className="max-w-[240px] truncate px-2 font-mono text-[12px]">{c.image}</td><td className="px-2 font-mono text-[12px]">{c.ports.map((p, i) => { const href = httpPort(p); const label = `${p.public_port ? `${p.ip ?? "0.0.0.0"}:${p.public_port} → ` : ""}${p.private_port}/${p.type}`; return <span key={i}>{i > 0 && ", "}{href ? <a href={href} target="_blank" rel="noreferrer">{label}</a> : label}</span>; })}</td><td className="px-2 font-mono text-[12px]">{uptime(c.created)}</td><td className="px-2 text-right"><span className="opacity-0 group-hover:opacity-100 focus-within:opacity-100"><ContainerRowActions container={c} onLifecycle={(row, action) => void lifecycle(row, action)} onEdit={(row) => void openEdit(row)} onRemove={setRemove} /></span></td></tr>)}</tbody></table>}</div>
    {remove && <RemoveDialog container={remove} close={() => setRemove(null)} done={() => void queryClient.invalidateQueries({ queryKey: ["containers"] })} />}
    {bulkRemove && <BulkRemoveDialog containers={bulkRemove} close={() => setBulkRemove(null)} done={() => void queryClient.invalidateQueries({ queryKey: ["containers"] })} />}
    {creating && <CreateContainerModal close={() => setCreating(false)} />}
    {editTarget && <CreateContainerModal existing={editTarget} close={() => setEditTarget(null)} />}
  </section>;
}
