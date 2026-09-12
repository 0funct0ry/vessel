import { useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Download, Eye, Files, FolderOpen, Link as LinkIcon, Trash2, Unlink } from "lucide-react";
import { Can } from "../auth/Can";
import { FileBrowser } from "../components/containers/FileBrowser";
import { Button } from "../components/ui/Button";
import { EmptyState } from "../components/ui/EmptyState";
import { Select, type SelectOption } from "../components/ui/Select";
import { Tabs } from "../components/ui/Tabs";
import { useToast } from "../components/ui/Toast";
import { api, apiRoot, getToken } from "../lib/api";
import { bytes } from "../lib/containers";
import type { Container, Host, Network, NetworkConnection, Volume } from "../types/api";

const inputClass = "mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 text-[13px]";
const rowAction = "rounded border border-transparent px-1.5 py-0.5 text-[12px] text-muted hover:border-line hover:bg-panel hover:text-text disabled:cursor-not-allowed disabled:opacity-45";
const dangerAction = `${rowAction} hover:text-fail`;
const builtIn = (name: string) => ["bridge", "host", "none"].includes(name);
const resourceName = /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$/;

function labelsFrom(rows: Array<[string, string]>) { return Object.fromEntries(rows.filter(([key]) => key.trim()).map(([key, value]) => [key.trim(), value])); }
function resourceQuery(path: string, q: string) { return q.trim() ? `${path}?q=${encodeURIComponent(q.trim())}` : path; }
function primaryIP(connection: NetworkConnection) { return connection.ipv4_address?.replace(/\/.*/, "") || connection.ipv6_address?.replace(/\/.*/, "") || "—"; }
async function copyToClipboard(text: string, label: string, push: (msg: string, kind?: "error") => void) {
  try { await navigator.clipboard.writeText(text); push(`Copied ${text.length > 16 ? `${text.slice(0, 12)}…` : text}.`); }
  catch { push(`Could not copy ${label}. Select it and copy manually.`, "error"); }
}

