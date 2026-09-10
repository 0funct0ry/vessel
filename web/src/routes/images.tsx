import { useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Can } from "../auth/Can";
import { CreateContainerModal } from "../components/containers/CreateContainerModal";
import { Button } from "../components/ui/Button";
import { EmptyState } from "../components/ui/EmptyState";
import { useToast } from "../components/ui/Toast";
import { api, getToken } from "../lib/api";
import { basePath } from "../lib/basePath";
import { bytes } from "../lib/containers";
import { pullImage, streamSSE } from "../lib/pullStream";
import { recentPulls, rememberPull } from "../lib/recentPulls";
import type { DockerfileReconstruction, Host, HistoryLayer, Image, ImageDetail, PullEvent } from "../types/api";

function imagesQuery(input: { q: string; sort: string }): string {
  const p = new URLSearchParams();
  if (input.q.trim()) p.set("q", input.q.trim());
  if (input.sort) p.set("sort", input.sort);
  const query = p.toString();
  return `/images${query ? `?${query}` : ""}`;
}

function shortID(id: string): string {
  return id.replace(/^sha256:/, "").slice(0, 12);
}

function createdAt(epochSeconds: number): string {
  return new Date(epochSeconds * 1000).toLocaleString();
}

function displayTag(tag: string): string {
  return tag === "<none>:<none>" ? "<none>" : tag;
}

/** Splits a Docker RepoTags entry ("registry:5000/acme/api:1.4.2") into repository and tag. */
function splitRepoTag(full: string): [string, string] {
  const slash = full.lastIndexOf("/");
  const colon = full.indexOf(":", slash + 1);
  if (colon === -1) return [full, ""];
  return [full.slice(0, colon), full.slice(colon + 1)];
}

const ROW_ACTION = "rounded border border-transparent px-1.5 py-0.5 text-[12px] text-muted hover:border-line hover:bg-panel hover:text-text";
const ROW_ACTION_DANGER = "rounded border border-transparent px-1.5 py-0.5 text-[12px] text-muted hover:border-line hover:bg-panel hover:text-fail";

function CopyID({ id }: { id: string }) {
  const { push } = useToast();
  async function copy() {
    try {
      await navigator.clipboard.writeText(id);
      push("Image ID copied.");
    } catch {
      push("Could not copy ID. Select it and copy manually.", "error");
    }
  }
  return <button onClick={() => void copy()} title={id} className="font-mono text-[12px] text-link underline decoration-dotted">{shortID(id)}</button>;
}

function ImportModal({ close, done }: { close: () => void; done: () => void }) {
  const { push } = useToast();
  const [file, setFile] = useState<File | null>(null);
  const [dragging, setDragging] = useState(false);
  const [busy, setBusy] = useState(false);
  const [lines, setLines] = useState<string[]>([]);
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);
  const controller = useRef<AbortController | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  function choose(candidate: File | null) {
    if (!candidate || busy) return;
    if (!candidate.name.toLowerCase().endsWith(".tar")) {
      setResult({ ok: false, message: "Choose a .tar image archive." });
      return;
    }
    setFile(candidate); setResult(null); setLines([]);
  }
  async function load() {
    if (!file) return;
    setBusy(true); setResult(null); setLines([]);
    const form = new FormData(); form.append("tar", file);
    const ac = new AbortController(); controller.current = ac;
    try {
      await streamSSE("/images/import", form, {}, (name, data) => {
        if (name === "import") setLines((current) => [...current, String((data as { stream?: string }).stream ?? "").trimEnd()]);
        if (name === "done") {
          const images = (data as { images?: string[] }).images ?? [];
          setResult({ ok: true, message: images.length ? `Loaded ${images.join(", ")}.` : "Image archive loaded." });
          done();
        }
        if (name === "error") throw new Error(String((data as { message?: string }).message ?? "import failed"));
      }, ac.signal);
    } catch (e) {
      const message = ac.signal.aborted ? "Import cancelled." : e instanceof Error ? e.message : "import failed";
      setResult({ ok: false, message });
      if (!ac.signal.aborted) push(`Could not import image archive: ${message}`, "error");
    } finally { setBusy(false); controller.current = null; }
  }
  return <div role="dialog" aria-modal="true" aria-labelledby="import-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4" onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) close(); }}>
    <div className="w-full max-w-lg rounded border border-line bg-panel p-5 shadow-lg">
      <div className="flex items-center gap-3"><h2 id="import-title" className="m-0 text-lg">Import image archive</h2><button aria-label="Close import dialog" disabled={busy} onClick={close} className="ml-auto rounded px-2 text-xl text-muted hover:bg-paper hover:text-text">×</button></div>
      <p className="mt-1 text-[12px] text-muted">Drop a Docker image <code>.tar</code> archive here, or select one from your computer.</p>
      <input ref={fileInput} type="file" accept=".tar" aria-label="Image tarball" className="sr-only" onChange={(e) => choose(e.target.files?.[0] ?? null)} />
      <div
        role="button" tabIndex={0} aria-label="Choose image tarball" onClick={() => !busy && fileInput.current?.click()}
        onKeyDown={(e) => { if (!busy && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); fileInput.current?.click(); } }}
        onDragOver={(e) => { if (!busy) { e.preventDefault(); setDragging(true); } }} onDragLeave={() => setDragging(false)}
        onDrop={(e) => { e.preventDefault(); setDragging(false); choose(e.dataTransfer.files?.[0] ?? null); }}
        className={`mt-4 cursor-pointer rounded border-2 border-dashed px-5 py-8 text-center text-[13px] ${dragging ? "border-link bg-[#EAF2F4]" : "border-line bg-paper hover:border-steel"} ${busy ? "cursor-not-allowed opacity-60" : ""}`}>
        <span className="block font-medium text-text">{file ? file.name : "Drag and drop a .tar archive"}</span>
        <span className="mt-1 block text-muted">{file ? "Click to choose a different file" : "or click to browse files"}</span>
      </div>
      {lines.length > 0 && <dl className="mt-4 max-h-[180px] overflow-auto text-[12px]">{lines.map((line, index) => <div key={`${index}-${line}`} className="flex gap-3 border-b border-linesoft py-1"><dt className="font-mono text-muted">stream</dt><dd className="min-w-0 break-all">{line}</dd></div>)}</dl>}
      {result && <p className={`mt-3 text-[13px] ${result.ok ? "text-run" : "text-fail"}`} role="status">{result.ok ? "✓ " : ""}{result.message}</p>}
      <div className="mt-5 flex justify-end gap-2"><Button onClick={close} disabled={busy}>Close</Button>{busy ? <Button variant="danger" onClick={() => controller.current?.abort()}>Cancel</Button> : <Button variant="primary" disabled={!file || !!result?.ok} onClick={() => void load()}>Load</Button>}</div>
    </div>
  </div>;
}

