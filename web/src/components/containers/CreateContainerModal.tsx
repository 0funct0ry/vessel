import { useEffect, useRef, useState, type Dispatch, type ReactNode, type SetStateAction } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "../ui/Button";
import { useToast } from "../ui/Toast";
import { api } from "../../lib/api";
import type { ContainerDetail, CreateContainerResponse, Image, Network, Volume } from "../../types/api";

type Pair = { key: string; value: string };
type Port = { container: string; host: string; protocol: string };
type Mount = { source: string; target: string; type: "volume" | "bind"; ro: boolean };
const input = "mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 text-[13px]";
const monoInput = `${input} font-mono`;
const words = (value: string) => value.trim() ? value.trim().split(/\s+/) : undefined;
const macAddressRE = /^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$/;

function pairsFromEnv(env: string[]): Pair[] {
  return env.map((entry) => { const at = entry.indexOf("="); return { key: at < 0 ? entry : entry.slice(0, at), value: at < 0 ? "" : entry.slice(at + 1) }; });
}

export function CreateContainerModal({ image = "", existing, close }: { image?: string; existing?: ContainerDetail; close: () => void }) {
  const navigate = useNavigate(); const queryClient = useQueryClient(); const { push } = useToast();
  const editing = !!existing;
  const [name, setName] = useState(existing?.name ?? "");
  const [reference, setReference] = useState(existing?.image ?? image);
  const [entrypoint, setEntrypoint] = useState("");
  const [command, setCommand] = useState(existing?.command.join(" ") ?? "");
  const [env, setEnv] = useState<Pair[]>(existing ? (pairsFromEnv(existing.env).length ? pairsFromEnv(existing.env) : [{ key: "", value: "" }]) : [{ key: "", value: "" }]);
  const [ports, setPorts] = useState<Port[]>(existing?.ports.length ? existing.ports.map((p) => ({ container: String(p.private_port), host: p.public_port ? String(p.public_port) : "", protocol: p.type })) : [{ container: "", host: "", protocol: "tcp" }]);
  const [mounts, setMounts] = useState<Mount[]>(existing ? existing.mounts.map((m) => ({ source: m.name || m.source, target: m.destination, type: m.name ? "volume" as const : "bind" as const, ro: !m.rw })) : []);
  const primaryNetwork = existing ? Object.keys(existing.networks)[0] ?? "bridge" : "bridge";
  const [network, setNetwork] = useState(primaryNetwork);
  const [additionalNetworks, setAdditionalNetworks] = useState<string[]>(existing ? Object.keys(existing.networks).filter((n) => n !== primaryNetwork) : []);
  const [macAddress, setMacAddress] = useState("");
  const [restartPolicy, setRestartPolicy] = useState(existing?.restart_policy || "unless-stopped");
  const [labels, setLabels] = useState<Pair[]>(existing && Object.keys(existing.labels).length ? Object.entries(existing.labels).map(([key, value]) => ({ key, value })) : [{ key: "", value: "" }]);
  const [busy, setBusy] = useState(false); const [warnings, setWarnings] = useState<string[]>([]);
  const images = useQuery({ queryKey: ["images", "create-typeahead"], queryFn: () => api.get<Image[]>("/images") });
  const volumes = useQuery({ queryKey: ["volumes", "create"], queryFn: () => api.get<Volume[]>("/volumes") });
  const networks = useQuery({ queryKey: ["networks", "create"], queryFn: () => api.get<Network[]>("/networks") });
  const updatePair = (set: Dispatch<SetStateAction<Pair[]>>, index: number, field: keyof Pair, value: string) => set(rows => rows.map((row, i) => i === index ? { ...row, [field]: value } : row));
  const updatePort = (index: number, field: keyof Port, value: string) => setPorts(rows => rows.map((row, i) => i === index ? { ...row, [field]: value } : row));
  const updateMount = (index: number, field: keyof Mount, value: string | boolean) => setMounts(rows => rows.map((row, i) => i === index ? { ...row, [field]: value } as Mount : row));
  const macInvalid = macAddress.trim() !== "" && !macAddressRE.test(macAddress.trim());
  async function fillNextPort(index: number) {
    const from = Number(ports[index].host) || 1024;
    try { const result = await api.get<{ port: number }>(`/host/next-port?from=${from}`); updatePort(index, "host", String(result.port)); } catch { push("Could not find a free port.", "error"); }
  }
  async function submit(start: boolean) {
    setBusy(true); setWarnings([]);
    const body = {
      name: editing ? undefined : (name.trim() || undefined), image: reference.trim(), command: words(command), entrypoint: words(entrypoint),
      env: env.filter(x => x.key.trim()).map(x => `${x.key.trim()}=${x.value}`), ports: ports.filter(x => x.container.trim()).map(x => ({ ...x, container: x.container.trim(), host: x.host.trim() || undefined })),
      mounts: mounts.filter(x => x.source.trim() && x.target.trim()).map(x => ({ ...x, source: x.source.trim(), target: x.target.trim() })), network: network || undefined,
      additional_networks: additionalNetworks.filter((n) => n.trim()), mac_address: macAddress.trim() || undefined,
      restart_policy: restartPolicy === "no" ? undefined : restartPolicy, labels: Object.fromEntries(labels.filter(x => x.key.trim()).map(x => [x.key.trim(), x.value])), start,
    };
    try {
      const result = editing
        ? await api.post<CreateContainerResponse>(`/containers/${encodeURIComponent(existing.id)}/recreate`, body)
        : await api.post<CreateContainerResponse>("/containers", body);
      setWarnings(result.warnings ?? []); await queryClient.invalidateQueries({ queryKey: ["containers"] }); await queryClient.invalidateQueries({ queryKey: ["container", existing?.id] });
      if (result.start_error) push(`Container was ${editing ? "recreated" : "created"} but could not start: ${result.start_error}`, "error");
      else push(`${editing ? existing.name : (result.name || result.id.slice(0, 12))} ${editing ? "updated" : start ? "created and started" : "created"}.`);
      navigate(`/containers/${encodeURIComponent(result.id)}`); close();
    } catch (error) { push(`Could not ${editing ? "update" : "create"} container: ${error instanceof Error ? error.message : "try again"}.`, "error"); setBusy(false); }
  }
  const canSubmit = !busy && reference.trim() && !macInvalid;
  return <div role="dialog" aria-modal="true" aria-labelledby="create-container-title" className="fixed inset-0 z-50 flex items-start justify-center overflow-auto bg-ink/45 px-5 py-[5vh]">
    <div className="flex max-h-[90vh] w-full max-w-[920px] flex-col rounded-md bg-panel shadow-[0_20px_60px_rgba(11,31,42,.35)]">
      <div className="flex items-center gap-2 border-b border-line px-[18px] py-[14px]"><h2 id="create-container-title" className="m-0 text-[15px] font-semibold">{editing ? "Edit container" : "Create container"}</h2><button aria-label="Close" onClick={close} className="ml-auto rounded px-1 text-lg leading-none text-muted hover:bg-paper hover:text-text">×</button></div>
      <div className="overflow-auto px-[18px] py-4"><div className="grid gap-6 md:grid-cols-2">
        <div>
          <Section title="Identity">
            <label>Name{editing ? <input value={name} readOnly className={`${input} cursor-not-allowed opacity-70`} /> : <input value={name} onChange={e => setName(e.target.value)} placeholder="api-2" className={input} />}</label>
            {editing && <p className="mt-1 text-[12px] text-muted">Renaming isn't supported here — remove and recreate under a new name instead.</p>}
            <label className="mt-3 block">Image<ImageInput value={reference} onChange={setReference} options={(images.data ?? []).flatMap(image => image.repo_tags).filter(tag => tag !== "<none>:<none>")} /></label>
            <label className="mt-3 block">MAC address<input value={macAddress} onChange={e => setMacAddress(e.target.value)} placeholder="02:42:ac:11:00:02" className={`${monoInput} ${macInvalid ? "border-fail" : ""}`} /></label>
            {macInvalid && <p className="mt-1 text-[12px] text-fail">Must look like xx:xx:xx:xx:xx:xx.</p>}
          </Section>
          <Section title="Command"><label>Entrypoint<input value={entrypoint} onChange={e => setEntrypoint(e.target.value)} placeholder="(image default)" className={monoInput} /></label><label className="mt-3 block">Command<input value={command} onChange={e => setCommand(e.target.value)} placeholder="(image default)" className={monoInput} /></label></Section>
          <RepeatPairs title="Environment" rows={env} setRows={setEnv} update={updatePair} keyPlaceholder="KEY" valuePlaceholder="value" />
        </div>
        <div>
          <Section title="Ports">{ports.map((p, i) => <div key={i} className="mb-2 grid grid-cols-[1fr_1fr_auto_auto_auto] gap-2"><input value={p.container} onChange={e => updatePort(i, "container", e.target.value)} placeholder="Container Port" className={monoInput.replace("mt-1 ", "")} /><input value={p.host} onChange={e => updatePort(i, "host", e.target.value)} placeholder="Host Port" className={monoInput.replace("mt-1 ", "")} /><select value={p.protocol} onChange={e => updatePort(i, "protocol", e.target.value)} className="rounded border border-line bg-panel px-1 text-[13px]"><option>tcp</option><option>udp</option></select><button type="button" aria-label="Find next free port" title="Find next free port" onClick={() => void fillNextPort(i)} className="text-muted hover:text-link">⌕</button><button onClick={() => setPorts(rows => rows.filter((_, n) => n !== i))} aria-label="Remove port" className="text-muted hover:text-fail">×</button></div>)}<button onClick={() => setPorts(rows => [...rows, { container: "", host: "", protocol: "tcp" }])} className="text-[12px] text-link underline">Add port</button></Section>
          <Section title="Mounts">{mounts.map((m, i) => <div key={i} className="mb-3 grid grid-cols-[auto_1fr_auto] gap-2"><select value={m.type} onChange={e => updateMount(i, "type", e.target.value)} className="rounded border border-line bg-panel px-1 text-[12px]"><option value="volume">volume</option><option value="bind">bind</option></select>{m.type === "volume" ? <select value={m.source} onChange={e => updateMount(i, "source", e.target.value)} className="rounded border border-line bg-panel px-2 text-[13px]"><option value="">Select volume</option>{(volumes.data ?? []).map(v => <option key={v.name}>{v.name}</option>)}</select> : <input value={m.source} onChange={e => updateMount(i, "source", e.target.value)} placeholder="/host/path" className={monoInput.replace("mt-1 ", "")} />}<button onClick={() => setMounts(rows => rows.filter((_, n) => n !== i))} aria-label="Remove mount" className="text-muted hover:text-fail">×</button><span /><input value={m.target} onChange={e => updateMount(i, "target", e.target.value)} placeholder="/var/lib/app" className={monoInput.replace("mt-1 ", "")} /><label className="flex items-center gap-1 text-[12px]"><input type="checkbox" checked={m.ro} onChange={e => updateMount(i, "ro", e.target.checked)} />ro</label></div>)}<button onClick={() => setMounts(rows => [...rows, { source: "", target: "", type: "volume", ro: false }])} className="text-[12px] text-link underline">Add mount</button></Section>
          <Section title="Network & restart">
            <label>Network<select value={network} onChange={e => setNetwork(e.target.value)} className={input}><option value="bridge">bridge</option>{(networks.data ?? []).filter(n => n.id !== "bridge" && n.name !== network).map(n => <option key={n.id} value={n.id}>{n.name}</option>)}</select></label>
            <label className="mt-3 block">Restart policy<select value={restartPolicy} onChange={e => setRestartPolicy(e.target.value)} className={input}><option value="no">no</option><option value="unless-stopped">unless-stopped</option><option value="always">always</option><option value="on-failure">on-failure</option></select></label>
          </Section>
          <Section title="Additional networks">{additionalNetworks.map((n, i) => <div key={i} className="mb-2 grid grid-cols-[1fr_auto] gap-2"><select value={n} onChange={e => setAdditionalNetworks(rows => rows.map((row, j) => j === i ? e.target.value : row))} className="rounded border border-line bg-panel px-2 text-[13px]"><option value="">Select network</option>{(networks.data ?? []).filter(net => net.id !== network && net.name !== network).map(net => <option key={net.id} value={net.id}>{net.name}</option>)}</select><button onClick={() => setAdditionalNetworks(rows => rows.filter((_, j) => j !== i))} aria-label="Remove network" className="text-muted hover:text-fail">×</button></div>)}<button onClick={() => setAdditionalNetworks(rows => [...rows, ""])} className="text-[12px] text-link underline">Add network</button></Section>
          <RepeatPairs title="Labels" rows={labels} setRows={setLabels} update={updatePair} keyPlaceholder="key" valuePlaceholder="value" />
        </div>
      </div><p className="mt-3 border border-[#E3CB93] border-l-[3px] border-l-pause rounded-sm bg-[#FDF8EC] px-3 py-[9px] text-[13px]">Bind-mounting a sensitive host path (<code>/</code>, <code>/etc</code>, <code>/var/run/docker.sock</code>) hands the container the same access as the Docker socket itself.</p>{warnings.length > 0 && <div className="border-l-2 border-pause bg-paper p-2 text-[12px]">{warnings.map(w => <p key={w} className="m-0">{w}</p>)}</div>}</div>
      <div className="flex items-center gap-2 border-t border-line px-[18px] py-3"><span className="flex-1" /><Button disabled={busy} onClick={close}>Cancel</Button>{editing ? <Button variant="primary" disabled={!canSubmit} onClick={() => void submit(false)}>Update container</Button> : <><Button variant="primary" disabled={!canSubmit} onClick={() => void submit(false)}>Create</Button><Button variant="primary" disabled={!canSubmit} onClick={() => void submit(true)}>Create and start</Button></>}</div>
    </div>
  </div>;
}
function ImageInput({ value, onChange, options }: { value: string; onChange: (value: string) => void; options: string[] }) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const query = value.trim().toLowerCase();
  const suggestions = (query ? options.filter((o) => o.toLowerCase().includes(query) && o.toLowerCase() !== query) : options).slice(0, 8);

  useEffect(() => {
    if (!open) return;
    const onDocClick = (e: MouseEvent) => { if (root.current && !root.current.contains(e.target as Node)) setOpen(false); };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", onDocClick);
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("mousedown", onDocClick); document.removeEventListener("keydown", onKey); };
  }, [open]);

  return <div ref={root} className="relative">
    <input
      autoFocus
      value={value}
      onChange={(e) => { onChange(e.target.value); setOpen(true); }}
      onFocus={() => setOpen(true)}
      placeholder="ghcr.io/acme/api:1.4.2"
      role="combobox"
      aria-expanded={open}
      className={monoInput}
    />
    {open && suggestions.length > 0 && <ul role="listbox" className="absolute z-10 mt-1 max-h-52 w-full overflow-auto rounded border border-line bg-panel py-1 shadow-[0_8px_24px_rgba(11,31,42,.18)]">
      {suggestions.map((tag) => <li key={tag}>
        <button
          type="button"
          role="option"
          aria-selected={tag === value}
          onClick={() => { onChange(tag); setOpen(false); }}
          className="block w-full px-2 py-1.5 text-left font-mono text-[13px] hover:bg-paper"
        >
          {tag}
        </button>
      </li>)}
    </ul>}
  </div>;
}
function Section({ title, children }: { title: string; children: ReactNode }) { return <section className="mb-4 text-[13px]"><h3 className="m-0 mb-2 text-[13px] font-semibold">{title}</h3>{children}</section>; }
function RepeatPairs({ title, rows, setRows, update, keyPlaceholder, valuePlaceholder }: { title: string; rows: Pair[]; setRows: Dispatch<SetStateAction<Pair[]>>; update: (set: Dispatch<SetStateAction<Pair[]>>, index: number, field: keyof Pair, value: string) => void; keyPlaceholder: string; valuePlaceholder: string }) { return <Section title={title}>{rows.map((row, i) => <div key={i} className="mb-2 grid grid-cols-[1fr_1fr_auto] gap-2"><input value={row.key} onChange={e => update(setRows, i, "key", e.target.value)} placeholder={keyPlaceholder} className={monoInput.replace("mt-1 ", "")} /><input value={row.value} onChange={e => update(setRows, i, "value", e.target.value)} placeholder={valuePlaceholder} className={monoInput.replace("mt-1 ", "")} /><button onClick={() => setRows(old => old.filter((_, n) => n !== i))} aria-label={`Remove ${title} row`} className="text-muted hover:text-fail">×</button></div>)}<button onClick={() => setRows(old => [...old, { key: "", value: "" }])} className="text-[12px] text-link underline">Add row</button></Section>; }
