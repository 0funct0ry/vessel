import { useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCw, Trash2 } from "lucide-react";
import { Button } from "../../components/ui/Button";
import { EmptyState } from "../../components/ui/EmptyState";
import { useToast } from "../../components/ui/Toast";
import { api } from "../../lib/api";
import { ApiRequestError } from "../../types/api";
import type { Token } from "../../types/api";

const inputClass = "mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 text-[13px]";
const iconAction = "rounded p-1 text-muted hover:bg-paper hover:text-text disabled:cursor-not-allowed disabled:opacity-45";
const dangerIconAction = `${iconAction} hover:bg-fail/10 hover:text-fail`;

/** Days remaining until a token's expiry, rounded up; undefined for tokens that never expire. */
function daysRemaining(expiresAt?: string): number | undefined {
  if (!expiresAt) return undefined;
  const ms = new Date(expiresAt).getTime() - Date.now();
  return Math.max(1, Math.ceil(ms / (24 * 60 * 60 * 1000)));
}

function errMsg(err: unknown, fallback: string) {
  if (err instanceof ApiRequestError) return err.message;
  return err instanceof Error ? err.message : fallback;
}

function Modal({ title, children, close, busy = false }: { title: string; children: React.ReactNode; close: () => void; busy?: boolean }) {
  return <div role="dialog" aria-modal="true" aria-label={title} className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4" onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) close(); }}>
    <div className="max-h-[90vh] w-full max-w-md overflow-auto rounded border border-line bg-panel p-5 shadow-lg">
      <div className="flex items-center gap-3"><h2 className="m-0 text-lg">{title}</h2><button type="button" aria-label={`Close ${title}`} disabled={busy} onClick={close} className="ml-auto rounded px-2 text-xl text-muted hover:bg-paper hover:text-text">×</button></div>
      {children}
    </div>
  </div>;
}

function CreateTokenModal({ close }: { close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast();
  const [name, setName] = useState(""); const [expiresInDays, setExpiresInDays] = useState(""); const [busy, setBusy] = useState(false); const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<string | null>(null);
  const valid = name.trim() !== "";
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true); setError(null);
    try {
      const days = expiresInDays.trim() === "" ? 0 : Number(expiresInDays);
      const res = await api.post<{ token: string }>("/tokens", { name: name.trim(), expires_in_days: days });
      await queryClient.invalidateQueries({ queryKey: ["tokens"] });
      setCreated(res.token);
    } catch (err) { setError(errMsg(err, "Could not create token. Try again.")); setBusy(false); }
  }
  async function copy() {
    if (!created) return;
    try { await navigator.clipboard.writeText(created); push("Token copied to clipboard."); }
    catch { push("Could not copy. Select the token and copy manually.", "error"); }
  }
  if (created) {
    return <Modal title="Token created" close={close}>
      <p className="mt-3 text-[13px] text-muted">Copy this token now — you won't be able to see it again.</p>
      <div className="mt-2 break-all rounded border border-line bg-paper p-2 font-mono text-[12px]">{created}</div>
      <div className="mt-5 flex justify-end gap-2"><Button onClick={() => void copy()}>Copy</Button><Button variant="primary" onClick={close}>Done</Button></div>
    </Modal>;
  }
  return <Modal title="Create token" close={close} busy={busy}>
    <form onSubmit={(e) => void submit(e)}>
      <label className="mt-4 block text-[13px]">Name<input autoFocus disabled={busy} value={name} onChange={(e) => setName(e.target.value)} placeholder="ci-deploy" className={inputClass} /></label>
      <label className="mt-3 block text-[13px]">Expires in days <span className="text-muted">optional, blank = never</span><input disabled={busy} value={expiresInDays} onChange={(e) => setExpiresInDays(e.target.value.replace(/[^0-9]/g, ""))} placeholder="90" className={inputClass} /></label>
      {error && <p className="mb-0 mt-3 text-[12px] text-fail">{error}</p>}
      <div className="mt-5 flex justify-end gap-2"><Button type="button" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" variant="primary" disabled={!valid || busy}>Create token</Button></div>
    </form>
  </Modal>;
}