type ContextFile = File & { webkitRelativePath?: string };

function BuildModal({ close, done }: { close: () => void; done: () => void }) {
  const { push } = useToast();
  const [tag, setTag] = useState("");
  const [dockerfile, setDockerfile] = useState("FROM alpine:3.20\n");
  const [files, setFiles] = useState<File[]>([]);
  const [lines, setLines] = useState<string[]>([]);
  const [step, setStep] = useState<{ current: number; total: number } | null>(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);
  const dockerfileInput = useRef<HTMLInputElement>(null);
  const contextInput = useRef<HTMLInputElement>(null);
  const controller = useRef<AbortController | null>(null);
  const readDockerfile = async (file: File | null) => {
    if (!file || busy) return;
    try { setDockerfile(await file.text()); setResult(null); } catch { setResult({ ok: false, message: "Could not read Dockerfile." }); }
  };
  const addFiles = (newFiles: FileList | null) => {
    if (!newFiles || busy) return;
    setFiles((current) => [...current, ...Array.from(newFiles)]);
    setResult(null);
  };
  async function build() {
    if (!tag.trim() || !dockerfile.trim()) return;
    setBusy(true); setResult(null); setLines([]); setStep(null);
    const form = new FormData();
    form.append("dockerfile", new Blob([dockerfile], { type: "text/plain" }), "Dockerfile");
    form.append("tags[]", tag.trim());
    files.forEach((file) => {
      const path = (file as ContextFile).webkitRelativePath || file.name;
      form.append("context_path[]", path);
      form.append("context", file, path);
    });
    const ac = new AbortController(); controller.current = ac;
    try {
      await streamSSE("/images/build", form, {}, (name, data) => {
        if (name === "build") {
          const event = data as { line?: string; step?: number; total_steps?: number };
          if (event.line) { const line = event.line; setLines((current) => [...current, line.trimEnd()]); }
          if (event.step && event.total_steps) setStep({ current: event.step, total: event.total_steps });
        }
        if (name === "done") { setResult({ ok: true, message: `Built ${(data as { image_id?: string }).image_id || tag.trim()}.` }); done(); }
        if (name === "error") throw new Error(String((data as { message?: string }).message ?? "build failed"));
      }, ac.signal);
    } catch (e) {
      const message = ac.signal.aborted ? "Build cancelled." : e instanceof Error ? e.message : "build failed";
      setResult({ ok: false, message });
      if (!ac.signal.aborted) push(`Could not build image: ${message}`, "error");
    } finally { setBusy(false); controller.current = null; }
  }
  return <div role="dialog" aria-modal="true" aria-labelledby="build-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4" onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) close(); }}>
    <div className="w-full max-w-2xl rounded border border-line bg-panel p-5 shadow-lg">
      <div className="flex items-center gap-3"><h2 id="build-title" className="m-0 text-lg">Build image</h2><button aria-label="Close build dialog" disabled={busy} onClick={close} className="ml-auto rounded px-2 text-xl text-muted hover:bg-paper hover:text-text">×</button></div>
      <div className="mt-4 flex flex-wrap items-end gap-2">
        <label className="min-w-[280px] flex-1 text-[13px]">Tag<input autoFocus disabled={busy} value={tag} onChange={(e) => setTag(e.target.value)} placeholder="ghcr.io/acme/api:1.4.4" className="mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 font-mono" /></label>
        <input ref={dockerfileInput} className="sr-only" type="file" accept=".dockerfile,Dockerfile,text/plain" onChange={(e) => void readDockerfile(e.target.files?.[0] ?? null)} />
        <input ref={contextInput} className="sr-only" type="file" multiple onChange={(e) => addFiles(e.target.files)} />
        <Button disabled={busy} onClick={() => dockerfileInput.current?.click()}>Upload Dockerfile</Button><Button disabled={busy} onClick={() => contextInput.current?.click()}>Add files</Button>
      </div>
      <textarea aria-label="Dockerfile" spellCheck={false} disabled={busy} value={dockerfile} onChange={(e) => setDockerfile(e.target.value)} className="mt-3 block h-48 w-full resize-y rounded border border-line bg-panel p-2.5 font-mono text-[12.5px]" />
      <div className="mt-2 flex flex-wrap gap-1.5 text-[12px]">{files.length === 0 ? <span className="text-muted">Dockerfile only</span> : files.map((file, index) => <span key={`${file.name}-${index}`} className="rounded border border-line bg-paper px-1.5 py-0.5 font-mono">{(file as ContextFile).webkitRelativePath || file.name}<button disabled={busy} aria-label={`Remove ${file.name}`} onClick={() => setFiles((current) => current.filter((_, i) => i !== index))} className="ml-1.5 text-muted hover:text-fail">×</button></span>)}</div>
      {(lines.length > 0 || step) && <div className="mt-3 rounded border border-line bg-paper p-2"><div className="max-h-48 overflow-auto whitespace-pre-wrap font-mono text-[12px]">{lines.map((line, index) => <div key={`${index}-${line}`}>{line}</div>)}</div>{step && <div className="mt-2 h-1.5 overflow-hidden rounded bg-panel"><div className="h-full bg-hull" style={{ width: `${Math.round(step.current / step.total * 100)}%` }} /></div>}</div>}
      {result && <p role="status" className={`mt-3 text-[13px] ${result.ok ? "text-run" : "text-fail"}`}>{result.ok ? "✓ " : ""}{result.message}</p>}
      <div className="mt-5 flex justify-end gap-2"><Button onClick={close} disabled={busy}>Close</Button>{busy ? <Button variant="danger" onClick={() => controller.current?.abort()}>Cancel</Button> : <Button variant="primary" disabled={!tag.trim() || !dockerfile.trim() || result?.ok} onClick={() => void build()}>Build</Button>}</div>
    </div>
  </div>;
}