function Toggle({ label, checked, onChange, disabled }: { label: string; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return <label className={`mt-3 flex items-center justify-between rounded border border-line px-2.5 py-1.5 text-[13px] ${disabled ? "opacity-45" : ""}`}>
    <span>{label}</span>
    <button type="button" role="switch" aria-checked={checked} disabled={disabled} onClick={() => onChange(!checked)} className={`relative inline-block h-5 w-9 shrink-0 appearance-none rounded-full border-0 p-0 outline-none transition-colors ${checked ? "bg-hull" : "bg-line"} disabled:cursor-not-allowed`}>
      <span className={`absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-panel transition-transform ${checked ? "translate-x-4" : "translate-x-0"}`} />
    </button>
  </label>;
}

function Modal({ title, children, close, busy = false }: { title: string; children: React.ReactNode; close: () => void; busy?: boolean }) {
  return <div role="dialog" aria-modal="true" aria-label={title} className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4" onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) close(); }}>
    <div className="max-h-[90vh] w-full max-w-lg overflow-auto rounded border border-line bg-panel p-5 shadow-lg">
      <div className="flex items-center gap-3"><h2 className="m-0 text-lg">{title}</h2><button type="button" aria-label={`Close ${title}`} disabled={busy} onClick={close} className="ml-auto rounded px-2 text-xl text-muted hover:bg-paper hover:text-text">×</button></div>
      {children}
    </div>
  </div>;
}

function LabelsEditor({ rows, setRows, disabled, title = "Labels", hint = "Optional key/value metadata.", addLabel = "Add label", keyPlaceholder = "key", valuePlaceholder = "value" }: { rows: Array<[string, string]>; setRows: (v: Array<[string, string]>) => void; disabled?: boolean; title?: string; hint?: string; addLabel?: string; keyPlaceholder?: string; valuePlaceholder?: string }) {
  const change = (index: number, part: 0 | 1, value: string) => setRows(rows.map((row, i) => i === index ? (part === 0 ? [value, row[1]] : [row[0], value]) : row));
  return <div className="mt-4"><div className="mb-1 flex items-center"><span className="text-[13px] font-medium">{title}</span><button type="button" disabled={disabled} onClick={() => setRows([...rows, ["", ""]])} className="ml-auto text-[12px] text-link underline">{addLabel}</button></div>{rows.length === 0 ? <p className="m-0 text-[12px] text-muted">{hint}</p> : <div className="space-y-2">{rows.map(([key, value], index) => <div key={index} className="flex gap-2"><input disabled={disabled} value={key} onChange={(e) => change(index, 0, e.target.value)} placeholder={keyPlaceholder} className="min-w-0 flex-1 rounded border border-line px-2 py-1 text-[12px]" /><input disabled={disabled} value={value} onChange={(e) => change(index, 1, e.target.value)} placeholder={valuePlaceholder} className="min-w-0 flex-1 rounded border border-line px-2 py-1 text-[12px]" /><button type="button" disabled={disabled} aria-label={`Remove ${title.toLowerCase()} ${index + 1}`} onClick={() => setRows(rows.filter((_, i) => i !== index))} className="text-muted hover:text-fail">×</button></div>)}</div>}</div>;
}

const DRIVER_OPT_PLACEHOLDERS: Record<string, Array<[string, string]>> = {
  nfs: [["type", "nfs"], ["o", "addr=<host>,rw"], ["device", ":<path>"]],
  cifs: [["type", "cifs"], ["o", "username=…,password=…"], ["device", "//<host>/<share>"]],
};

function DriverOptsEditor({ rows, setRows, placeholders, disabled }: { rows: Array<[string, string]>; setRows: (v: Array<[string, string]>) => void; placeholders: Array<[string, string]>; disabled?: boolean }) {
  const change = (index: number, part: 0 | 1, value: string) => setRows(rows.map((row, i) => i === index ? (part === 0 ? [value, row[1]] : [row[0], value]) : row));
  return <div className="mt-4"><div className="mb-1 flex items-center"><span className="text-[13px] font-medium">Driver options</span><button type="button" disabled={disabled} onClick={() => setRows([...rows, ["", ""]])} className="ml-auto text-[12px] text-link underline">Add option</button></div>{rows.length === 0 ? <p className="m-0 text-[12px] text-muted">Optional driver-specific options (e.g. NFS/CIFS mount settings).</p> : <div className="space-y-2">{rows.map(([key, value], index) => <div key={index} className="flex gap-2"><input disabled={disabled} value={key} onChange={(e) => change(index, 0, e.target.value)} placeholder={placeholders[index]?.[0] ?? "key"} className="min-w-0 flex-1 rounded border border-line px-2 py-1 font-mono text-[12px]" /><input disabled={disabled} value={value} onChange={(e) => change(index, 1, e.target.value)} placeholder={placeholders[index]?.[1] ?? "value"} className="min-w-0 flex-1 rounded border border-line px-2 py-1 font-mono text-[12px]" /><button type="button" disabled={disabled} aria-label={`Remove option ${index + 1}`} onClick={() => setRows(rows.filter((_, i) => i !== index))} className="text-muted hover:text-fail">×</button></div>)}</div>}</div>;
}

const DRIVER_OPTIONS: SelectOption[] = [{ value: "local", label: "local" }, { value: "nfs", label: "nfs" }, { value: "cifs", label: "cifs" }, { value: "custom", label: "custom…" }];

function CreateVolumeModal({ close }: { close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const navigate = useNavigate();
  const [name, setName] = useState(""); const [driver, setDriver] = useState("local"); const [customDriver, setCustomDriver] = useState("");
  const [driverOpts, setDriverOpts] = useState<Array<[string, string]>>([]); const [labels, setLabels] = useState<Array<[string, string]>>([]); const [busy, setBusy] = useState(false);
  const valid = resourceName.test(name); const effectiveDriver = driver === "custom" ? customDriver.trim() : driver;
  const placeholders = DRIVER_OPT_PLACEHOLDERS[driver] ?? [];
  function changeDriver(next: string) {
    setDriver(next);
    const nextPlaceholders = DRIVER_OPT_PLACEHOLDERS[next] ?? [];
    setDriverOpts(nextPlaceholders.length ? nextPlaceholders.map(() => ["", ""] as [string, string]) : []);
  }
  async function submit(e: FormEvent) { e.preventDefault(); if (!valid) return; setBusy(true); try { const volume = await api.post<Volume>("/volumes", { name, driver: effectiveDriver, driver_opts: labelsFrom(driverOpts), labels: labelsFrom(labels) }); await queryClient.invalidateQueries({ queryKey: ["volumes"] }); push(`Created volume ${volume.name}.`); navigate(`/volumes/${encodeURIComponent(volume.name)}`); } catch (err) { push(`Could not create volume: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title="Create volume" close={close} busy={busy}><form onSubmit={(e) => void submit(e)}><label className="mt-4 block text-[13px]">Name<input autoFocus disabled={busy} value={name} onChange={(e) => setName(e.target.value)} placeholder="api-data" className={inputClass} /></label>{name && !valid && <p className="mb-0 text-[12px] text-fail">Use 1–63 letters, digits, dots, underscores, or hyphens; start with a letter or digit.</p>}<label className="mt-3 block text-[13px]">Driver<Select disabled={busy} value={driver} onChange={changeDriver} options={DRIVER_OPTIONS} /></label>{driver === "custom" && <label className="mt-3 block text-[13px]">Custom driver name<input disabled={busy} value={customDriver} onChange={(e) => setCustomDriver(e.target.value)} placeholder="my-driver" className={inputClass} /></label>}<DriverOptsEditor rows={driverOpts} setRows={setDriverOpts} placeholders={placeholders} disabled={busy} /><LabelsEditor rows={labels} setRows={setLabels} disabled={busy} /><div className="mt-5 flex justify-end gap-2"><Button type="button" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" variant="primary" disabled={!valid || busy}>Create volume</Button></div></form></Modal>;
}

function RemoveVolumeModal({ volume, close }: { volume: Volume; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const navigate = useNavigate(); const [busy, setBusy] = useState(false);
  async function remove() { setBusy(true); try { await api.delete(`/volumes/${encodeURIComponent(volume.name)}`); await queryClient.invalidateQueries({ queryKey: ["volumes"] }); push(`Removed volume ${volume.name}.`); close(); navigate("/volumes"); } catch (err) { push(`Could not remove ${volume.name}: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title={`Remove ${volume.name}?`} close={close} busy={busy}><p className="mt-3 text-[13px] text-muted">This permanently deletes the volume and its data.</p><div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void remove()}>Remove</Button></div></Modal>;
}

async function exportVolume(name: string, push: (msg: string, kind?: "error") => void) {
  try {
    const headers = new Headers(); const token = getToken(); if (token) headers.set("Authorization", `Bearer ${token}`);
    const res = await fetch(`${apiRoot()}/volumes/${encodeURIComponent(name)}/export`, { headers });
    if (!res.ok) throw new Error(res.statusText);
    const href = URL.createObjectURL(await res.blob()); const a = document.createElement("a"); a.href = href; a.download = `${name}.tar`; a.click(); URL.revokeObjectURL(href);
  } catch (err) { push(`Could not export ${name}: ${err instanceof Error ? err.message : "try again"}.`, "error"); }
}

function VolumeFilesModal({ volume, close }: { volume: Volume; close: () => void }) {
  return <div role="dialog" aria-modal="true" aria-label={`Browse ${volume.name}`} className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4" onMouseDown={(e) => { if (e.target === e.currentTarget) close(); }}>
    <div className="flex max-h-[97vh] w-full max-w-[1200px] flex-col overflow-hidden rounded border border-line bg-panel p-5 shadow-lg">
      <div className="flex items-center gap-3"><h2 className="m-0 text-lg">{volume.name}</h2><span className="flex-1" /><button type="button" aria-label="Close" onClick={close} className="rounded px-2 text-xl text-muted hover:bg-paper hover:text-text">×</button></div>
      <div className="mt-4 min-h-0 flex-1 overflow-auto"><FileBrowser containerID={volume.name} basePath={`/volumes/${encodeURIComponent(volume.name)}`} capabilityPrefix="volumes.files" resourceLabel="the volume's helper container" /></div>
    </div>
  </div>;
}

function CloneVolumeModal({ volume, close }: { volume: Volume; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const navigate = useNavigate();
  const [name, setName] = useState(`${volume.name}-copy`); const [busy, setBusy] = useState(false); const valid = resourceName.test(name);
  async function submit(e: FormEvent) { e.preventDefault(); if (!valid) return; setBusy(true); try { const clone = await api.post<Volume>(`/volumes/${encodeURIComponent(volume.name)}/clone`, { name }); await queryClient.invalidateQueries({ queryKey: ["volumes"] }); push(`Cloned ${volume.name} to ${clone.name}.`); navigate(`/volumes/${encodeURIComponent(clone.name)}`); } catch (err) { push(`Could not clone ${volume.name}: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title={`Clone ${volume.name}`} close={close} busy={busy}><form onSubmit={(e) => void submit(e)}><label className="mt-4 block text-[13px]">New volume name<input autoFocus disabled={busy} value={name} onChange={(e) => setName(e.target.value)} className={inputClass} /></label>{name && !valid && <p className="mb-0 text-[12px] text-fail">Use 1–63 letters, digits, dots, underscores, or hyphens; start with a letter or digit.</p>}<div className="mt-5 flex justify-end gap-2"><Button type="button" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" variant="primary" disabled={!valid || busy}>Clone volume</Button></div></form></Modal>;
}

function PruneVolumesModal({ close }: { close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const host = useQuery({ queryKey: ["host"], queryFn: () => api.get<Host>("/host") }); const [busy, setBusy] = useState(false); const estimate = host.data?.disk.volumes_reclaimable ?? 0;
  async function prune() { setBusy(true); try { const result = await api.post<{ deleted: string[]; space_reclaimed: number }>("/prune/volumes"); await queryClient.invalidateQueries({ queryKey: ["volumes"] }); await queryClient.invalidateQueries({ queryKey: ["host"] }); push(`Pruned ${result.deleted.length} volume(s), reclaimed ${bytes(result.space_reclaimed)}.`); close(); } catch (err) { push(`Could not prune volumes: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title="Prune unused volumes?" close={close} busy={busy}><p className="mt-3 text-[13px] text-muted">Removes every unused volume. Estimated reclaim: <b>{host.isLoading ? "…" : bytes(estimate)}</b>.</p><div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy || host.isLoading} onClick={() => void prune()}>Prune</Button></div></Modal>;
}

const iconAction = "rounded p-1 text-muted hover:bg-paper hover:text-text disabled:cursor-not-allowed disabled:opacity-45";
const dangerIconAction = `${iconAction} hover:bg-fail/10 hover:text-fail`;

function VolumeRowActions({ volume, onBrowse, onClone, onRemove, push }: { volume: Volume; onBrowse: () => void; onClone: () => void; onRemove: () => void; push: (msg: string, kind?: "error") => void }) {
  const navigate = useNavigate();
  return <span className="flex items-center justify-end gap-0.5">
    <button type="button" title="View details" aria-label="View details" onClick={() => navigate(`/volumes/${encodeURIComponent(volume.name)}`)} className={iconAction}><Eye size={15} /></button>
    <Can do="volumes.files.list"><button type="button" title="Browse files" aria-label="Browse files" onClick={onBrowse} className={iconAction}><FolderOpen size={15} /></button></Can>
    <Can do="volumes.export"><button type="button" title="Export as tar" aria-label="Export as tar" onClick={() => void exportVolume(volume.name, push)} className={iconAction}><Download size={15} /></button></Can>
    <Can do="volumes.clone"><button type="button" title="Clone" aria-label="Clone" onClick={onClone} className={iconAction}><Copy size={15} /></button></Can>
    {volume.used_by.length ? <button disabled title={`In use by ${volume.used_by.map((u) => u.container_name).join(", ")}`} className={dangerIconAction}><Trash2 size={15} /></button> : <Can do="volumes.remove"><button type="button" title="Remove" aria-label="Remove" onClick={onRemove} className={dangerIconAction}><Trash2 size={15} /></button></Can>}
  </span>;
}

export function VolumesPage() {
  const [q, setQ] = useState(""); const [create, setCreate] = useState(false); const [remove, setRemove] = useState<Volume | null>(null); const [prune, setPrune] = useState(false);
  const [browse, setBrowse] = useState<Volume | null>(null); const [clone, setClone] = useState<Volume | null>(null);
  const { push } = useToast();
  const volumes = useQuery({ queryKey: ["volumes", q], queryFn: () => api.get<Volume[]>(resourceQuery("/volumes", q)) }); const host = useQuery({ queryKey: ["host"], queryFn: () => api.get<Host>("/host") }); const rows = volumes.data ?? [];
  const reclaimable = host.data?.disk.volumes_reclaimable;
  return <section><div className="mb-3 flex flex-wrap items-center gap-2"><input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter volumes" aria-label="Filter volumes" className="min-w-[200px] rounded border border-line bg-panel px-2 py-1.5 text-[13px]" /><span className="flex-1" /><Can do="prune.run"><Button onClick={() => setPrune(true)}>Prune unused{reclaimable === undefined ? "" : ` · ${bytes(reclaimable)}`}</Button></Can><Can do="volumes.create"><Button variant="primary" onClick={() => setCreate(true)}>Create volume</Button></Can></div><div className="overflow-x-auto rounded border border-line bg-panel">{volumes.isLoading ? <EmptyState title="Loading volumes" action="Contacting Docker…" /> : rows.length === 0 ? <EmptyState title="No volumes found" action="Create a volume, or broaden the current filter." /> : <table className="w-full min-w-[760px] border-collapse text-[13px]"><thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted"><tr><th className="px-3">Name</th><th className="px-3">Driver</th><th className="px-3">Mountpoint</th><th className="px-3 text-right">Size</th><th className="px-3">Used by</th><th className="w-[160px] px-3" /></tr></thead><tbody>{rows.map((volume) => <tr key={volume.name} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper"><td className="px-3 font-medium"><Link to={`/volumes/${encodeURIComponent(volume.name)}`}>{volume.name}</Link></td><td className="px-3 font-mono text-[12px] text-muted">{volume.driver || "—"}</td><td className="max-w-[280px] truncate px-3 font-mono text-[12px] text-muted" title={volume.mountpoint}>{volume.mountpoint || "—"}</td><td className="px-3 text-right font-mono text-[12px] text-muted">{volume.size_bytes === undefined ? "—" : bytes(volume.size_bytes)}</td><td className="px-3">{volume.used_by.length ? <span className="flex flex-wrap gap-1">{volume.used_by.map((use) => <Link key={use.container_id} to={`/containers/${encodeURIComponent(use.container_id)}`} className="rounded-sm border border-[#BFD5CE] px-1 font-mono text-[11px] text-run">{use.container_name || use.container_id.slice(0, 12)}</Link>)}</span> : <span className="rounded-sm border border-line px-1 font-mono text-[11px] text-muted">unused</span>}</td><td className="px-3 text-right"><span className="opacity-0 group-hover:opacity-100 focus-within:opacity-100"><VolumeRowActions volume={volume} onBrowse={() => setBrowse(volume)} onClone={() => setClone(volume)} onRemove={() => setRemove(volume)} push={push} /></span></td></tr>)}</tbody></table>}</div>{create && <CreateVolumeModal close={() => setCreate(false)} />}{remove && <RemoveVolumeModal volume={remove} close={() => setRemove(null)} />}{prune && <PruneVolumesModal close={() => setPrune(false)} />}{browse && <VolumeFilesModal volume={browse} close={() => setBrowse(null)} />}{clone && <CloneVolumeModal volume={clone} close={() => setClone(null)} />}</section>;
}

export function VolumeDetailPage() {
  const { name = "" } = useParams(); const [remove, setRemove] = useState(false); const [browse, setBrowse] = useState(false); const [clone, setClone] = useState(false);
  const { push } = useToast();
  const volume = useQuery({ queryKey: ["volume", name], queryFn: () => api.get<Volume>(`/volumes/${encodeURIComponent(name)}`) }); const item = volume.data;
  if (volume.isLoading) return <EmptyState title="Loading volume" action="Contacting Docker…" />; if (!item) return <EmptyState title="Volume not found" action="Return to the volumes list and refresh." />;
  return <section><div className="mb-3 flex items-center gap-3"><h1 className="m-0 text-lg">{item.name}</h1><span className="flex-1" />
    <Can do="volumes.files.list"><Button onClick={() => setBrowse(true)}><FolderOpen size={14} className="mr-1 inline" />Browse files</Button></Can>
    <Can do="volumes.export"><Button onClick={() => void exportVolume(item.name, push)}><Download size={14} className="mr-1 inline" />Export</Button></Can>
    <Can do="volumes.clone"><Button onClick={() => setClone(true)}><Copy size={14} className="mr-1 inline" />Clone</Button></Can>
    <Can do="volumes.remove"><Button variant="danger" disabled={item.used_by.length > 0} title={item.used_by.length ? "This volume is in use and cannot be removed." : undefined} onClick={() => setRemove(true)}>Remove</Button></Can>
  </div><div className="grid gap-3 md:grid-cols-2"><Info title="Configuration" rows={[["Driver", item.driver || "—"], ["Mountpoint", item.mountpoint || "—"], ["Scope", item.scope || "—"], ["Size", item.size_bytes === undefined ? "—" : bytes(item.size_bytes)], ["Created", item.created_at || "—"]]} /><Info title="Labels" rows={Object.entries(item.labels)} /><div className="rounded border border-line bg-panel p-4 md:col-span-2"><h2 className="m-0 mb-3 text-[14px]">Containers using this volume</h2>{item.used_by.length ? <div className="overflow-x-auto"><table className="w-full border-collapse text-[12px]"><thead className="border-b border-line bg-paper text-left text-[11px] uppercase text-muted"><tr><th className="px-2">Container</th><th className="px-2">Mount path</th><th className="px-2">Mode</th></tr></thead><tbody>{item.used_by.map((use) => <tr key={use.container_id} className="h-row border-b border-linesoft last:border-0"><td className="px-2"><Link to={`/containers/${encodeURIComponent(use.container_id)}`} className="font-mono">{use.container_name || use.container_id.slice(0, 12)}</Link></td><td className="px-2 font-mono">{use.mount_path}</td><td className="px-2 font-mono">{use.rw ? "rw" : "ro"}</td></tr>)}</tbody></table></div> : <p className="m-0 text-[12px] text-muted">No containers use this volume.</p>}</div><Raw value={item.raw} /></div>{remove && <RemoveVolumeModal volume={item} close={() => setRemove(false)} />}{browse && <VolumeFilesModal volume={item} close={() => setBrowse(false)} />}{clone && <CloneVolumeModal volume={item} close={() => setClone(false)} />}</section>;
}

function validIPv4(value: string) { const parts = value.split("."); return parts.length === 4 && parts.every((part) => /^\d+$/.test(part) && Number(part) >= 0 && Number(part) <= 255); }
function validIPv6(value: string) { if (!value || value.includes(":::") || !/^[0-9a-fA-F:]+$/.test(value)) return false; const parts = value.split("::"); if (parts.length > 2) return false; const count = value.split(":").filter(Boolean).length; return parts.length === 2 ? count < 8 : count === 8; }
function validCIDR(value: string) { const [ip, prefix, ...more] = value.split("/"); if (more.length || !/^\d+$/.test(prefix ?? "")) return false; return validIPv4(ip) ? Number(prefix) <= 32 : validIPv6(ip) && Number(prefix) <= 128; }
function validIP(value: string) { return validIPv4(value) || validIPv6(value); }

const NETWORK_DRIVER_OPTIONS: SelectOption[] = [{ value: "bridge", label: "bridge" }, { value: "host", label: "host" }, { value: "overlay", label: "overlay" }, { value: "macvlan", label: "macvlan" }, { value: "ipvlan", label: "ipvlan" }, { value: "none", label: "none" }, { value: "custom", label: "custom…" }];
const DRIVER_OPT_REFERENCE: Array<[string, string]> = [["com.docker.network.bridge.name", "Name of the bridge device to use"], ["com.docker.network.bridge.enable_ip_masquerade", "Enable IP masquerading (true/false)"], ["com.docker.network.bridge.enable_icc", "Enable inter-container communication (true/false)"], ["com.docker.network.bridge.host_binding_ipv4", "Default IP for bound ports"], ["com.docker.network.driver.mtu", "Set the MTU"]];
const NETWORK_TABS = [{ key: "basic", label: "Basic" }, { key: "ipam", label: "IPAM" }, { key: "options", label: "Options" }, { key: "labels", label: "Labels" }];

function CreateNetworkModal({ close, initial }: { close: () => void; initial?: Network }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const navigate = useNavigate();
  const [tab, setTab] = useState("basic");
  const [name, setName] = useState(initial ? `${initial.name}-copy` : "");
  const knownDriver = initial && NETWORK_DRIVER_OPTIONS.some((o) => o.value === initial.driver);
  const [driver, setDriver] = useState(initial ? (knownDriver ? initial.driver : "custom") : "bridge");
  const [customDriver, setCustomDriver] = useState(initial && !knownDriver ? initial.driver : "");
  const [internal, setInternal] = useState(initial?.internal ?? false);
  const [attachable, setAttachable] = useState(initial?.attachable ?? false);
  const [enableIPv6, setEnableIPv6] = useState(initial?.enable_ipv6 ?? false);
  const initialIPAM = initial?.ipam[0];
  const [ipamDriver, setIPAMDriver] = useState(initial?.ipam_driver ?? "");
  const [subnet, setSubnet] = useState(initialIPAM?.subnet ?? "");
  const [gateway, setGateway] = useState(initialIPAM?.gateway ?? "");
  const [ipRange, setIPRange] = useState(initialIPAM?.ip_range ?? "");
  const [auxAddresses, setAuxAddresses] = useState<Array<[string, string]>>(Object.entries(initialIPAM?.aux_addresses ?? {}));
  const [ipamOptions, setIPAMOptions] = useState<Array<[string, string]>>(Object.entries(initial?.ipam_options ?? {}));
  const [driverOpts, setDriverOpts] = useState<Array<[string, string]>>(Object.entries(initial?.driver_opts ?? {}));
  const [labels, setLabels] = useState<Array<[string, string]>>(Object.entries(initial?.labels ?? {}));
  const [busy, setBusy] = useState(false);
  const effectiveDriver = driver === "custom" ? customDriver.trim() : driver;
  const ipamDisabled = effectiveDriver === "host" || effectiveDriver === "none";
  const nameOK = resourceName.test(name); const subnetOK = ipamDisabled || !subnet || validCIDR(subnet); const gatewayOK = ipamDisabled || !gateway || validIP(gateway); const ipRangeOK = ipamDisabled || !ipRange || validCIDR(ipRange);
  const basicValid = nameOK; const ipamValid = subnetOK && gatewayOK && ipRangeOK;
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!basicValid) { setTab("basic"); return; }
    if (!ipamValid) { setTab("ipam"); return; }
    setBusy(true);
    try {
      const network = await api.post<Network>("/networks", {
        name, driver: effectiveDriver, internal, attachable, enable_ipv6: enableIPv6,
        ipam_driver: ipamDisabled ? "" : ipamDriver.trim(),
        subnet: ipamDisabled ? "" : subnet.trim(), gateway: ipamDisabled ? "" : gateway.trim(), ip_range: ipamDisabled ? "" : ipRange.trim(),
        aux_addresses: ipamDisabled ? {} : labelsFrom(auxAddresses), ipam_options: ipamDisabled ? {} : labelsFrom(ipamOptions),
        driver_opts: labelsFrom(driverOpts), labels: labelsFrom(labels),
      });
      await queryClient.invalidateQueries({ queryKey: ["networks"] });
      push(`Created network ${network.name}.`);
      navigate(`/networks/${encodeURIComponent(network.id)}`);
    } catch (err) { push(`Could not create network: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); }
  }
  return <Modal title={initial ? `Duplicate ${initial.name}` : "Create network"} close={close} busy={busy}>
    <form onSubmit={(e) => void submit(e)}>
      <Tabs tabs={NETWORK_TABS.map((t) => ({ ...t, invalid: (t.key === "basic" && !basicValid) || (t.key === "ipam" && !ipamValid) }))} active={tab} onChange={setTab} />
      {tab === "basic" && <div>
        <label className="mt-4 block text-[13px]">Name<input autoFocus disabled={busy} value={name} onChange={(e) => setName(e.target.value)} placeholder="acme_default" className={inputClass} /></label>
        {name && !nameOK && <p className="mb-0 text-[12px] text-fail">Use 1–63 letters, digits, dots, underscores, or hyphens; start with a letter or digit.</p>}
        <label className="mt-3 block text-[13px]">Driver<Select disabled={busy} value={driver} onChange={setDriver} options={NETWORK_DRIVER_OPTIONS} /></label>
        {driver === "custom" && <label className="mt-3 block text-[13px]">Custom driver name<input disabled={busy} value={customDriver} onChange={(e) => setCustomDriver(e.target.value)} placeholder="my-driver" className={inputClass} /></label>}
        <Toggle label="Internal network" checked={internal} onChange={setInternal} disabled={busy} />
        <Toggle label="Attachable" checked={attachable} onChange={setAttachable} disabled={busy} />
        <Toggle label="Enable IPv6" checked={enableIPv6} onChange={setEnableIPv6} disabled={busy} />
      </div>}
      {tab === "ipam" && <div>
        {ipamDisabled && <p className="mt-4 mb-0 text-[12px] text-muted">host and none networks do not take IPAM configuration.</p>}
        <label className="mt-4 block text-[13px]">IPAM driver<input disabled={busy || ipamDisabled} value={ipamDriver} onChange={(e) => setIPAMDriver(e.target.value)} placeholder="default" className={inputClass} /></label>
        <label className="mt-3 block text-[13px]">Subnet <span className="text-muted">optional</span><input disabled={busy || ipamDisabled} value={subnet} onChange={(e) => setSubnet(e.target.value)} placeholder="172.22.0.0/16" className={inputClass} /></label>
        {subnet && !subnetOK && <p className="mb-0 text-[12px] text-fail">Enter a valid IPv4 or IPv6 CIDR.</p>}
        <label className="mt-3 block text-[13px]">Gateway <span className="text-muted">optional</span><input disabled={busy || ipamDisabled} value={gateway} onChange={(e) => setGateway(e.target.value)} placeholder="172.22.0.1" className={inputClass} /></label>
        {gateway && !gatewayOK && <p className="mb-0 text-[12px] text-fail">Enter a valid IPv4 or IPv6 address.</p>}
        <label className="mt-3 block text-[13px]">IP range <span className="text-muted">optional</span><input disabled={busy || ipamDisabled} value={ipRange} onChange={(e) => setIPRange(e.target.value)} placeholder="172.22.5.0/24" className={inputClass} /></label>
        {ipRange && !ipRangeOK && <p className="mb-0 text-[12px] text-fail">Enter a valid IPv4 or IPv6 CIDR.</p>}
        <LabelsEditor rows={auxAddresses} setRows={setAuxAddresses} disabled={busy || ipamDisabled} title="Auxiliary addresses" hint="Optional host-name → IP reservations." addLabel="Add address" keyPlaceholder="name" valuePlaceholder="192.168.1.1" />
        <LabelsEditor rows={ipamOptions} setRows={setIPAMOptions} disabled={busy || ipamDisabled} title="IPAM options" hint="Optional driver-specific IPAM options." addLabel="Add option" />
      </div>}
      {tab === "options" && <div className="mt-4">
        <p className="m-0 text-[12px] text-muted">Common bridge-driver keys:</p>
        <ul className="m-0 mt-1 list-disc pl-4 text-[11.5px] text-muted">{DRIVER_OPT_REFERENCE.map(([key, desc]) => <li key={key}><span className="font-mono">{key}</span> — {desc}</li>)}</ul>
        <DriverOptsEditor rows={driverOpts} setRows={setDriverOpts} placeholders={[]} disabled={busy} />
      </div>}
      {tab === "labels" && <LabelsEditor rows={labels} setRows={setLabels} disabled={busy} />}
      <div className="mt-5 flex justify-end gap-2"><Button type="button" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" variant="primary" disabled={busy}>{initial ? "Create duplicate" : "Create network"}</Button></div>
    </form>
  </Modal>;
}

function RemoveNetworkModal({ network, close }: { network: Network; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const navigate = useNavigate(); const [busy, setBusy] = useState(false);
  async function remove() { setBusy(true); try { await api.delete(`/networks/${encodeURIComponent(network.id)}`); await queryClient.invalidateQueries({ queryKey: ["networks"] }); push(`Removed network ${network.name}.`); close(); navigate("/networks"); } catch (err) { push(`Could not remove ${network.name}: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title={`Remove ${network.name}?`} close={close} busy={busy}><p className="mt-3 text-[13px] text-muted">This disconnects its containers and removes the network.</p><div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void remove()}>Remove</Button></div></Modal>;
}

function PruneNetworksModal({ close }: { close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const [busy, setBusy] = useState(false);
  async function prune() { setBusy(true); try { const result = await api.post<{ deleted: string[] }>("/prune/networks"); await queryClient.invalidateQueries({ queryKey: ["networks"] }); push(`Pruned ${result.deleted.length} network(s).`); close(); } catch (err) { push(`Could not prune networks: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title="Prune unused networks?" close={close} busy={busy}><p className="mt-3 text-[13px] text-muted">Removes every network not used by at least one container.</p><div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void prune()}>Prune</Button></div></Modal>;
}

function ConnectNetworkModal({ network, close }: { network: Network; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const [container, setContainer] = useState(""); const [busy, setBusy] = useState(false); const all = useQuery({ queryKey: ["containers", "network-connect"], queryFn: () => api.get<Container[]>("/containers?all=true") }); const available = (all.data ?? []).filter((item) => !network.containers.some((connection) => connection.container_id === item.id));
  async function connect() { if (!container) return; setBusy(true); try { await api.post(`/networks/${encodeURIComponent(network.id)}/connect`, { container }); await queryClient.invalidateQueries({ queryKey: ["network", network.id] }); await queryClient.invalidateQueries({ queryKey: ["networks"] }); await queryClient.invalidateQueries({ queryKey: ["containers"] }); push("Container connected to network."); close(); } catch (err) { push(`Could not connect container: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title={`Connect to ${network.name}`} close={close} busy={busy}><label className="mt-4 block text-[13px]">Container<select autoFocus disabled={busy || all.isLoading} value={container} onChange={(e) => setContainer(e.target.value)} className={inputClass}><option value="">Select a container</option>{available.map((item) => <option key={item.id} value={item.id}>{item.name || item.id.slice(0, 12)}</option>)}</select></label>{!all.isLoading && available.length === 0 && <p className="mb-0 text-[12px] text-muted">Every available container is already attached.</p>}<div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="primary" disabled={!container || busy} onClick={() => void connect()}>Connect</Button></div></Modal>;
}

function DisconnectNetworkModal({ network, connection, close }: { network: Network; connection: NetworkConnection; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const [busy, setBusy] = useState(false);
  async function disconnect() { setBusy(true); try { await api.post(`/networks/${encodeURIComponent(network.id)}/disconnect`, { container: connection.container_id }); await queryClient.invalidateQueries({ queryKey: ["network", network.id] }); await queryClient.invalidateQueries({ queryKey: ["networks"] }); await queryClient.invalidateQueries({ queryKey: ["containers"] }); push("Container disconnected from network."); close(); } catch (err) { push(`Could not disconnect container: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title={`Disconnect ${connection.container_name || connection.container_id.slice(0, 12)}?`} close={close} busy={busy}><p className="mt-3 text-[13px] text-muted">The container will lose access to this network.</p><div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void disconnect()}>Disconnect</Button></div></Modal>;
}

function DisconnectPickerModal({ network, close }: { network: Network; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const [containerID, setContainerID] = useState(""); const [busy, setBusy] = useState(false);
  async function disconnect() { if (!containerID) return; setBusy(true); try { await api.post(`/networks/${encodeURIComponent(network.id)}/disconnect`, { container: containerID }); await queryClient.invalidateQueries({ queryKey: ["network", network.id] }); await queryClient.invalidateQueries({ queryKey: ["networks"] }); await queryClient.invalidateQueries({ queryKey: ["containers"] }); push("Container disconnected from network."); close(); } catch (err) { push(`Could not disconnect container: ${err instanceof Error ? err.message : "try again"}.`, "error"); setBusy(false); } }
  return <Modal title={`Disconnect from ${network.name}`} close={close} busy={busy}><label className="mt-4 block text-[13px]">Container<select autoFocus disabled={busy} value={containerID} onChange={(e) => setContainerID(e.target.value)} className={inputClass}><option value="">Select a container</option>{network.containers.map((connection) => <option key={connection.container_id} value={connection.container_id}>{connection.container_name || connection.container_id.slice(0, 12)}</option>)}</select></label><div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={!containerID || busy} onClick={() => void disconnect()}>Disconnect</Button></div></Modal>;
}

function NetworkRowActions({ network, onView, onConnect, onDuplicate, onDisconnect, onRemove, push }: { network: Network; onView: () => void; onConnect: () => void; onDuplicate: () => void; onDisconnect: () => void; onRemove: () => void; push: (msg: string, kind?: "error") => void }) {
  const system = builtIn(network.name);
  return <span className="flex items-center justify-end gap-0.5">
    <button type="button" title="View details" aria-label="View details" onClick={onView} className={iconAction}><Eye size={15} /></button>
    <Can do="networks.connect"><button type="button" title="Connect container" aria-label="Connect container" onClick={onConnect} className={iconAction}><LinkIcon size={15} /></button></Can>
    <button type="button" title="Copy network ID" aria-label="Copy network ID" onClick={() => void copyToClipboard(network.id, "network ID", push)} className={iconAction}><Copy size={15} /></button>
    <Can do="networks.create"><button type="button" title="Duplicate" aria-label="Duplicate" onClick={onDuplicate} className={iconAction}><Files size={15} /></button></Can>
    {network.containers.length === 0 ? <button disabled title="No containers attached" className={iconAction}><Unlink size={15} /></button> : <Can do="networks.disconnect"><button type="button" title="Disconnect container" aria-label="Disconnect container" onClick={onDisconnect} className={iconAction}><Unlink size={15} /></button></Can>}
    {system ? <button disabled title="Docker built-in network" className={dangerIconAction}><Trash2 size={15} /></button> : <Can do="networks.remove"><button type="button" title="Remove" aria-label="Remove" onClick={onRemove} className={dangerIconAction}><Trash2 size={15} /></button></Can>}
  </span>;
}

export function NetworksPage() {
  const [q, setQ] = useState(""); const [create, setCreate] = useState(false); const [remove, setRemove] = useState<Network | null>(null); const [prune, setPrune] = useState(false);
  const [connect, setConnect] = useState<Network | null>(null); const [duplicate, setDuplicate] = useState<Network | null>(null); const [disconnect, setDisconnect] = useState<Network | null>(null); const [view, setView] = useState<Network | null>(null);
  const { push } = useToast();
  const networks = useQuery({ queryKey: ["networks", q], queryFn: () => api.get<Network[]>(resourceQuery("/networks", q)) }); const rows = networks.data ?? [];
  return <section><div className="mb-3 flex flex-wrap items-center gap-2"><input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter networks" aria-label="Filter networks" className="min-w-[200px] rounded border border-line bg-panel px-2 py-1.5 text-[13px]" /><span className="flex-1" /><Can do="prune.run"><Button onClick={() => setPrune(true)}>Prune unused</Button></Can><Can do="networks.create"><Button variant="primary" onClick={() => setCreate(true)}>Create network</Button></Can></div><div className="overflow-x-auto rounded border border-line bg-panel">{networks.isLoading ? <EmptyState title="Loading networks" action="Contacting Docker…" /> : rows.length === 0 ? <EmptyState title="No networks found" action="Create a network, or broaden the current filter." /> : <table className="w-full min-w-[760px] border-collapse text-[13px]"><thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted"><tr><th className="px-3">Name</th><th className="px-3">Driver</th><th className="px-3">Scope</th><th className="px-3">Subnet</th><th className="px-3">Gateway</th><th className="px-3 text-right">Containers</th><th className="w-[190px] px-3" /></tr></thead><tbody>{rows.map((network) => { const ipam = network.ipam[0]; const system = builtIn(network.name); return <tr key={network.id} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper"><td className="px-3 font-medium"><Link to={`/networks/${encodeURIComponent(network.id)}`}>{network.name}</Link>{system && <span className="ml-2 rounded-sm border border-line px-1 font-mono text-[11px] text-muted">built-in</span>}</td><td className="px-3 font-mono text-[12px] text-muted">{network.driver || "—"}</td><td className="px-3 font-mono text-[12px] text-muted">{network.scope || "—"}</td><td className="px-3 font-mono text-[12px]">{ipam?.subnet || "—"}</td><td className="px-3 font-mono text-[12px] text-muted">{ipam?.gateway || "—"}</td><td className="px-3 text-right font-mono text-[12px]">{network.containers.length}</td><td className="px-3 text-right"><span className="opacity-0 group-hover:opacity-100 focus-within:opacity-100"><NetworkRowActions network={network} onView={() => setView(network)} onConnect={() => setConnect(network)} onDuplicate={() => setDuplicate(network)} onDisconnect={() => setDisconnect(network)} onRemove={() => setRemove(network)} push={push} /></span></td></tr>; })}</tbody></table>}</div>{create && <CreateNetworkModal close={() => setCreate(false)} />}{duplicate && <CreateNetworkModal initial={duplicate} close={() => setDuplicate(null)} />}{remove && <RemoveNetworkModal network={remove} close={() => setRemove(null)} />}{prune && <PruneNetworksModal close={() => setPrune(false)} />}{connect && <ConnectNetworkModal network={connect} close={() => setConnect(null)} />}{disconnect && <DisconnectPickerModal network={disconnect} close={() => setDisconnect(null)} />}{view && <NetworkDetailModal network={view} close={() => setView(null)} />}</section>;
}

function NetworkDetailBody({ item }: { item: Network }) {
  const [disconnect, setDisconnect] = useState<NetworkConnection | null>(null);
  const ipam = item.ipam[0];
  return <div className="grid gap-3 md:grid-cols-2">
    <Info title="Configuration" rows={[["Driver", item.driver || "—"], ["Scope", item.scope || "—"], ["Internal", item.internal ? "yes" : "no"], ["Attachable", item.attachable ? "yes" : "no"], ["IPv6", item.enable_ipv6 ? "enabled" : "disabled"], ["IPAM driver", item.ipam_driver || "—"], ["Subnet", ipam?.subnet || "—"], ["Gateway", ipam?.gateway || "—"], ["IP range", ipam?.ip_range || "—"]]} />
    <Info title="Labels" rows={Object.entries(item.labels)} />
    {Object.keys(item.driver_opts).length > 0 && <Info title="Driver options" rows={Object.entries(item.driver_opts)} />}
    <div className="rounded border border-line bg-panel p-4 md:col-span-2"><h2 className="m-0 mb-3 text-[14px]">Attached containers</h2>{item.containers.length ? <div className="overflow-x-auto"><table className="w-full border-collapse text-[12px]"><thead className="border-b border-line bg-paper text-left text-[11px] uppercase text-muted"><tr><th className="px-2">Container</th><th className="px-2">IP address</th><th className="w-[90px] px-2" /></tr></thead><tbody>{item.containers.map((connection) => <tr key={connection.container_id} className="group h-row border-b border-linesoft last:border-0"><td className="px-2"><Link to={`/containers/${encodeURIComponent(connection.container_id)}`} className="font-mono">{connection.container_name || connection.container_id.slice(0, 12)}</Link></td><td className="px-2 font-mono">{primaryIP(connection)}</td><td className="px-2 text-right"><Can do="networks.disconnect"><button onClick={() => setDisconnect(connection)} className={`${dangerAction} opacity-0 group-hover:opacity-100 focus:opacity-100`}>disconnect</button></Can></td></tr>)}</tbody></table></div> : <p className="m-0 text-[12px] text-muted">No containers are attached.</p>}</div>
    <Raw value={item.raw} />
    {disconnect && <DisconnectNetworkModal network={item} connection={disconnect} close={() => setDisconnect(null)} />}
  </div>;
}

function NetworkDetailModal({ network, close }: { network: Network; close: () => void }) {
  return <div role="dialog" aria-modal="true" aria-label={network.name} className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4" onMouseDown={(e) => { if (e.target === e.currentTarget) close(); }}>
    <div className="max-h-[90vh] w-full max-w-3xl overflow-auto rounded border border-line bg-panel p-5 shadow-lg">
      <div className="mb-3 flex items-center gap-3"><h2 className="m-0 text-lg">{network.name}</h2><span className="flex-1" /><button type="button" aria-label="Close" onClick={close} className="rounded px-2 text-xl text-muted hover:bg-paper hover:text-text">×</button></div>
      <NetworkDetailBody item={network} />
    </div>
  </div>;
}

export function NetworkDetailPage() {
  const { id = "" } = useParams(); const [connect, setConnect] = useState(false); const [remove, setRemove] = useState(false); const [duplicate, setDuplicate] = useState(false); const { push } = useToast();
  const network = useQuery({ queryKey: ["network", id], queryFn: () => api.get<Network>(`/networks/${encodeURIComponent(id)}`) }); const item = network.data;
  if (network.isLoading) return <EmptyState title="Loading network" action="Contacting Docker…" />; if (!item) return <EmptyState title="Network not found" action="Return to the networks list and refresh." />;
  const system = builtIn(item.name);
  return <section><div className="mb-3 flex items-center gap-3"><h1 className="m-0 text-lg">{item.name}</h1>{system && <span className="rounded-sm border border-line px-1 font-mono text-[11px] text-muted">built-in</span>}<span className="flex-1" />
    <Button onClick={() => void copyToClipboard(item.id, "network ID", push)}><Copy size={14} className="mr-1 inline" />Copy ID</Button>
    <Can do="networks.create"><Button onClick={() => setDuplicate(true)}><Files size={14} className="mr-1 inline" />Duplicate</Button></Can>
    <Can do="networks.connect"><Button onClick={() => setConnect(true)}>Connect container</Button></Can>
    <Can do="networks.remove"><Button variant="danger" disabled={system} title={system ? "Docker built-in networks cannot be removed." : undefined} onClick={() => setRemove(true)}>Remove</Button></Can>
  </div>
    <NetworkDetailBody item={item} />
    {connect && <ConnectNetworkModal network={item} close={() => setConnect(false)} />}
    {duplicate && <CreateNetworkModal initial={item} close={() => setDuplicate(false)} />}
    {remove && <RemoveNetworkModal network={item} close={() => setRemove(false)} />}
  </section>;
}

function Info({ title, rows }: { title: string; rows: readonly (readonly [string, string])[] }) { return <div className="rounded border border-line bg-panel p-4"><h2 className="m-0 mb-3 text-[14px]">{title}</h2>{rows.length ? <dl className="grid grid-cols-[minmax(100px,auto)_1fr] gap-x-4 gap-y-2 text-[12px]">{rows.map(([key, value]) => <><dt key={`${key}-key`} className="text-muted">{key}</dt><dd key={`${key}-value`} className="min-w-0 break-all font-mono">{value || "—"}</dd></>)}</dl> : <p className="m-0 text-[12px] text-muted">None</p>}</div>; }
function Raw({ value }: { value: unknown }) { const { push } = useToast(); async function copy() { try { await navigator.clipboard.writeText(JSON.stringify(value, null, 2)); push("Inspect JSON copied."); } catch { push("Could not copy JSON. Select it and copy manually.", "error"); } } return <div className="rounded border border-line bg-panel p-4 md:col-span-2"><div className="mb-2 flex items-center"><h2 className="m-0 text-[14px]">Inspect</h2><Button className="ml-auto" onClick={() => void copy()}>Copy JSON</Button></div><pre className="max-h-[340px] overflow-auto rounded bg-ink p-3 text-[12px] text-[#D7E7EA]">{JSON.stringify(value, null, 2)}</pre></div>; }
