import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import { bytes, flagState } from "../lib/containers";
import { useSSE, type SSEStatus } from "../lib/sse";
import { usePrefersReducedMotion } from "../lib/useReducedMotion";
import type { Container, DockerEvent, Host, Stats } from "../types/api";
import { EmptyState } from "../components/ui/EmptyState";
import { Flag } from "../components/ui/Flag";
import { Can } from "../auth/Can";
import { ArrowDown, ArrowUp, Eye, Trash2, X } from "lucide-react";

type Metric = Stats & { history: number[] };
const eventTypes = ["All", "Containers", "Images", "Volumes", "Networks"] as const;
type EventType = typeof eventTypes[number];

function relativeTime(value: string): string {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

function Sparkline({ values }: { values: number[] }) {
  if (values.length < 2) return <span className="text-[12px] text-muted">collecting…</span>;
  const width = 120; const height = 22; const max = Math.max(1, ...values) * 1.25;
  const points = values.map((value, index) => `${index * width / (values.length - 1)},${height - value / max * height}`).join(" ");
  return <svg viewBox={`0 0 ${width} ${height}`} width="100%" height={height} preserveAspectRatio="none" aria-label="CPU over the last minute"><polygon points={`0,${height} ${points} ${width},${height}`} fill="#1F5E7E" opacity=".13" /><polyline points={points} fill="none" stroke="#1F5E7E" strokeWidth="1.5" /></svg>;
}

function MetricProbe({ id, report }: { id: string; report: (id: string, value: Stats) => void }) {
  useSSE(`/containers/${encodeURIComponent(id)}/stats?interval=2s`, { stats: raw => report(id, JSON.parse(raw) as Stats) });
  return null;
}

function statusLabel(status: SSEStatus): string { return status === "open" ? "streaming" : status === "reconnecting" ? "reconnecting" : "connecting"; }

function eventSubject(event: DockerEvent): string { return event.name || event.attrs.image || event.id.slice(0, 12) || "—"; }
function eventActionName(event: DockerEvent): string { return event.action.split(":", 1)[0]; }
function eventDetail(event: DockerEvent): string {
  if (event.type === "container" && event.action === "die") return event.attrs.exitCode ? `exit code ${event.attrs.exitCode}` : "container exited";
  if (event.action.includes("health_status")) return event.action.split(":").slice(1).join(":").trim() || event.attrs.health || "health changed";
  return event.attrs.image || event.attrs.driver || event.attrs.container || event.attrs.from || "—";
}
function eventLink(event: DockerEvent): string | null {
  if (event.type === "container" && event.id) return `/containers/${encodeURIComponent(event.id)}`;
  if (event.type === "image" && event.id) return `/images/${encodeURIComponent(event.id)}`;
  if (event.type === "volume" && (event.name || event.id)) return `/volumes/${encodeURIComponent(event.name || event.id)}`;
  if (event.type === "network" && event.id) return `/networks/${encodeURIComponent(event.id)}`;
  return null;
}

function useDockerEvents(since?: number, limit = 50) {
  const [events, setEvents] = useState<DockerEvent[]>([]);
  const query = new URLSearchParams({ limit: String(limit) }); if (since) query.set("since", String(since));
  const status = useSSE(`/events?${query}`, { docker: raw => setEvents(old => { const next = JSON.parse(raw) as DockerEvent; return old.some(e => e.event_id === next.event_id) ? old : [next, ...old]; }) });
  return { events, status, setEvents };
}

export function DashboardPage() {
  const host = useQuery({ queryKey: ["host"], queryFn: () => api.get<Host>("/host"), refetchInterval: 5000 });
  const containers = useQuery({ queryKey: ["dashboard-containers"], queryFn: () => api.get<Container[]>("/containers?all=true"), refetchInterval: 5000 });
  const eventSince = useRef(Math.floor(Date.now() / 1000) - 3600).current;
  const { events } = useDockerEvents(eventSince);
  const [metrics, setMetrics] = useState<Record<string, Metric>>({});
  const rows = useMemo(() => containers.data ?? [], [containers.data]);
  const report = (id: string, stat: Stats) => setMetrics(old => ({ ...old, [id]: { ...stat, history: [...(old[id]?.history ?? []), stat.cpu_pct ?? 0].slice(-30) } }));
  const busiest = useMemo(() => rows.filter(c => c.state === "running").sort((a, b) => (metrics[b.id]?.cpu_pct ?? -1) - (metrics[a.id]?.cpu_pct ?? -1)).slice(0, 5), [rows, metrics]);
  const attention = useMemo(() => {
    const hourAgo = Date.now() - 60 * 60 * 1000;
    const failed = new Map(events.filter(e => e.type === "container" && e.action === "die" && Number(e.attrs.exitCode) !== 0 && new Date(e.timestamp).getTime() >= hourAgo).map(e => [e.id, e]));
    return rows.filter(c => c.state === "restarting" || /unhealthy|health:\s*starting/i.test(c.status) || failed.has(c.id)).slice(0, 4);
  }, [rows, events]);
  if (host.isLoading || containers.isLoading) return <EmptyState title="Loading dashboard" action="Contacting Docker…" />;
  if (!host.data || !containers.data) return <EmptyState title="Dashboard is unavailable" action="Refresh the page after Docker is reachable." />;
  if (host.data.containers.total === 0) return <EmptyState title="No containers on this host" action="Create a container from the Containers page, or run one with Docker, then return here." />;
  const h = host.data; const diskTotal = Math.max(1, h.disk.images + h.disk.volumes + h.disk.build_cache + h.disk.containers);
  return <section className="grid grid-cols-12 gap-3.5">{rows.filter(c => c.state === "running").map(c => <MetricProbe key={c.id} id={c.id} report={report} />)}
    <article className="col-span-12 rounded-md border border-line bg-panel p-3.5 md:col-span-4"><h2 className="m-0 mb-2.5 text-[13px] font-semibold">Containers</h2><div className="flex items-end gap-[22px]"><StateFigure count={h.containers.running} state="running" label="running" /><StateFigure count={h.containers.stopped} state="stopped" label="stopped" /><StateFigure count={h.containers.paused} state="paused" label="paused" /></div></article>
    <article className="col-span-12 rounded-md border border-line bg-panel p-3.5 md:col-span-4"><h2 className="m-0 mb-2.5 text-[13px] font-semibold">Host</h2><dl className="grid grid-cols-[auto_1fr] gap-x-3.5 gap-y-1 text-[13px]"><dt className="text-muted">Engine</dt><dd className="m-0 font-mono text-[12.5px]">{h.server_version}</dd><dt className="text-muted">Kernel</dt><dd className="m-0 font-mono text-[12.5px]">{h.kernel_version}</dd><dt className="text-muted">OS</dt><dd className="m-0 font-mono text-[12.5px]">{h.operating_system} · {h.architecture}</dd><dt className="text-muted">CPU</dt><dd className="m-0 font-mono text-[12.5px]">{h.cpus} cores · {h.cpu_pct.toFixed(0)}% used</dd><dt className="text-muted">Memory</dt><dd className="m-0 font-mono text-[12.5px]">{bytes(h.memory.used)} / {bytes(h.memory.limit || h.memory_bytes)}</dd></dl></article>
    <article className="col-span-12 rounded-md border border-line bg-panel p-3.5 md:col-span-4"><h2 className="m-0 mb-2.5 text-[13px] font-semibold">Disk</h2><div className="flex h-2 overflow-hidden rounded-[1px] bg-[#E6ECEB]"><span style={{ width: `${h.disk.images / diskTotal * 100}%`, background: "#12303F" }} /><span style={{ width: `${h.disk.volumes / diskTotal * 100}%`, background: "#24505F" }} /><span style={{ width: `${h.disk.build_cache / diskTotal * 100}%`, background: "#C8860D" }} /></div><div className="mt-2 flex flex-wrap gap-x-3.5 gap-y-1 text-[12px] text-muted"><span><i className="mr-1 inline-block h-[9px] w-[9px] bg-hull" />Images {bytes(h.disk.images)}</span><span><i className="mr-1 inline-block h-[9px] w-[9px] bg-steel" />Volumes {bytes(h.disk.volumes)}</span><span><i className="mr-1 inline-block h-[9px] w-[9px] bg-pause" />Build cache {bytes(h.disk.build_cache)}</span></div><p className="mb-0 mt-2.5 text-[12.5px] text-muted">{bytes(h.disk.reclaimable)} reclaimable. <Link to="/images">Prune images</Link></p></article>
    <article className="col-span-12 rounded-md border border-line bg-panel p-3.5 md:col-span-8"><h2 className="m-0 mb-2.5 text-[13px] font-semibold">Busiest containers</h2><div className="overflow-x-auto rounded border border-line"><table className="w-full border-collapse text-[13px]"><thead className="border-b border-line bg-paper text-left text-[11.5px] text-muted"><tr><th className="px-3 py-[7px]">Container</th><th className="px-3 py-[7px]">CPU</th><th className="w-[130px] px-3 py-[7px]">Last 60s</th><th className="px-3 py-[7px] text-right">Memory</th><th className="px-3 py-[7px] text-right">Net RX/TX</th></tr></thead><tbody>{busiest.map(c => { const m = metrics[c.id]; return <tr key={c.id} className="h-row border-b border-linesoft last:border-0 hover:bg-paper"><td className="px-3 font-semibold"><Link className="no-underline hover:underline" to={`/containers/${c.id}`}>{c.name}</Link></td><td className="px-3 font-mono text-[12.5px]">{m?.cpu_pct == null ? "—" : `${m.cpu_pct.toFixed(1)}%`}</td><td className="px-3 py-1"><Sparkline values={m?.history ?? []} /></td><td className="px-3 text-right font-mono text-[12.5px] text-muted">{m ? bytes(m.mem.used) : "—"}</td><td className="px-3 text-right font-mono text-[12.5px] text-muted">{m ? `${bytes(m.net.rx)}/s / ${bytes(m.net.tx)}/s` : "—"}</td></tr>; })}</tbody></table></div></article>
    <article className="col-span-12 rounded-md border border-line bg-panel p-3.5 md:col-span-4"><h2 className="m-0 mb-2.5 text-[13px] font-semibold">Needs attention</h2><div>{attention.length ? attention.map(c => <div key={c.id} className="flex items-center gap-2.5 border-b border-linesoft py-[7px] text-[13px] last:border-0"><Flag state={flagState(c.state)} label="" /><Link to={`/containers/${c.id}?tab=logs`}>{c.name}</Link><span className="ml-auto text-[12.5px] text-muted">{c.status}</span></div>) : <p className="m-0 py-[7px] text-[13px] text-muted">No containers need attention.</p>}</div></article>
  </section>;
}

function StateFigure({ count, state, label }: { count: number; state: "running" | "stopped" | "paused"; label: string }) { return <div className="flex flex-col gap-1"><b className={`font-mono text-[26px] font-medium leading-none ${state === "running" ? "text-run" : state === "paused" ? "text-pause" : "text-muted"}`}>{count}</b><Flag state={state} label={label} /></div>; }

export function EventsPage() {
  const { events, status, setEvents } = useDockerEvents(undefined, 5000); const [filter, setFilter] = useState<EventType>("All"); const [subject, setSubject] = useState(""); const [search, setSearch] = useState(""); const [timeDirection, setTimeDirection] = useState<"asc" | "desc">("desc"); const reducedMotion = usePrefersReducedMotion(); const table = useRef<HTMLDivElement>(null); const [detail, setDetail] = useState<DockerEvent | null>(null); const [confirm, setConfirm] = useState<{ kind: "one" | "all"; event?: DockerEvent } | null>(null); const [busy, setBusy] = useState(false);
  const subjects = useMemo(() => [...new Set(events.map(eventSubject).filter(Boolean))].sort(), [events]);
  const visible = useMemo(() => events.filter(event => { const haystack = `${event.type} ${event.action} ${eventSubject(event)} ${eventDetail(event)} ${JSON.stringify(event.attrs)}`.toLowerCase(); return (filter === "All" || event.type === filter.slice(0, -1).toLowerCase()) && (!subject || eventSubject(event) === subject) && (!search.trim() || haystack.includes(search.trim().toLowerCase())); }).sort((left, right) => { const leftTime = Date.parse(left.timestamp); const rightTime = Date.parse(right.timestamp); const difference = (Number.isNaN(leftTime) ? 0 : leftTime) - (Number.isNaN(rightTime) ? 0 : rightTime); const direction = timeDirection === "desc" ? -1 : 1; return difference !== 0 ? difference * direction : (left.event_id ?? left.id).localeCompare(right.event_id ?? right.id) * direction; }), [events, filter, search, subject, timeDirection]);
  useEffect(() => { if (!reducedMotion && timeDirection === "desc" && table.current && table.current.scrollTop < 16) table.current.scrollTop = 0; }, [events, reducedMotion, timeDirection]);
  const performDelete = async () => { if (!confirm) return; setBusy(true); try { if (confirm.kind === "all") { await api.delete("/events"); setEvents([]); } else if (confirm.event?.event_id) { await api.delete(`/events/${encodeURIComponent(confirm.event.event_id)}`); setEvents(old => old.filter(e => e.event_id !== confirm.event?.event_id)); } setConfirm(null); } finally { setBusy(false); } };
  return <section><div className="mb-3 flex flex-wrap items-center gap-2"><div className="flex overflow-hidden rounded border border-line">{eventTypes.map(type => <button key={type} onClick={() => setFilter(type)} className={`border-r border-line px-3 py-[5px] text-[13px] last:border-r-0 ${filter === type ? "bg-hull text-white" : "bg-panel"}`}>{type}</button>)}</div><input value={search} onChange={e => setSearch(e.target.value)} type="search" placeholder="Search events, subjects, details" aria-label="Search events" className="min-w-[240px] rounded border border-line bg-panel px-2 py-1.5 text-[13px]" /><select value={subject} onChange={e => setSubject(e.target.value)} aria-label="Filter by subject" className="rounded border border-line bg-panel px-2 py-1.5 text-[13px]"><option value="">All subjects</option>{subjects.map(value => <option key={value} value={value}>{value}</option>)}</select><span className="flex-1" /><Can do="events.clear"><button onClick={() => setConfirm({ kind: "all" })} className="rounded border border-line bg-panel px-3 py-[5px] text-[13px] text-fail">Clear events</button></Can><span className="flex items-center gap-1.5 text-[12px] text-muted"><i className="inline-block h-[7px] w-[7px] rounded-full bg-run" />{statusLabel(status)}</span></div><div ref={table} className="max-h-[calc(100vh-150px)] overflow-auto rounded border border-line bg-panel"><table className="w-full min-w-[650px] border-collapse text-[13px]"><thead className="sticky top-0 z-10 border-b border-line bg-paper text-left text-[11.5px] text-muted"><tr><th aria-sort={timeDirection === "desc" ? "descending" : "ascending"} className="w-[90px] px-3 py-[7px]"><button onClick={() => setTimeDirection(direction => direction === "desc" ? "asc" : "desc")} className="inline-flex items-center gap-1 font-medium hover:text-text" title={`Sort ${timeDirection === "desc" ? "oldest first" : "newest first"}`}>Time {timeDirection === "desc" ? <ArrowDown size={13} aria-hidden="true" /> : <ArrowUp size={13} aria-hidden="true" />}</button></th><th className="w-[180px] px-3 py-[7px]">Event</th><th className="px-3 py-[7px]">Subject</th><th className="px-3 py-[7px]">Detail</th><th className="w-[80px] px-3 py-[7px]" /></tr></thead><tbody>{visible.map((event, index) => { const link = eventLink(event); const eventSubjectName = eventSubject(event); return <tr key={event.event_id || `${event.timestamp}-${event.id}-${index}`} className="h-row border-b border-linesoft last:border-0 hover:bg-paper"><td className="px-3 font-mono text-[12.5px] text-muted">{relativeTime(event.timestamp)}</td><td className="px-3 font-mono text-[12.5px]">{event.type}.{eventActionName(event)}</td><td className="px-3 font-semibold">{link ? <Link className="no-underline hover:underline" to={link}>{eventSubjectName}</Link> : eventSubjectName}</td><td className="max-w-[420px] truncate px-3 text-muted">{eventDetail(event)}</td><td className="px-3 text-right"><button onClick={() => setDetail(event)} aria-label={`Show ${event.type} event`} title="Show event details" className="rounded p-1.5 text-muted hover:bg-paper hover:text-text"><Eye size={15} /></button><Can do="events.delete"><button onClick={() => setConfirm({ kind: "one", event })} aria-label={`Remove ${event.type} event`} title="Remove event" className="rounded p-1.5 text-fail hover:bg-paper" disabled={!event.event_id}><Trash2 size={15} /></button></Can></td></tr>; })}</tbody></table>{visible.length === 0 && <EmptyState title="No matching events" action="Clear the search or choose a different event or subject filter." />}</div>{detail && <EventDetailsModal event={detail} close={() => setDetail(null)} />}{confirm && <EventModal title={confirm.kind === "all" ? "Clear all events?" : "Remove event?"} close={() => !busy && setConfirm(null)}><p className="m-0 text-[13px] text-muted">{confirm.kind === "all" ? "This permanently removes every stored event." : "This permanently removes the selected stored event."}</p><div className="mt-5 flex justify-end gap-2"><button disabled={busy} onClick={() => setConfirm(null)} className="rounded border border-line bg-panel px-3 py-1.5 text-[13px]">Cancel</button><button disabled={busy} onClick={() => void performDelete()} className="rounded border border-fail bg-fail px-3 py-1.5 text-[13px] text-white">{busy ? "Removing…" : "Remove"}</button></div></EventModal>}</section>;
}

function EventModal({ title, close, children }: { title: string; close: () => void; children: ReactNode }) { return <div role="dialog" aria-modal="true" aria-labelledby="event-modal-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4"><div className="w-full max-w-2xl rounded-md border border-line bg-panel shadow-lg"><div className="flex items-center border-b border-line px-5 py-3"><h2 id="event-modal-title" className="m-0 text-[15px] font-semibold">{title}</h2><button onClick={close} aria-label="Close dialog" className="ml-auto rounded p-1 text-muted hover:bg-paper"><X size={18} /></button></div><div className="p-5">{children}</div></div></div>; }

function EventDetailsModal({ event, close }: { event: DockerEvent; close: () => void }) { const [tab, setTab] = useState<"details" | "raw">("details"); const rows = [["Event", `${event.type}.${eventActionName(event)}`], ["Time", new Date(event.timestamp).toLocaleString()], ["Subject", eventSubject(event)], ["Subject ID", event.id], ["Event ID", event.event_id || "pending persistence"]]; return <EventModal title={`${event.type}.${eventActionName(event)}`} close={close}><div className="mb-4 flex border-b border-line" role="tablist"><button role="tab" aria-selected={tab === "details"} onClick={() => setTab("details")} className={`px-3 py-2 text-[13px] ${tab === "details" ? "border-b-2 border-hull font-medium" : "text-muted"}`}>Details</button><button role="tab" aria-selected={tab === "raw"} onClick={() => setTab("raw")} className={`px-3 py-2 text-[13px] ${tab === "raw" ? "border-b-2 border-hull font-medium" : "text-muted"}`}>Raw JSON</button></div>{tab === "details" ? <div className="grid gap-4"><dl className="grid grid-cols-[auto_1fr] gap-x-5 gap-y-2 text-[13px]">{rows.map(([label, value]) => <><dt key={`${label}-dt`} className="text-muted">{label}</dt><dd key={`${label}-dd`} className="m-0 break-all font-mono text-[12px]">{value}</dd></>)}</dl><div><h3 className="m-0 mb-2 text-[13px] font-semibold">Attributes</h3><dl className="grid grid-cols-[auto_1fr] gap-x-5 gap-y-2 text-[13px]">{Object.entries(event.attrs).length ? Object.entries(event.attrs).map(([key, value]) => <><dt key={`${key}-dt`} className="font-mono text-[12px] text-muted">{key}</dt><dd key={`${key}-dd`} className="m-0 break-all font-mono text-[12px]">{value}</dd></>) : <dd className="m-0 text-muted">None</dd>}</dl></div></div> : <pre className="max-h-[60vh] overflow-auto rounded bg-ink p-4 text-[12px] text-[#D7E7EA]">{JSON.stringify(event, null, 2)}</pre>}</EventModal>; }