function PullDialog({ close, done }: { close: () => void; done: () => void }) {
  const { push } = useToast();
  const [reference, setReference] = useState("");
  const [busy, setBusy] = useState(false);
  const [layers, setLayers] = useState<Record<string, PullEvent>>({});
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);
  const controller = useRef<AbortController | null>(null);
  const recent = recentPulls();

  async function start() {
    const ref = reference.trim();
    if (!ref) return;
    setBusy(true);
    setResult(null);
    setLayers({});
    const ac = new AbortController();
    controller.current = ac;
    try {
      await pullImage(ref, (name, data) => {
        if (name !== "pull") return;
        const event = data as PullEvent;
        setLayers((current) => ({ ...current, [event.id || event.status]: event }));
      }, ac.signal);
      setResult({ ok: true, message: `Pulled ${ref}.` });
      rememberPull(ref);
      done();
    } catch (e) {
      if (ac.signal.aborted) {
        setResult({ ok: false, message: "Pull cancelled." });
      } else {
        const message = e instanceof Error ? e.message : "pull failed";
        setResult({ ok: false, message });
        push(`Could not pull ${ref}: ${message}`, "error");
      }
    } finally {
      setBusy(false);
      controller.current = null;
    }
  }
  function cancel() {
    controller.current?.abort();
  }
  const entries = Object.values(layers);
  // Docker's terminal progress event for a layer ("Pull complete", "Already
  // exists") drops current/total, so treat any known-complete status as 100%
  // rather than excluding it from the average.
  const layerRatio = (l: PullEvent) => (l.total ? (l.current ?? 0) / l.total : /pull complete|already exists|download complete/i.test(l.status) ? 1 : 0);
  const withProgress = entries.filter((l) => l.total || layerRatio(l) > 0);
  const overallPct = result?.ok ? 100 : withProgress.length ? Math.round((withProgress.reduce((sum, l) => sum + layerRatio(l), 0) / withProgress.length) * 100) : 0;

  return <div role="dialog" aria-modal="true" aria-labelledby="pull-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4">
    <div className="w-full max-w-lg rounded border border-line bg-panel p-5 shadow-lg">
      <h2 id="pull-title" className="m-0 text-lg">Pull image</h2>
      <p className="mt-1 text-[12px] text-muted">Reference format: <code>repo[:tag]</code> or <code>registry.example.com/repo:tag</code>. Public registries only.</p>
      <input list="recent-pulls" autoFocus disabled={busy} value={reference} onChange={(e) => setReference(e.target.value)} placeholder="e.g. ghcr.io/acme/api:1.4.3" className="mt-3 block w-full rounded border border-line bg-panel px-2 py-1.5 font-mono text-[13px]" />
      <datalist id="recent-pulls">{recent.map((r) => <option key={r} value={r} />)}</datalist>
      {entries.length > 0 && <div className="mt-4">
        <dl className="max-h-[180px] overflow-auto text-[12px]">
          {entries.map((l) => <div key={l.id || l.status} className="flex justify-between gap-2 border-b border-linesoft py-1"><dt className="font-mono text-muted">{l.id || "—"}</dt><dd className="min-w-0 truncate">{l.error ? <span className="text-fail">{l.error}</span> : l.status}{l.total ? ` · ${bytes(l.current ?? 0)}/${bytes(l.total)}` : ""}</dd></div>)}
        </dl>
        <div className="mt-2 h-2 overflow-hidden rounded bg-paper"><div className="h-full bg-hull transition-[width]" style={{ width: `${overallPct}%` }} /></div>
      </div>}
      {result?.ok && <p className="mt-4 rounded border border-run/40 bg-run/10 px-3 py-2 text-[13px] text-run" role="status">✓ Pull complete — {result.message}</p>}
      {result && !result.ok && <p className="mt-3 text-[13px] text-fail" role="status">{result.message}</p>}
      <div className="mt-5 flex justify-end gap-2">
        <Button onClick={close}>Close</Button>
        {!result?.ok && (busy ? <Button variant="danger" onClick={cancel}>Cancel</Button> : <Button variant="primary" disabled={!reference.trim()} onClick={() => void start()}>Pull</Button>)}
      </div>
    </div>
  </div>;
}