function RotateTokenModal({ token, close }: { token: Token; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const [busy, setBusy] = useState(false); const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<string | null>(null);
  async function rotate() {
    setBusy(true); setError(null);
    try {
      const days = daysRemaining(token.expires_at) ?? 0;
      const res = await api.post<{ token: string }>("/tokens", { name: token.name, expires_in_days: days });
      await api.delete(`/tokens/${encodeURIComponent(token.id)}`);
      await queryClient.invalidateQueries({ queryKey: ["tokens"] });
      setCreated(res.token);
    } catch (err) { setError(errMsg(err, "Could not rotate token. Try again.")); setBusy(false); }
  }
  async function copy() {
    if (!created) return;
    try { await navigator.clipboard.writeText(created); push("Token copied to clipboard."); }
    catch { push("Could not copy. Select the token and copy manually.", "error"); }
  }
  if (created) {
    return <Modal title="Token rotated" close={close}>
      <p className="mt-3 text-[13px] text-muted">The old token stopped working immediately. Copy the new one now — you won't be able to see it again.</p>
      <div className="mt-2 break-all rounded border border-line bg-paper p-2 font-mono text-[12px]">{created}</div>
      <div className="mt-5 flex justify-end gap-2"><Button onClick={() => void copy()}>Copy</Button><Button variant="primary" onClick={close}>Done</Button></div>
    </Modal>;
  }
  return <Modal title={`Rotate ${token.name}?`} close={close} busy={busy}>
    <p className="mt-3 text-[13px] text-muted">Issues a new token with the same name and expiry, and revokes this one immediately. Anything using the old value will need updating.</p>
    {error && <p className="mb-0 text-[12px] text-fail">{error}</p>}
    <div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="primary" disabled={busy} onClick={() => void rotate()}>Rotate</Button></div>
  </Modal>;
}

function RevokeTokenModal({ token, close }: { token: Token; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const [busy, setBusy] = useState(false); const [error, setError] = useState<string | null>(null);
  async function revoke() {
    setBusy(true); setError(null);
    try {
      await api.delete(`/tokens/${encodeURIComponent(token.id)}`);
      await queryClient.invalidateQueries({ queryKey: ["tokens"] });
      push(`Revoked ${token.name}.`);
      close();
    } catch (err) { setError(errMsg(err, "Could not revoke token. Try again.")); setBusy(false); }
  }
  return <Modal title={`Revoke ${token.name}?`} close={close} busy={busy}>
    <p className="mt-3 text-[13px] text-muted">Anything using this token will stop authenticating immediately.</p>
    {error && <p className="mb-0 text-[12px] text-fail">{error}</p>}
    <div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void revoke()}>Revoke</Button></div>
  </Modal>;
}

export function TokensTab() {
  const [create, setCreate] = useState(false); const [rotate, setRotate] = useState<Token | null>(null); const [revoke, setRevoke] = useState<Token | null>(null);
  const tokens = useQuery({ queryKey: ["tokens"], queryFn: () => api.get<{ tokens: Token[] }>("/tokens"), retry: false });
  const rows = tokens.data?.tokens ?? [];
  return <div>
    <p className="mb-3 text-[13px] text-muted">Personal API tokens authenticate as you — anywhere a session login would work, e.g. scripts or webhook test calls.</p>
    <div className="mb-3 flex items-center gap-2"><span className="flex-1" /><Button variant="primary" disabled={tokens.isError} onClick={() => setCreate(true)}>Create token</Button></div>
    <div className="overflow-x-auto rounded border border-line bg-panel">
      {tokens.isLoading ? <EmptyState title="Loading tokens" action="…" /> : tokens.isError ? <EmptyState title="Tokens are unavailable" action="Personal API tokens require the server to be running with --auth." /> : rows.length === 0 ? <EmptyState title="No tokens yet" action="Create one to authenticate a script or CLI call." /> : <table className="w-full min-w-[560px] border-collapse text-[13px]">
        <thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted"><tr><th className="px-3">Name</th><th className="px-3">Created</th><th className="px-3">Last used</th><th className="px-3">Expires</th><th className="w-[90px] px-3" /></tr></thead>
        <tbody>{rows.map((t) => <tr key={t.id} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper">
          <td className="px-3 font-medium">{t.name}</td>
          <td className="px-3 font-mono text-[12px] text-muted">{new Date(t.created_at).toLocaleDateString()}</td>
          <td className="px-3 font-mono text-[12px] text-muted">{t.last_used_at ? new Date(t.last_used_at).toLocaleString() : "never"}</td>
          <td className="px-3 font-mono text-[12px] text-muted">{t.expires_at ? new Date(t.expires_at).toLocaleDateString() : "never"}</td>
          <td className="px-3 text-right">
            <span className="flex items-center justify-end gap-0.5 opacity-0 group-hover:opacity-100 focus-within:opacity-100">
              <button type="button" title="Rotate" aria-label={`Rotate ${t.name}`} onClick={() => setRotate(t)} className={iconAction}><RotateCw size={15} /></button>
              <button type="button" title="Revoke" aria-label={`Revoke ${t.name}`} onClick={() => setRevoke(t)} className={dangerIconAction}><Trash2 size={15} /></button>
            </span>
          </td>
        </tr>)}</tbody>
      </table>}
    </div>
    {create && <CreateTokenModal close={() => setCreate(false)} />}
    {rotate && <RotateTokenModal token={rotate} close={() => setRotate(null)} />}
    {revoke && <RevokeTokenModal token={revoke} close={() => setRevoke(null)} />}
  </div>;
}
