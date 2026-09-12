import { useRef, useState, type DragEvent, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Eye, FilePlus, FolderOpen, FolderPlus, Pencil, PencilLine, Trash2, Upload as UploadIcon } from "lucide-react";
import { Can } from "../../auth/Can";
import { api, apiRoot, getToken } from "../../lib/api";
import type { ApiRequestError, ContainerFileEntry, ContainerFileView } from "../../types/api";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { useToast } from "../ui/Toast";

function size(n: number) { if (!n) return "—"; const u = ["B", "KiB", "MiB", "GiB"]; let i = 0; while (n >= 1024 && i < 3) { n /= 1024; i++; } return `${n >= 10 || i === 0 ? Math.round(n) : n.toFixed(1)} ${u[i]}`; }
function modified(v: string) { const d = new Date(v); return Number.isNaN(d.valueOf()) ? v : d.toISOString().replace("T", " ").slice(0, 16); }

// Extension/name heuristics deciding whether the view/edit actions appear for a
// file row, without a network round trip per row. The /files/view endpoint is
// the definitive check (content-sniffed server-side); this only predicts it so
// binary files don't show actions that will fail (SPEC: hidden, not disabled).
const TEXT_EXTENSIONS = new Set(["txt", "md", "markdown", "yml", "yaml", "json", "js", "jsx", "ts", "tsx", "go", "py", "rb", "sh", "bash", "zsh", "conf", "cfg", "ini", "toml", "env", "log", "csv", "tsv", "html", "htm", "css", "scss", "xml", "sql", "rs", "c", "h", "cpp", "hpp", "cc", "java", "kt", "swift", "gradle", "properties", "proto", "graphql", "vue", "svelte", "lock"]);
const TEXT_NAMES = new Set(["dockerfile", "makefile", "license", "readme", "gemfile", "procfile", "rakefile", ".gitignore", ".dockerignore", ".editorconfig", ".env"]);
const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "ico"]);

function guessKind(name: string): "text" | "image" | null {
  const lower = name.toLowerCase();
  if (TEXT_NAMES.has(lower)) return "text";
  const ext = lower.includes(".") ? lower.slice(lower.lastIndexOf(".") + 1) : "";
  if (IMAGE_EXTENSIONS.has(ext)) return "image";
  if (TEXT_EXTENSIONS.has(ext)) return "text";
  return null;
}

function Dialog({ title, close, children }: { title: string; close: () => void; children: ReactNode }) {
  return <div role="dialog" aria-modal="true" aria-label={title} className="fixed inset-0 z-50 grid place-items-center bg-ink/45 p-4" onMouseDown={e => { if (e.target === e.currentTarget) close(); }}><div className="w-full max-w-lg rounded-md bg-panel shadow-[0_20px_60px_rgba(11,31,42,.35)]"><div className="flex items-center border-b border-line px-[18px] py-[14px]"><h2 className="m-0 text-[15px] font-semibold">{title}</h2><button aria-label="Close" onClick={close} className="ml-auto rounded px-1 text-lg leading-none text-muted hover:bg-paper hover:text-text">×</button></div>{children}</div></div>;
}

function RowIconButton({ label, onClick, danger, children }: { label: string; onClick: () => void; danger?: boolean; children: ReactNode }) {
  return <button type="button" title={label} aria-label={label} onClick={onClick} className={`rounded p-1 ${danger ? "text-muted hover:bg-fail/10 hover:text-fail" : "text-muted hover:bg-panel hover:text-text"}`}>{children}</button>;
}