function TagDialog({ image, close, done }: { image: Image; close: () => void; done: () => void }) {
  const { push } = useToast();
  const [repo, setRepo] = useState("");
  const [tag, setTag] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit() {
    setBusy(true);
    try {
      await api.post(`/images/${encodeURIComponent(image.id)}/tag`, { repo: repo.trim(), tag: tag.trim() || undefined });
      push(`Tagged ${repo.trim()}${tag.trim() ? `:${tag.trim()}` : ""}.`);
      done();
      close();
    } catch (e) {
      push(`Could not tag image: ${e instanceof Error ? e.message : "try again"}.`, "error");
      setBusy(false);
    }
  }
  return <div role="dialog" aria-modal="true" aria-labelledby="tag-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4">
    <div className="w-full max-w-md rounded border border-line bg-panel p-5 shadow-lg">
      <h2 id="tag-title" className="m-0 text-lg">Tag image</h2>
      <label className="mt-3 block text-[13px]">Repository<input autoFocus value={repo} onChange={(e) => setRepo(e.target.value)} placeholder="acme/api" className="mt-1 block w-full rounded border border-line px-2 py-1 font-mono" /></label>
      <label className="mt-3 block text-[13px]">Tag (optional)<input value={tag} onChange={(e) => setTag(e.target.value)} placeholder="latest" className="mt-1 block w-full rounded border border-line px-2 py-1 font-mono" /></label>
      <div className="mt-5 flex justify-end gap-2"><Button onClick={close}>Cancel</Button><Button variant="primary" disabled={!repo.trim() || busy} onClick={() => void submit()}>Tag</Button></div>
    </div>
  </div>;
}

function UntagDialog({ image, tag, close, done }: { image: Image; tag: string; close: () => void; done: () => void }) {
  const { push } = useToast();
  const [busy, setBusy] = useState(false);
  async function submit() {
    setBusy(true);
    try {
      await api.delete(`/images/${encodeURIComponent(tag)}`);
      push(`Untagged ${tag}.`);
      done();
      close();
    } catch (e) {
      push(`Could not untag ${tag}: ${e instanceof Error ? e.message : "try again"}.`, "error");
      setBusy(false);
    }
  }
  return <div role="dialog" aria-modal="true" aria-labelledby="untag-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4">
    <div className="w-full max-w-md rounded border border-line bg-panel p-5 shadow-lg">
      <h2 id="untag-title" className="m-0 text-lg">Remove tag {tag}?</h2>
      <p className="mt-2 text-[13px] text-muted">The underlying image ({shortID(image.id)}) stays until its last tag is removed.</p>
      <div className="mt-5 flex justify-end gap-2"><Button onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void submit()}>Remove tag</Button></div>
    </div>
  </div>;
}

function RemoveDialog({ image, close, done }: { image: Image; close: () => void; done: () => void }) {
  const { push } = useToast();
  const label = image.repo_tags[0] || shortID(image.id);
  const inUse = image.used_by_count > 0;
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const allowed = !inUse || typed === label;
  async function remove() {
    setBusy(true);
    try {
      await api.delete(`/images/${encodeURIComponent(image.id)}?force=${inUse}`);
      push(`Removed ${label}.`);
      done();
      close();
    } catch (e) {
      push(`Could not remove ${label}: ${e instanceof Error ? e.message : "try again"}.`, "error");
      setBusy(false);
    }
  }
  return <div role="dialog" aria-modal="true" aria-labelledby="remove-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4">
    <div className="w-full max-w-md rounded border border-line bg-panel p-5 shadow-lg">
      <h2 id="remove-title" className="m-0 text-lg">Remove {label}?</h2>
      {inUse ? <p className="mt-2 text-[13px] text-muted">This image is used by {image.used_by_count} container{image.used_by_count === 1 ? "" : "s"}. Removing it forces those references loose.</p> : <p className="mt-2 text-[13px] text-muted">This permanently deletes the image.</p>}
      {inUse && <label className="mt-3 block text-[13px]">Type <b>{label}</b> to confirm<input autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} className="mt-1 block w-full rounded border border-line px-2 py-1" /></label>}
      <div className="mt-5 flex justify-end gap-2"><Button onClick={close}>Cancel</Button><Button variant="danger" disabled={!allowed || busy} onClick={() => void remove()}>Remove</Button></div>
    </div>
  </div>;
}

