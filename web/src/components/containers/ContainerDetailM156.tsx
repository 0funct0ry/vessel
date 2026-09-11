import { useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Can } from "../../auth/Can";
import { api } from "../../lib/api";
import { flagState } from "../../lib/containers";
import type { ContainerDetail, Top } from "../../types/api";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { Flag } from "../ui/Flag";
import { CommitContainerModal } from "./CommitContainerModal";
import { FileBrowser } from "./FileBrowser";
import { LogViewer } from "./LogViewer";

const tabs = ["overview", "logs", "stats", "console", "files", "processes", "inspect"] as const;
type Tab = typeof tabs[number];

export function M156ContainerDetailPage() {
  const { id = "" } = useParams(); const [params, setParams] = useSearchParams(); const requested = params.get("tab"); const initial = tabs.includes(requested as Tab) ? requested as Tab : "overview"; const [tab, setTab] = useState<Tab>(initial); const [commitOpen, setCommitOpen] = useState(false);
  const detail = useQuery({ queryKey: ["container", id], queryFn: () => api.get<ContainerDetail>(`/containers/${encodeURIComponent(id)}`) });
  const c = detail.data;
  const processes = useQuery({ queryKey: ["container", id, "top"], queryFn: () => api.get<Top>(`/containers/${encodeURIComponent(id)}/top`), enabled: tab === "processes" && c?.state === "running", refetchInterval: 3000 });
  if (detail.isLoading) return <EmptyState title="Loading container" action="Contacting Docker…" />;
  if (!c) return <EmptyState title="Container not found" action="Return to the containers list and refresh." />;
  return <section><div className="mb-3 flex items-center gap-3"><Flag state={flagState(c.state)} /><h1 className="m-0 text-lg">{c.name}</h1><span className="font-mono text-[12px] text-muted">{c.id.slice(0, 12)}</span><span className="flex-1" /><Can do="containers.commit"><Button onClick={() => setCommitOpen(true)}>Commit to image</Button></Can></div><div role="tablist" className="mb-4 flex gap-1 border-b border-line">{tabs.map(name => <button key={name} role="tab" aria-selected={tab === name} onClick={() => { setTab(name); setParams(name === "overview" ? {} : { tab: name }, { replace: true }); }} className={`px-3 py-2 text-[13px] capitalize ${tab === name ? "border-b-2 border-hull font-medium text-text" : "text-muted"}`}>{name}</button>)}</div>{tab === "overview" && <Overview c={c} />}{tab === "logs" && <LogViewer containerID={c.id} />}{tab === "files" && <FileBrowser containerID={c.id} />}{tab === "processes" && <Processes running={c.state === "running"} data={processes.data} loading={processes.isLoading} />}{["stats", "console"].includes(tab) && <EmptyState title={`${tab[0].toUpperCase() + tab.slice(1)} is not built yet`} action={tab === "console" ? "The interactive terminal lands in M19." : "The full stats experience lands in M14."} />}{tab === "inspect" && <Inspect raw={c.raw} />}{commitOpen && <CommitContainerModal containerID={c.id} close={() => setCommitOpen(false)} />}</section>;
}

function Overview({ c }: { c: ContainerDetail }) { const env = c.env.map(entry => { const at = entry.indexOf("="); return [at < 0 ? entry : entry.slice(0, at), at < 0 ? "" : entry.slice(at + 1)] as const; }); return <div className="grid gap-3 md:grid-cols-2"><Info title="Configuration" rows={[["Image", c.image], ["Command", c.command.join(" ") || "—"], ["Created", c.created], ["Restart policy", c.restart_policy || "—"], ["Health", c.health || "not configured"], ["Exit code", String(c.exit_code)]]} /><Info title="Environment" rows={env.map(([key]) => [key, "••••••••••••"])} /><Info title="Mounts" rows={c.mounts.map(m => [m.destination, `${m.name || m.source} · ${m.rw ? "rw" : "ro"}`])} /><Info title="Networks" rows={Object.entries(c.networks).map(([name, n]) => [name, n.ip_address || "—"])} /></div>; }
function Info({ title, rows }: { title: string; rows: readonly (readonly [string, string])[] }) { return <div className="rounded border border-line bg-panel p-4"><h2 className="m-0 mb-3 text-[14px]">{title}</h2>{rows.length ? <dl className="grid grid-cols-[minmax(100px,auto)_1fr] gap-x-4 gap-y-2 text-[12px]">{rows.map(([key, value]) => <><dt key={`${key}-k`} className="text-muted">{key}</dt><dd key={`${key}-v`} className="min-w-0 break-all font-mono">{value}</dd></>)}</dl> : <p className="text-[12px] text-muted">None</p>}</div>; }
function Processes({ running, data, loading }: { running: boolean; data?: Top; loading: boolean }) { if (!running) return <EmptyState title="Process list is only available for running containers." action="Start the container, then return to this tab." />; if (loading) return <EmptyState title="Loading processes" action="Requesting a docker top snapshot…" />; return <div><div className="mb-2 text-[13px] text-muted">Snapshot from <span className="font-mono">docker top</span> · refreshes every few seconds</div><div className="overflow-x-auto rounded border border-line bg-panel"><table className="w-full min-w-max border-collapse text-[13px] font-mono"><thead className="border-b border-line bg-paper text-left text-[11.5px] font-sans uppercase tracking-wide text-muted"><tr>{(data?.titles ?? []).map(title => <th key={title} className="px-3 py-[7px]">{title}</th>)}</tr></thead><tbody>{(data?.processes ?? []).map((row, i) => <tr key={i} className="h-row border-b border-linesoft last:border-0 hover:bg-paper">{row.map((cell, j) => <td key={j} className="whitespace-nowrap px-3">{cell}</td>)}</tr>)}</tbody></table>{(data?.processes.length ?? 0) === 0 && <div className="p-8 text-center text-[13px] text-muted">No processes found.</div>}</div></div>; }
function Inspect({ raw }: { raw: unknown }) { return <div><div className="mb-2 text-[13px] text-muted">Full engine response</div><pre className="max-h-[65vh] overflow-auto rounded border border-line bg-ink p-4 text-[12px] text-[#D7E7EA]">{JSON.stringify(raw, null, 2)}</pre></div>; }