export function FileBrowser({ containerID, basePath, readOnly = false, capabilityPrefix = "containers.files", resourceLabel = "the container" }: { containerID: string; basePath?: string; readOnly?: boolean; capabilityPrefix?: string; resourceLabel?: string }) {
  const base = basePath ?? `/containers/${encodeURIComponent(containerID)}`;
  const cap = (suffix: string) => `${capabilityPrefix}.${suffix}`;
  const { push } = useToast(); const client = useQueryClient(); const picker = useRef<HTMLInputElement>(null);
  const [dir, setDir] = useState("/"); const [busy, setBusy] = useState(false);
  const [folderOpen, setFolderOpen] = useState(false); const [folderName, setFolderName] = useState("");
  const [newFileOpen, setNewFileOpen] = useState(false); const [newFileName, setNewFileName] = useState("");
  const [uploadOpen, setUploadOpen] = useState(false); const [files, setFiles] = useState<File[]>([]); const [dragging, setDragging] = useState(false);
  const [renameTarget, setRenameTarget] = useState<ContainerFileEntry | null>(null); const [renameValue, setRenameValue] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<ContainerFileEntry | null>(null);
  const [viewer, setViewer] = useState<{ entry: ContainerFileEntry; mode: "view" | "edit" } | null>(null);
  const editRef = useRef<HTMLTextAreaElement>(null);

  const listing = useQuery({ queryKey: ["container-files", containerID, dir], queryFn: () => api.get<ContainerFileEntry[]>(`${base}/files?path=${encodeURIComponent(dir)}`) });
  const fileView = useQuery({
    queryKey: ["container-file-view", containerID, viewer?.entry.path],
    queryFn: () => api.get<ContainerFileView>(`${base}/files/view?path=${encodeURIComponent(viewer!.entry.path)}`),
    enabled: viewer != null,
  });
  const refresh = () => client.invalidateQueries({ queryKey: ["container-files", containerID] });

  const addFiles = (next: FileList | File[]) => setFiles(old => { const all = [...old, ...Array.from(next)]; return all.filter((file, i) => all.findIndex(other => other.name === file.name && other.size === file.size && other.lastModified === file.lastModified) === i); });
  const closeUpload = () => { if (!busy) { setUploadOpen(false); setFiles([]); setDragging(false); } };

  async function upload() {
    if (!files.length) return;
    setBusy(true);
    const form = new FormData(); form.set("path", dir); files.forEach(file => form.append("files", file));
    try {
      await api.upload(`${base}/files`, form);
      push(`${files.length} file${files.length === 1 ? "" : "s"} uploaded to ${dir}.`);
      await refresh(); setUploadOpen(false); setFiles([]);
    } catch (e) { push(`Could not upload: ${e instanceof Error ? e.message : "try again"}.`, "error"); } finally { setBusy(false); }
  }

  async function createFolder() {
    const name = folderName.trim(); if (!name) return;
    setBusy(true); const target = `${dir.replace(/\/$/, "")}/${name}`;
    try {
      await api.post(`${base}/folders`, { path: target });
      push(`Created ${target}.`); setFolderOpen(false); setFolderName(""); await refresh();
    } catch (e) { push(`Could not create folder: ${e instanceof Error ? e.message : "try again"}.`, "error"); } finally { setBusy(false); }
  }

  async function createFile() {
    const name = newFileName.trim(); if (!name) return;
    setBusy(true); const target = `${dir.replace(/\/$/, "")}/${name}`;
    try {
      await api.put(`${base}/files/content`, { path: target, content: "" });
      push(`Created ${target}.`); setNewFileOpen(false); setNewFileName(""); await refresh();
      setViewer({ entry: { name, path: target, type: "file", size: 0, mode: "", modified_at: new Date().toISOString() }, mode: "edit" });
    } catch (e) { push(`Could not create ${target}: ${e instanceof Error ? e.message : "try again"}.`, "error"); } finally { setBusy(false); }
  }

  async function rename() {
    if (!renameTarget) return;
    const name = renameValue.trim(); if (!name || name === renameTarget.name) { setRenameTarget(null); return; }
    setBusy(true);
    try {
      await api.post(`${base}/files/rename`, { path: renameTarget.path, name });
      push(`Renamed to ${name}.`); setRenameTarget(null); await refresh();
    } catch (e) { push(`Could not rename ${renameTarget.name}: ${e instanceof Error ? e.message : "try again"}.`, "error"); } finally { setBusy(false); }
  }

  async function remove() {
    if (!deleteTarget) return;
    setBusy(true);
    try {
      await api.delete(`${base}/files?path=${encodeURIComponent(deleteTarget.path)}`);
      push(`Deleted ${deleteTarget.name}.`); setDeleteTarget(null); await refresh();
    } catch (e) { push(`Could not delete ${deleteTarget.name}: ${e instanceof Error ? e.message : "try again"}.`, "error"); } finally { setBusy(false); }
  }

  async function download(entry: ContainerFileEntry) {
    try {
      const headers = new Headers(); const token = getToken(); if (token) headers.set("Authorization", `Bearer ${token}`);
      const res = await fetch(`${apiRoot()}${base}/files/download?path=${encodeURIComponent(entry.path)}`, { headers });
      if (!res.ok) throw new Error(res.statusText);
      const href = URL.createObjectURL(await res.blob()); const a = document.createElement("a"); a.href = href; a.download = `${entry.name}.tar`; a.click(); URL.revokeObjectURL(href);
    } catch (e) { push(`Could not download ${entry.name}: ${e instanceof Error ? e.message : "try again"}.`, "error"); }
  }

  async function saveEdit() {
    if (!viewer) return;
    const content = editRef.current?.value ?? "";
    setBusy(true);
    try {
      await api.put(`${base}/files/content`, { path: viewer.entry.path, content });
      push(`Saved ${viewer.entry.name}.`); setViewer(null); await refresh();
    } catch (e) { push(`Could not save ${viewer.entry.name}: ${e instanceof Error ? e.message : "try again"}.`, "error"); } finally { setBusy(false); }
  }

  function openViewer(entry: ContainerFileEntry, mode: "view" | "edit") {
    setViewer({ entry, mode });
  }

  const error = listing.error as ApiRequestError | null;
  const unavailable = error?.status === 404;
  const notFound = !!error && !unavailable && error.code !== "container_not_running" && /no such file|not found|cannot access/i.test(error.message);
  const drop = (e: DragEvent<HTMLDivElement>) => { e.preventDefault(); setDragging(false); addFiles(e.dataTransfer.files); };

  const segments = dir.split("/").filter(Boolean);
  const crumbs: { label: string; path: string }[] = [{ label: "/", path: "/" }, ...segments.map((label, i) => ({ label, path: "/" + segments.slice(0, i + 1).join("/") }))];

  return <div>
    <div className="mb-3 flex flex-wrap items-center gap-1">
      {crumbs.map((crumb, i) => (
        <span key={crumb.path} className="flex items-center gap-1">
          {i > 1 && <span className="text-muted">/</span>}
          {i === crumbs.length - 1
            ? <span className="font-mono text-[12.5px] text-muted">{crumb.label}</span>
            : <button className="font-mono text-[12.5px] text-link hover:underline" onClick={() => setDir(crumb.path)}>{crumb.label}</button>}
        </span>
      ))}
      <span className="flex-1" />
      {!readOnly && <Can do={cap("mkdir")}><Button title="New folder" aria-label="New folder" disabled={busy} onClick={() => setFolderOpen(true)} className="px-2"><FolderPlus size={15} /></Button></Can>}
      {!readOnly && <Can do={cap("edit")}><Button title="New file" aria-label="New file" disabled={busy} onClick={() => setNewFileOpen(true)} className="px-2"><FilePlus size={15} /></Button></Can>}
      {!readOnly && <Can do={cap("upload")}><Button title="Upload" aria-label="Upload" variant="primary" disabled={busy} onClick={() => setUploadOpen(true)} className="px-2"><UploadIcon size={15} /></Button></Can>}
    </div>

    <div className="overflow-x-auto rounded border border-line bg-panel">
      {listing.isLoading ? (
        <EmptyState title="Loading directory" action="Reading the container filesystem…" />
      ) : error ? (
        <EmptyState
          title={unavailable ? "Directory browsing is unavailable" : error.code === "container_not_running" ? "Container is not running" : notFound ? "Directory not found" : "Could not list directory"}
          action={unavailable ? "Start Vessel without --allow-exec=false to browse directories or create folders." : error.code === "container_not_running" ? "Start the container before browsing its filesystem." : notFound ? "Check the path, or the container may not have this filesystem layout." : error.message}
        />
      ) : (listing.data?.length ?? 0) === 0 ? (
        <EmptyState title="This directory is empty" action="Upload files or create a folder here." />
      ) : (
        <table className="w-full min-w-[680px] border-collapse text-[13px]">
          <thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted">
            <tr>
              <th className="px-3 py-[7px]">Name</th>
              <th className="w-20 px-3 py-[7px]">Type</th>
              <th className="w-[90px] px-3 py-[7px] text-right">Size</th>
              <th className="w-[170px] px-3 py-[7px]">Modified</th>
              <th className="w-[140px] px-3 py-[7px]" />
            </tr>
          </thead>
          <tbody>
            {listing.data?.map(entry => {
              const kind = entry.type === "file" ? guessKind(entry.name) : null;
              return (
                <tr key={entry.path} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper">
                  <td className="px-3 font-medium">
                    {entry.type === "dir" ? "📁 " : ""}
                    {entry.type === "dir" ? <button className="text-left hover:underline" onClick={() => setDir(entry.path)}>{entry.name}</button> : entry.name}
                  </td>
                  <td className="px-3 text-muted">{entry.type === "dir" ? "directory" : entry.type}</td>
                  <td className="px-3 text-right font-mono text-[12.5px] text-muted">{entry.type === "dir" ? "—" : size(entry.size)}</td>
                  <td className="px-3 font-mono text-[12.5px] text-muted">{modified(entry.modified_at)}</td>
                  <td className="px-3 text-right">
                    <span className="invisible flex items-center justify-end gap-0.5 group-hover:visible group-focus-within:visible">
                      {entry.type === "dir" && <RowIconButton label="Open" onClick={() => setDir(entry.path)}><FolderOpen size={15} /></RowIconButton>}
                      {!readOnly && kind && <RowIconButton label="View" onClick={() => openViewer(entry, "view")}><Eye size={15} /></RowIconButton>}
                      {!readOnly && kind === "text" && <Can do={cap("edit")}><RowIconButton label="Edit" onClick={() => openViewer(entry, "edit")}><PencilLine size={15} /></RowIconButton></Can>}
                      {entry.type !== "dir" && <Can do={cap("download")}><RowIconButton label="Download" onClick={() => void download(entry)}><Download size={15} /></RowIconButton></Can>}
                      {!readOnly && entry.type === "dir" && <Can do={cap("rename")}><RowIconButton label="Rename" onClick={() => { setRenameValue(entry.name); setRenameTarget(entry); }}><Pencil size={15} /></RowIconButton></Can>}
                      {!readOnly && <Can do={cap("delete")}><RowIconButton label="Delete" danger onClick={() => setDeleteTarget(entry)}><Trash2 size={15} /></RowIconButton></Can>}
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
    <p className="mt-2 text-[12px] text-muted">Directory listing, “New folder” and rename/delete use {resourceLabel}’s shell (<span className="font-mono">exec</span>) — these are unavailable when <span className="font-mono">--allow-exec=false</span>. Upload, download, view and edit do not depend on exec and stay available.</p>

    {folderOpen && (
      <Dialog title="New folder" close={() => { if (!busy) { setFolderOpen(false); setFolderName(""); } }}>
        <div className="px-[18px] py-4">
          <label className="block text-[13px]">Folder name
            <input autoFocus value={folderName} onChange={e => setFolderName(e.target.value)} onKeyDown={e => { if (e.key === "Enter") void createFolder(); }} placeholder="reports" className="mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 font-mono text-[13px]" />
          </label>
          <p className="mb-0 mt-2 text-[12px] text-muted">Created in <span className="font-mono">{dir}</span></p>
        </div>
        <div className="flex gap-2 border-t border-line px-[18px] py-3">
          <span className="flex-1" />
          <Button disabled={busy} onClick={() => { setFolderOpen(false); setFolderName(""); }}>Cancel</Button>
          <Button variant="primary" disabled={busy || !folderName.trim()} onClick={() => void createFolder()}>Create folder</Button>
        </div>
      </Dialog>
    )}

    {newFileOpen && (
      <Dialog title="New file" close={() => { if (!busy) { setNewFileOpen(false); setNewFileName(""); } }}>
        <div className="px-[18px] py-4">
          <label className="block text-[13px]">File name
            <input autoFocus value={newFileName} onChange={e => setNewFileName(e.target.value)} onKeyDown={e => { if (e.key === "Enter") void createFile(); }} placeholder="notes.txt" className="mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 font-mono text-[13px]" />
          </label>
          <p className="mb-0 mt-2 text-[12px] text-muted">Created empty in <span className="font-mono">{dir}</span>, then opened for editing.</p>
        </div>
        <div className="flex gap-2 border-t border-line px-[18px] py-3">
          <span className="flex-1" />
          <Button disabled={busy} onClick={() => { setNewFileOpen(false); setNewFileName(""); }}>Cancel</Button>
          <Button variant="primary" disabled={busy || !newFileName.trim()} onClick={() => void createFile()}><FilePlus size={14} className="mr-1 inline" />Create file</Button>
        </div>
      </Dialog>
    )}

    {renameTarget && (
      <Dialog title={`Rename ${renameTarget.name}`} close={() => { if (!busy) setRenameTarget(null); }}>
        <div className="px-[18px] py-4">
          <label className="block text-[13px]">New name
            <input autoFocus value={renameValue} onChange={e => setRenameValue(e.target.value)} onKeyDown={e => { if (e.key === "Enter") void rename(); }} className="mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 font-mono text-[13px]" />
          </label>
        </div>
        <div className="flex gap-2 border-t border-line px-[18px] py-3">
          <span className="flex-1" />
          <Button disabled={busy} onClick={() => setRenameTarget(null)}>Cancel</Button>
          <Button variant="primary" disabled={busy || !renameValue.trim()} onClick={() => void rename()}>Rename</Button>
        </div>
      </Dialog>
    )}

    {deleteTarget && (
      <Dialog title={`Delete ${deleteTarget.name}?`} close={() => { if (!busy) setDeleteTarget(null); }}>
        <div className="px-[18px] py-4 text-[13px]">
          {deleteTarget.type === "dir" ? <p className="m-0">This permanently deletes the folder and everything inside it.</p> : <p className="m-0">This permanently deletes the file.</p>}
        </div>
        <div className="flex gap-2 border-t border-line px-[18px] py-3">
          <span className="flex-1" />
          <Button disabled={busy} onClick={() => setDeleteTarget(null)}>Cancel</Button>
          <Button variant="danger" disabled={busy} onClick={() => void remove()}>Delete</Button>
        </div>
      </Dialog>
    )}

    {uploadOpen && (
      <Dialog title="Upload files" close={closeUpload}>
        <div className="px-[18px] py-4">
          <p className="m-0 text-[13px] text-muted">Upload into <span className="font-mono">{dir}</span></p>
          <div onDragEnter={e => { e.preventDefault(); setDragging(true); }} onDragOver={e => e.preventDefault()} onDragLeave={e => { if (e.currentTarget === e.target) setDragging(false); }} onDrop={drop} className={`mt-4 grid min-h-36 place-items-center rounded border border-dashed p-5 text-center ${dragging ? "border-link bg-paper" : "border-line bg-paper/40"}`}>
            <div>
              <p className="m-0 text-[13px] font-medium">Drag files here</p>
              <p className="mb-3 mt-1 text-[12px] text-muted">or choose files from your computer</p>
              <Button onClick={() => picker.current?.click()}>Choose files</Button>
              <input ref={picker} className="hidden" type="file" multiple onChange={e => { if (e.target.files) addFiles(e.target.files); e.currentTarget.value = ""; }} />
            </div>
          </div>
          {files.length > 0 && (
            <ul className="mt-3 max-h-32 divide-y divide-linesoft overflow-auto rounded border border-line text-[13px]">
              {files.map(file => (
                <li key={`${file.name}-${file.lastModified}`} className="flex items-center gap-2 px-3 py-2">
                  <span className="min-w-0 flex-1 truncate font-mono text-[12px]">{file.name}</span>
                  <span className="font-mono text-[12px] text-muted">{size(file.size)}</span>
                  <button aria-label={`Remove ${file.name}`} onClick={() => setFiles(old => old.filter(item => item !== file))} className="text-muted hover:text-fail">×</button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="flex gap-2 border-t border-line px-[18px] py-3">
          <span className="flex-1" />
          <Button disabled={busy} onClick={closeUpload}>Cancel</Button>
          <Button variant="primary" disabled={busy || files.length === 0} onClick={() => void upload()}>Upload {files.length || ""} file{files.length === 1 ? "" : "s"}</Button>
        </div>
      </Dialog>
    )}

    {viewer && (
      <Dialog title={`${viewer.mode === "edit" ? "Edit" : "View"} ${viewer.entry.name}`} close={() => { if (!busy) setViewer(null); }}>
        <div className="px-[18px] py-4">
          {fileView.isLoading ? (
            <p className="m-0 text-[13px] text-muted">Loading file…</p>
          ) : fileView.error ? (
            <p className="m-0 text-[13px] text-fail">Could not load {viewer.entry.name}: {(fileView.error as ApiRequestError).message}</p>
          ) : fileView.data?.kind === "image" ? (
            <img className="mx-auto max-h-[60vh] max-w-full rounded border border-line" src={`data:${fileView.data.mime};base64,${fileView.data.content ?? ""}`} alt={viewer.entry.name} />
          ) : viewer.mode === "edit" ? (
            <textarea
              ref={editRef}
              autoFocus
              className="h-[50vh] w-full resize-y rounded border border-line bg-paper p-3 font-mono text-[12.5px]"
              defaultValue={fileView.data?.content ?? ""}
            />
          ) : (
            <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap rounded border border-line bg-paper p-3 font-mono text-[12.5px]">{fileView.data?.content ?? ""}</pre>
          )}
        </div>
        <div className="flex gap-2 border-t border-line px-[18px] py-3">
          <span className="flex-1" />
          <Button disabled={busy} onClick={() => setViewer(null)}>{viewer.mode === "edit" ? "Cancel" : "Close"}</Button>
          {viewer.mode === "edit" && fileView.data?.kind === "text" && <Button variant="primary" disabled={busy || fileView.isLoading} onClick={() => void saveEdit()}>Save</Button>}
        </div>
      </Dialog>
    )}
  </div>;
}