function PruneDialog({ close, done }: { close: () => void; done: () => void }) {
  const { push } = useToast();
  const [busy, setBusy] = useState(false);
  const host = useQuery({ queryKey: ["host"], queryFn: () => api.get<Host>("/host") });
  const reclaimable = host.data?.disk.images_reclaimable ?? 0;
  async function prune() {
    setBusy(true);
    try {
      const report = await api.post<{ ImagesDeleted?: unknown[]; SpaceReclaimed?: number }>("/prune/images");
      push(`Pruned ${report.ImagesDeleted?.length ?? 0} image(s), reclaimed ${bytes(report.SpaceReclaimed ?? 0)}.`);
      done();
      close();
    } catch (e) {
      push(`Could not prune images: ${e instanceof Error ? e.message : "try again"}.`, "error");
      setBusy(false);
    }
  }
  return <div role="dialog" aria-modal="true" aria-labelledby="prune-title" className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4">
    <div className="w-full max-w-md rounded border border-line bg-panel p-5 shadow-lg">
      <h2 id="prune-title" className="m-0 text-lg">Prune unused images?</h2>
      <p className="mt-2 text-[13px] text-muted">Removes every image with no container reference. Estimated reclaim: <b>{bytes(reclaimable)}</b>.</p>
      <div className="mt-5 flex justify-end gap-2"><Button onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void prune()}>Prune</Button></div>
    </div>
  </div>;
}

function UsedByCell({ image }: { image: Image }) {
  return image.used_by_count > 0
    ? <span className="rounded-sm border border-[#BFD5CE] px-1 font-mono text-[11px] text-run">in use · {image.used_by_count}</span>
    : <span className="font-mono text-[11px] text-muted">—</span>;
}

