import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "../ui/Button";
import { useToast } from "../ui/Toast";
import { api } from "../../lib/api";
import type { CommitContainerResponse } from "../../types/api";

export function CommitContainerModal({ containerID, close }: { containerID: string; close: () => void }) {
  const navigate = useNavigate();
  const { push } = useToast();
  const [repo, setRepo] = useState(""); const [tag, setTag] = useState(""); const [comment, setComment] = useState(""); const [pause, setPause] = useState(true); const [busy, setBusy] = useState(false);
  async function commit() {
    setBusy(true);
    try {
      const result = await api.post<CommitContainerResponse>(`/containers/${encodeURIComponent(containerID)}/commit`, { repo: repo.trim(), tag: tag.trim() || undefined, comment: comment.trim() || undefined, pause });
      push("Container committed to image."); close(); navigate(`/images/${encodeURIComponent(result.image_id)}`);
    } catch (error) {
      push(`Could not commit container: ${error instanceof Error ? error.message : "try again"}.`, "error"); setBusy(false);
    }
  }
  return <div role="dialog" aria-modal="true" aria-labelledby="commit-title" className="fixed inset-0 z-50 flex items-start justify-center overflow-auto bg-ink/45 px-5 py-[5vh]">
    <div className="flex max-h-[90vh] w-full max-w-[520px] flex-col rounded-md bg-panel shadow-[0_20px_60px_rgba(11,31,42,.35)]">
      <div className="flex items-center gap-2 border-b border-line px-[18px] py-[14px]"><h2 id="commit-title" className="m-0 text-[15px] font-semibold">Commit to image</h2><button aria-label="Close" onClick={close} className="ml-auto rounded px-1 text-lg leading-none text-muted hover:bg-paper hover:text-text">×</button></div>
      <div className="overflow-auto px-[18px] py-4"><dl className="grid grid-cols-[auto_1fr] items-center gap-x-4 gap-y-3 text-[13px]"><dt>Repository</dt><dd><input autoFocus value={repo} onChange={e => setRepo(e.target.value)} placeholder="ghcr.io/acme/api" aria-label="Repository" className="w-full rounded border border-line bg-panel px-2 py-1.5 font-mono text-[13px]" /></dd><dt>Tag</dt><dd><input value={tag} onChange={e => setTag(e.target.value)} placeholder="snapshot-2026-09-10" aria-label="Tag" className="w-full rounded border border-line bg-panel px-2 py-1.5 font-mono text-[13px]" /></dd><dt>Comment</dt><dd><input value={comment} onChange={e => setComment(e.target.value)} placeholder="optional" aria-label="Comment" className="w-full rounded border border-line bg-panel px-2 py-1.5 text-[13px]" /></dd><dt>Pause during commit</dt><dd><input type="checkbox" checked={pause} onChange={e => setPause(e.target.checked)} aria-label="Pause during commit" /></dd></dl><div className="mt-3 rounded-sm border border-[#E3CB93] border-l-[3px] border-l-pause bg-[#FDF8EC] px-3 py-[9px] text-[13px]">Committing without pausing can snapshot the filesystem mid-write. Leave “Pause during commit” on unless the container can’t tolerate a brief pause.</div></div>
      <div className="flex items-center gap-2 border-t border-line px-[18px] py-3"><span className="flex-1" /><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="primary" disabled={busy || !repo.trim()} onClick={() => void commit()}>Commit</Button></div>
    </div>
  </div>;
}