export function ImagesPage() {
  const queryClient = useQueryClient();
  const { push } = useToast();
  const filterRef = useRef<HTMLInputElement>(null);
  const [q, setQ] = useState("");
  const [sort, setSort] = useState("created");
  const [grouped, setGrouped] = useState(false);
  const [pull, setPull] = useState(false);
  const [tag, setTag] = useState<Image | null>(null);
  const [untag, setUntag] = useState<{ image: Image; tag: string } | null>(null);
  const [remove, setRemove] = useState<Image | null>(null);
  const [prune, setPrune] = useState(false);
  const [runImage, setRunImage] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [importOpen, setImportOpen] = useState(false);
  const [buildOpen, setBuildOpen] = useState(false);

  const path = imagesQuery({ q, sort });
  const images = useQuery({ queryKey: ["images", q, sort], queryFn: () => api.get<Image[]>(path), refetchInterval: 5000 });
  const rows = images.data ?? [];
  const selectableRefs = Array.from(new Set(rows.flatMap((image) => (image.repo_tags.length ? image.repo_tags : [image.id]).map((tag) => tag === "<none>:<none>" ? image.id : tag))));
  const refresh = () => { void queryClient.invalidateQueries({ queryKey: ["images"] }); void queryClient.invalidateQueries({ queryKey: ["host"] }); };

  function toggleSelection(ref: string, checked: boolean) {
    setSelected((current) => { const next = new Set(current); if (checked) next.add(ref); else next.delete(ref); return next; });
  }
  function toggleAll(checked: boolean) { setSelected(checked ? new Set(selectableRefs) : new Set()); }
  async function exportImages() {
    const refs = selected.size ? [...selected] : selectableRefs;
    if (!refs.length) return;
    const params = new URLSearchParams(); refs.forEach((ref) => params.append("ref", ref));
    const base = basePath(); const token = getToken();
    const response = await fetch(`${base === "/" ? "" : base}/api/v1/images/export?${params}`, { headers: token ? { Authorization: `Bearer ${token}` } : {} });
    if (!response.ok) {
      let message = response.statusText;
      try { message = (await response.json()).error?.message ?? message; } catch { /* retain status */ }
      throw new Error(message || "export failed");
    }
    const blob = await response.blob();
    const match = /filename="?([^";]+)"?/i.exec(response.headers.get("Content-Disposition") ?? "");
    const link = document.createElement("a"); link.href = URL.createObjectURL(blob); link.download = match?.[1] ?? "vessel-images.tar";
    document.body.appendChild(link); link.click(); link.remove(); window.setTimeout(() => URL.revokeObjectURL(link.href), 0);
  }

  function actions(image: Image) {
    return <div className="flex justify-end gap-1 opacity-0 group-hover:opacity-100 focus-within:opacity-100">
      <Can do="containers.create"><button className={ROW_ACTION} onClick={() => setRunImage(image.repo_tags.find((tag) => tag !== "<none>:<none>") ?? "")}>run</button></Can>
      <Can do="images.tag"><button className={ROW_ACTION} onClick={() => setTag(image)}>tag</button></Can>
      <Can do="images.remove"><button className={ROW_ACTION_DANGER} onClick={() => setRemove(image)}>remove</button></Can>
    </div>;
  }
  function tagCell(image: Image, fullTag: string, tag: string) {
    const isNone = fullTag === "<none>:<none>";
    return <span className="flex items-center gap-2">
      <span className="font-mono">{isNone ? "<none>" : tag}</span>
      {image.dangling && <span className="rounded-sm border border-[#E3CB93] px-1 font-mono text-[11px] text-pause">dangling</span>}
      {!isNone && <Can do="images.tag"><button title="Remove this tag" onClick={() => setUntag({ image, tag: fullTag })} className={`${ROW_ACTION} opacity-0 group-hover:opacity-100 focus-within:opacity-100`}>untag</button></Can>}
    </span>;
  }

  return <section>
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <input ref={filterRef} type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter by repository or tag" aria-label="Filter images" className="min-w-[230px] rounded border border-line bg-panel px-2 py-1.5 text-[13px]" />
      <label className="ml-1 flex items-center gap-1.5 text-[13px]"><input type="checkbox" checked={grouped} onChange={(e) => setGrouped(e.target.checked)} /> group by image</label>
      <span className="flex-1" />
      <Can do="prune.run"><Button onClick={() => setPrune(true)}>Prune unused</Button></Can>
      <Can do="images.export"><Button onClick={() => void exportImages().catch((e: unknown) => { const message = e instanceof Error ? e.message : "export failed"; push(`Could not export images: ${message}`, "error"); })}>Export selected</Button></Can>
      <Can do="images.import"><Button onClick={() => setImportOpen(true)}>Import</Button></Can>
      <Can do="images.build"><Button onClick={() => setBuildOpen(true)}>Build image</Button></Can>
      <Can do="images.pull"><Button variant="primary" onClick={() => setPull(true)}>Pull image</Button></Can>
    </div>
    {importOpen && <ImportModal close={() => setImportOpen(false)} done={refresh} />}
    {buildOpen && <BuildModal close={() => setBuildOpen(false)} done={refresh} />}
    {selected.size > 0 && <div className="mb-2 flex items-center gap-2.5 rounded bg-hull px-3 py-2 text-[13px] text-white"><span>{selected.size} selected</span><span className="flex-1" /><Button className="border-white/35 bg-transparent text-white hover:bg-white/10" onClick={() => setSelected(new Set())}>Clear</Button></div>}
    <div className="overflow-x-auto rounded border border-line bg-panel">
      {images.isLoading ? <EmptyState title="Loading images" action="Contacting Docker…" /> : rows.length === 0 ? <EmptyState title="No images found" action="Pull an image, or broaden the current filter." /> : <table className="w-full min-w-[860px] border-collapse text-[13px]">
        <thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted">
          <tr>
            <th className="w-[26px] px-2"><input type="checkbox" aria-label="Select all images" checked={selectableRefs.length > 0 && selectableRefs.every((ref) => selected.has(ref))} onChange={(e) => toggleAll(e.target.checked)} /></th>
            <th className="px-2">Repository</th>
            <th className="px-2">Tag</th>
            <th className="px-2"><button onClick={() => setSort("name")}>Image ID</button></th>
            <th className="px-2 text-right">Size</th>
            <th className="px-2"><button onClick={() => setSort("created")}>Created</button></th>
            <th className="px-2">Used by</th>
            <th className="w-[110px] px-2" />
          </tr>
        </thead>
        <tbody>{rows.flatMap((image, imageIndex) => {
          const tags = image.repo_tags.length ? image.repo_tags : ["<none>:<none>"];
          const isLastImage = imageIndex === rows.length - 1;
          if (!grouped) {
            return tags.map((fullTag) => {
              const [repo, tag] = splitRepoTag(fullTag);
              const ref = fullTag === "<none>:<none>" ? image.id : fullTag;
              return <tr key={`${image.id}-${fullTag}`} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper">
                <td className="px-2 align-middle"><input type="checkbox" aria-label={`Select ${ref}`} checked={selected.has(ref)} onChange={(e) => toggleSelection(ref, e.target.checked)} /></td>
                <td className="max-w-[220px] truncate px-2 align-middle font-medium"><Link to={`/images/${encodeURIComponent(image.id)}`}>{repo}</Link></td>
                <td className="px-2 align-middle">{tagCell(image, fullTag, tag)}</td>
                <td className="px-2 align-middle"><CopyID id={image.id} /></td>
                <td className="px-2 text-right align-middle font-mono text-[12px]">{bytes(image.size)}</td>
                <td className="px-2 align-middle font-mono text-[12px]">{createdAt(image.created)}</td>
                <td className="px-2 align-middle"><UsedByCell image={image} /></td>
                <td className="px-2 text-right align-middle whitespace-nowrap">{actions(image)}</td>
              </tr>;
            });
          }
          const groupBorder = isLastImage ? "" : "border-b border-linesoft";
          return tags.map((fullTag, i) => {
            const [repo, tag] = splitRepoTag(fullTag);
            const ref = fullTag === "<none>:<none>" ? image.id : fullTag;
            const last = i === tags.length - 1;
            return <tr key={`${image.id}-${fullTag}`} className={`group h-row hover:bg-paper ${last ? groupBorder : ""}`}>
              <td className={`px-2 align-middle ${last ? "" : "border-b border-transparent"}`}><input type="checkbox" aria-label={`Select ${ref}`} checked={selected.has(ref)} onChange={(e) => toggleSelection(ref, e.target.checked)} /></td>
              <td className={`max-w-[220px] truncate px-2 align-middle font-medium ${last ? "" : "border-b border-transparent"}`}><Link to={`/images/${encodeURIComponent(image.id)}`}>{repo}</Link></td>
              <td className={`px-2 align-middle ${last ? "" : "border-b border-transparent"}`}>{tagCell(image, fullTag, tag)}</td>
              {i === 0 && <td rowSpan={tags.length} className={`px-2 align-middle ${groupBorder}`}><CopyID id={image.id} /></td>}
              {i === 0 && <td rowSpan={tags.length} className={`px-2 text-right align-middle font-mono text-[12px] ${groupBorder}`}>{bytes(image.size)}</td>}
              {i === 0 && <td rowSpan={tags.length} className={`px-2 align-middle font-mono text-[12px] ${groupBorder}`}>{createdAt(image.created)}</td>}
              {i === 0 && <td rowSpan={tags.length} className={`px-2 align-middle ${groupBorder}`}><UsedByCell image={image} /></td>}
              {i === 0 && <td rowSpan={tags.length} className={`px-2 text-right align-middle whitespace-nowrap ${groupBorder}`}>{actions(image)}</td>}
            </tr>;
          });
        })}</tbody>
      </table>}
    </div>
    {pull && <PullDialog close={() => setPull(false)} done={refresh} />}
    {tag && <TagDialog image={tag} close={() => setTag(null)} done={refresh} />}
    {untag && <UntagDialog image={untag.image} tag={untag.tag} close={() => setUntag(null)} done={refresh} />}
    {remove && <RemoveDialog image={remove} close={() => setRemove(null)} done={refresh} />}
    {prune && <PruneDialog close={() => setPrune(false)} done={refresh} />}
    {runImage !== "" && <CreateContainerModal image={runImage} close={() => setRunImage("")} />}
  </section>;
}

function Info({ title, rows }: { title: string; rows: readonly (readonly [string, string])[] }) {
  return <div className="rounded border border-line bg-panel p-4">
    <h2 className="m-0 mb-3 text-[14px]">{title}</h2>
    {rows.length ? <dl className="grid grid-cols-[minmax(100px,auto)_1fr] gap-x-4 gap-y-2 text-[12px]">{rows.map(([key, value]) => <>
      <dt key={`${key}-k`} className="text-muted">{key}</dt><dd key={`${key}-v`} className="min-w-0 break-all font-mono">{value}</dd>
    </>)}</dl> : <p className="text-[12px] text-muted">None</p>}
  </div>;
}

function HistoryTab({ id }: { id: string }) {
  const history = useQuery({ queryKey: ["image-history", id], queryFn: () => api.get<HistoryLayer[]>(`/images/${encodeURIComponent(id)}/history`) });
  const layers = history.data ?? [];
  if (history.isLoading) return <EmptyState title="Loading layer history" action="Contacting Docker…" />;
  if (layers.length === 0) return <EmptyState title="No layer history" action="This image has no recorded build history." />;
  return <div className="overflow-x-auto rounded border border-line bg-panel"><table className="w-full min-w-[700px] border-collapse text-[12px]">
    <thead className="border-b border-line bg-paper text-left text-[11px] uppercase tracking-wide text-muted"><tr><th className="px-2">Layer</th><th className="px-2 text-right">Size</th><th className="px-2">Command</th></tr></thead>
    <tbody>{layers.map((l, i) => <tr key={`${l.id}-${i}`} className="border-b border-linesoft last:border-0 align-top"><td className="px-2 py-1.5 font-mono">{l.id.startsWith("sha256:") ? shortID(l.id) : l.id}</td><td className="whitespace-nowrap px-2 py-1.5 text-right font-mono">{bytes(l.size)}</td><td className="px-2 py-1.5 font-mono break-all">{l.created_by || "—"}</td></tr>)}</tbody>
  </table></div>;
}

function DockerfileTab({ id }: { id: string }) {
  const { push } = useToast();
  const reconstruction = useQuery({ queryKey: ["image-dockerfile", id], queryFn: () => api.get<DockerfileReconstruction>(`/images/${encodeURIComponent(id)}/dockerfile`) });
  const dockerfile = reconstruction.data?.dockerfile ?? "";
  async function copy() {
    try { await navigator.clipboard.writeText(dockerfile); push("Dockerfile copied."); }
    catch { push("Could not copy Dockerfile. Select it and copy manually.", "error"); }
  }
  function download() {
    try {
      const url = URL.createObjectURL(new Blob([dockerfile], { type: "text/plain;charset=utf-8" }));
      const link = document.createElement("a"); link.href = url; link.download = "Dockerfile";
      document.body.appendChild(link); link.click(); link.remove(); window.setTimeout(() => URL.revokeObjectURL(url), 0);
    } catch { push("Could not download Dockerfile.", "error"); }
  }
  if (reconstruction.isLoading) return <EmptyState title="Reconstructing Dockerfile" action="Reading image history from Docker…" />;
  if (reconstruction.isError) return <EmptyState title="Could not reconstruct Dockerfile" action="Refresh the page or check that Docker can inspect this image." />;
  return <div>
    <div className="mb-3 rounded border border-[#E3CB93] border-l-[3px] border-l-pause bg-[#FDF8EC] px-3 py-2 text-[13px]">Best-effort reconstruction from image history — the original Dockerfile is not stored by Docker and this may not rebuild identically.</div>
    <div className="mb-2 flex items-center"><span className="text-[13px] text-muted">Reconstructed Dockerfile</span><div className="ml-auto flex gap-2"><Button onClick={() => void copy()}>Copy</Button><Button onClick={download}>Download as Dockerfile</Button></div></div>
    <pre className="max-h-[65vh] overflow-auto rounded border border-line bg-ink p-4 text-[12px] text-[#D7E7EA]">{dockerfile}</pre>
  </div>;
}

export function ImageDetailPage() {
  const { id = "" } = useParams();
  const { push } = useToast();
  const [tabName, setTabName] = useState("overview");
  const [run, setRun] = useState(false);
  const detail = useQuery({ queryKey: ["image", id], queryFn: () => api.get<ImageDetail>(`/images/${encodeURIComponent(id)}`) });
  const image = detail.data;
  if (detail.isLoading) return <EmptyState title="Loading image" action="Contacting Docker…" />;
  if (!image) return <EmptyState title="Image not found" action="Return to the images list and refresh." />;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(image.raw, null, 2));
      push("Inspect JSON copied.");
    } catch {
      push("Could not copy JSON. Select it and copy manually.", "error");
    }
  };
  return <section>
    <div className="mb-3 flex items-center gap-3">
      <h1 className="m-0 text-lg">{image.repo_tags[0] ? displayTag(image.repo_tags[0]) : "<none>"}</h1>
      <span className="font-mono text-[12px] text-muted">{shortID(image.id)}</span>
      <span className="flex-1" /><Can do="containers.create"><Button variant="primary" disabled={!image.repo_tags[0] || image.repo_tags[0] === "<none>:<none>"} onClick={() => setRun(true)}>Run</Button></Can>
    </div>
    <div role="tablist" className="mb-4 flex gap-1 border-b border-line">{["overview", "history", "dockerfile", "inspect"].map((name) => <button key={name} role="tab" aria-selected={tabName === name} onClick={() => setTabName(name)} className={`px-3 py-2 text-[13px] capitalize ${tabName === name ? "border-b-2 border-hull font-medium" : "text-muted"}`}>{name}</button>)}</div>
    {tabName === "overview" && <div className="grid gap-3 md:grid-cols-2">
      <Info title="Configuration" rows={[["Architecture", image.architecture || "—"], ["OS", image.os || "—"], ["Entrypoint", image.entrypoint.join(" ") || "—"], ["Command", image.cmd.join(" ") || "—"], ["Size", bytes(image.size)]]} />
      <Info title="Labels" rows={Object.entries(image.labels)} />
      <Info title="Environment" rows={image.env.map((entry) => { const at = entry.indexOf("="); return [at < 0 ? entry : entry.slice(0, at), at < 0 ? "" : entry.slice(at + 1)] as const; })} />
      <div className="rounded border border-line bg-panel p-4">
        <h2 className="m-0 mb-3 text-[14px]">Containers using this image</h2>
        {image.used_by.length ? <ul className="m-0 list-none space-y-1 p-0 text-[12px]">{image.used_by.map((u) => <li key={u.container_id}><Link to={`/containers/${encodeURIComponent(u.container_id)}`} className="font-mono">{u.container_name || u.container_id.slice(0, 12)}</Link> <span className="text-muted">{u.state}</span></li>)}</ul> : <p className="text-[12px] text-muted">None</p>}
      </div>
    </div>}
    {tabName === "history" && <HistoryTab id={image.id} />}
    {tabName === "dockerfile" && <DockerfileTab id={image.id} />}
    {tabName === "inspect" && <div><div className="mb-2 flex items-center"><span className="text-[13px] text-muted">Full engine response</span><Button className="ml-auto" onClick={() => void copy()}>Copy JSON</Button></div><pre className="max-h-[65vh] overflow-auto rounded border border-line bg-ink p-4 text-[12px] text-[#D7E7EA]">{JSON.stringify(image.raw, null, 2)}</pre></div>}
    {run && <CreateContainerModal image={image.repo_tags[0]} close={() => setRun(false)} />}
  </section>;
}
