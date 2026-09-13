import { useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Pencil, Trash2 } from "lucide-react";
import { Button } from "../../components/ui/Button";
import { EmptyState } from "../../components/ui/EmptyState";
import { Select, type SelectOption } from "../../components/ui/Select";
import { useToast } from "../../components/ui/Toast";
import { Can } from "../../auth/Can";
import { useAuth } from "../../auth/AuthProvider";
import { api } from "../../lib/api";
import { ApiRequestError } from "../../types/api";
import type { VesselUser } from "../../types/api";

const inputClass = "mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 text-[13px]";
const ROLE_OPTIONS: SelectOption[] = [{ value: "admin", label: "admin" }, { value: "operator", label: "operator" }, { value: "viewer", label: "viewer" }];
const MIN_PASSWORD = 8;
const iconAction = "rounded p-1 text-muted hover:bg-paper hover:text-text disabled:cursor-not-allowed disabled:opacity-45";
const dangerIconAction = `${iconAction} hover:bg-fail/10 hover:text-fail`;

function Modal({ title, children, close, busy = false }: { title: string; children: React.ReactNode; close: () => void; busy?: boolean }) {
  return <div role="dialog" aria-modal="true" aria-label={title} className="fixed inset-0 z-40 grid place-items-center bg-ink/45 p-4" onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) close(); }}>
    <div className="max-h-[90vh] w-full max-w-md overflow-auto rounded border border-line bg-panel p-5 shadow-lg">
      <div className="flex items-center gap-3"><h2 className="m-0 text-lg">{title}</h2><button type="button" aria-label={`Close ${title}`} disabled={busy} onClick={close} className="ml-auto rounded px-2 text-xl text-muted hover:bg-paper hover:text-text">×</button></div>
      {children}
    </div>
  </div>;
}

function errMsg(err: unknown, fallback: string) {
  if (err instanceof ApiRequestError) return err.message;
  return err instanceof Error ? err.message : fallback;
}

function AddUserModal({ close }: { close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast();
  const [username, setUsername] = useState(""); const [password, setPassword] = useState(""); const [confirm, setConfirm] = useState(""); const [role, setRole] = useState("viewer"); const [busy, setBusy] = useState(false); const [error, setError] = useState<string | null>(null);
  const passwordOK = password.length >= MIN_PASSWORD; const matches = password === confirm;
  const valid = username.trim() !== "" && passwordOK && matches;
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true); setError(null);
    try {
      await api.post("/users", { username: username.trim(), password, role });
      await queryClient.invalidateQueries({ queryKey: ["users"] });
      push(`Added user ${username.trim()}.`);
      close();
    } catch (err) { setError(errMsg(err, "Could not add user. Try again.")); setBusy(false); }
  }
  return <Modal title="Add user" close={close} busy={busy}>
    <form onSubmit={(e) => void submit(e)}>
      <label className="mt-4 block text-[13px]">Username<input autoFocus disabled={busy} value={username} onChange={(e) => setUsername(e.target.value)} className={inputClass} /></label>
      <label className="mt-3 block text-[13px]">Role<Select disabled={busy} value={role} onChange={setRole} options={ROLE_OPTIONS} /></label>
      <label className="mt-3 block text-[13px]">Password<input type="password" disabled={busy} value={password} onChange={(e) => setPassword(e.target.value)} className={inputClass} /></label>
      {password && !passwordOK && <p className="mb-0 text-[12px] text-fail">At least {MIN_PASSWORD} characters.</p>}
      <label className="mt-3 block text-[13px]">Confirm password<input type="password" disabled={busy} value={confirm} onChange={(e) => setConfirm(e.target.value)} className={inputClass} /></label>
      {confirm && !matches && <p className="mb-0 text-[12px] text-fail">Passwords do not match.</p>}
      {error && <p className="mb-0 mt-3 text-[12px] text-fail">{error}</p>}
      <div className="mt-5 flex justify-end gap-2"><Button type="button" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" variant="primary" disabled={!valid || busy}>Add user</Button></div>
    </form>
  </Modal>;
}

function ResetPasswordModal({ user, close }: { user: VesselUser; close: () => void }) {
  const { push } = useToast();
  const [password, setPassword] = useState(""); const [confirm, setConfirm] = useState(""); const [busy, setBusy] = useState(false); const [error, setError] = useState<string | null>(null);
  const passwordOK = password.length >= MIN_PASSWORD; const matches = password === confirm; const valid = passwordOK && matches;
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true); setError(null);
    try {
      await api.post(`/users/${user.id}/password`, { password });
      push(`Reset password for ${user.username}.`);
      close();
    } catch (err) { setError(errMsg(err, "Could not reset password. Try again.")); setBusy(false); }
  }
  return <Modal title={`Reset password for ${user.username}`} close={close} busy={busy}>
    <form onSubmit={(e) => void submit(e)}>
      <label className="mt-4 block text-[13px]">New password<input type="password" autoFocus disabled={busy} value={password} onChange={(e) => setPassword(e.target.value)} className={inputClass} /></label>
      {password && !passwordOK && <p className="mb-0 text-[12px] text-fail">At least {MIN_PASSWORD} characters.</p>}
      <label className="mt-3 block text-[13px]">Confirm new password<input type="password" disabled={busy} value={confirm} onChange={(e) => setConfirm(e.target.value)} className={inputClass} /></label>
      {confirm && !matches && <p className="mb-0 text-[12px] text-fail">Passwords do not match.</p>}
      {error && <p className="mb-0 mt-3 text-[12px] text-fail">{error}</p>}
      <div className="mt-5 flex justify-end gap-2"><Button type="button" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" variant="primary" disabled={!valid || busy}>Reset password</Button></div>
    </form>
  </Modal>;
}

function ChangeMyPasswordModal({ user, close }: { user: VesselUser; close: () => void }) {
  const { push } = useToast();
  const [current, setCurrent] = useState(""); const [password, setPassword] = useState(""); const [confirm, setConfirm] = useState(""); const [busy, setBusy] = useState(false); const [error, setError] = useState<string | null>(null);
  const passwordOK = password.length >= MIN_PASSWORD; const matches = password === confirm; const valid = current !== "" && passwordOK && matches;
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true); setError(null);
    try {
      await api.post(`/users/${user.id}/password`, { current_password: current, new_password: password });
      push("Password changed.");
      close();
    } catch (err) { setError(errMsg(err, "Could not change password. Try again.")); setBusy(false); }
  }
  return <Modal title="Change my password" close={close} busy={busy}>
    <form onSubmit={(e) => void submit(e)}>
      <label className="mt-4 block text-[13px]">Current password<input type="password" autoFocus disabled={busy} value={current} onChange={(e) => setCurrent(e.target.value)} className={inputClass} /></label>
      <label className="mt-3 block text-[13px]">New password<input type="password" disabled={busy} value={password} onChange={(e) => setPassword(e.target.value)} className={inputClass} /></label>
      {password && !passwordOK && <p className="mb-0 text-[12px] text-fail">At least {MIN_PASSWORD} characters.</p>}
      <label className="mt-3 block text-[13px]">Confirm new password<input type="password" disabled={busy} value={confirm} onChange={(e) => setConfirm(e.target.value)} className={inputClass} /></label>
      {confirm && !matches && <p className="mb-0 text-[12px] text-fail">Passwords do not match.</p>}
      {error && <p className="mb-0 mt-3 text-[12px] text-fail">{error}</p>}
      <div className="mt-5 flex justify-end gap-2"><Button type="button" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" variant="primary" disabled={!valid || busy}>Change password</Button></div>
    </form>
  </Modal>;
}

function RemoveUserModal({ user, close }: { user: VesselUser; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast(); const [busy, setBusy] = useState(false); const [error, setError] = useState<string | null>(null);
  async function remove() {
    setBusy(true); setError(null);
    try {
      await api.delete(`/users/${user.id}`);
      await queryClient.invalidateQueries({ queryKey: ["users"] });
      push(`Removed ${user.username}.`);
      close();
    } catch (err) { setError(errMsg(err, "Could not remove user. Try again.")); setBusy(false); }
  }
  return <Modal title={`Remove ${user.username}?`} close={close} busy={busy}>
    <p className="mt-3 text-[13px] text-muted">This permanently removes their account and access.</p>
    {error && <p className="mb-0 text-[12px] text-fail">{error}</p>}
    <div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void remove()}>Remove</Button></div>
  </Modal>;
}

function EditUserModal({ user, close }: { user: VesselUser; close: () => void }) {
  const queryClient = useQueryClient(); const { push } = useToast();
  const [role, setRole] = useState(user.role); const [busy, setBusy] = useState(false); const [error, setError] = useState<string | null>(null);
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (role === user.role) { close(); return; }
    setBusy(true); setError(null);
    try {
      await api.patch(`/users/${user.id}`, { role });
      await queryClient.invalidateQueries({ queryKey: ["users"] });
      push(`${user.username} is now ${role}.`);
      close();
    } catch (err) { setError(errMsg(err, "Could not update role.")); setBusy(false); }
  }
  return <Modal title={`Edit ${user.username}`} close={close} busy={busy}>
    <form onSubmit={(e) => void submit(e)}>
      <label className="mt-4 block text-[13px]">Role<Select disabled={busy} value={role} onChange={(v) => setRole(v as VesselUser["role"])} options={ROLE_OPTIONS} /></label>
      {error && <p className="mb-0 mt-3 text-[12px] text-fail">{error}</p>}
      <div className="mt-5 flex justify-end gap-2"><Button type="button" disabled={busy} onClick={close}>Cancel</Button><Button type="submit" variant="primary" disabled={busy}>Save</Button></div>
    </form>
  </Modal>;
}

export function UsersTab() {
  const { user: me } = useAuth();
  const [add, setAdd] = useState(false); const [editTarget, setEditTarget] = useState<VesselUser | null>(null); const [resetTarget, setResetTarget] = useState<VesselUser | null>(null); const [changeMine, setChangeMine] = useState<VesselUser | null>(null); const [removeTarget, setRemoveTarget] = useState<VesselUser | null>(null);
  const users = useQuery({ queryKey: ["users"], queryFn: () => api.get<{ users: VesselUser[] }>("/users") });
  const rows = users.data?.users ?? [];
  return <div>
    <div className="mb-3 flex items-center gap-2">
      <span className="flex-1" />
      <Can do="users.create"><Button variant="primary" onClick={() => setAdd(true)}>Add user</Button></Can>
    </div>
    <div className="overflow-x-auto rounded border border-line bg-panel">
      {users.isLoading ? <EmptyState title="Loading users" action="…" /> : rows.length === 0 ? <EmptyState title="No users found" action="Add a user to get started." /> : <table className="w-full min-w-[560px] border-collapse text-[13px]">
        <thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted"><tr><th className="px-3">Username</th><th className="px-3">Role</th><th className="px-3">Created</th><th className="px-3">Last login</th><th className="w-[110px] px-3" /></tr></thead>
        <tbody>{rows.map((u) => <tr key={u.id} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper">
          <td className="px-3 font-medium">{u.username}{me?.id === String(u.id) && <span className="ml-2 rounded-sm border border-line px-1 font-mono text-[11px] text-muted">you</span>}</td>
          <td className="px-3 font-mono text-[12px] text-muted">{u.role}</td>
          <td className="px-3 font-mono text-[12px] text-muted">{new Date(u.created_at).toLocaleDateString()}</td>
          <td className="px-3 font-mono text-[12px] text-muted">{u.last_login_at ? new Date(u.last_login_at).toLocaleString() : "never"}</td>
          <td className="px-3 text-right">
            <span className="flex items-center justify-end gap-0.5 opacity-0 group-hover:opacity-100 focus-within:opacity-100">
              <Can do="users.update"><button type="button" title="Edit role" aria-label={`Edit ${u.username}`} onClick={() => setEditTarget(u)} className={iconAction}><Pencil size={15} /></button></Can>
              {me?.id === String(u.id) ? <button type="button" title="Change my password" aria-label="Change my password" onClick={() => setChangeMine(u)} className={iconAction}><KeyRound size={15} /></button>
                : me?.role === "admin" && <button type="button" title="Reset password" aria-label={`Reset password for ${u.username}`} onClick={() => setResetTarget(u)} className={iconAction}><KeyRound size={15} /></button>}
              <Can do="users.remove"><button type="button" title="Remove" aria-label={`Remove ${u.username}`} onClick={() => setRemoveTarget(u)} className={dangerIconAction}><Trash2 size={15} /></button></Can>
            </span>
          </td>
        </tr>)}</tbody>
      </table>}
    </div>
    {add && <AddUserModal close={() => setAdd(false)} />}
    {editTarget && <EditUserModal user={editTarget} close={() => setEditTarget(null)} />}
    {resetTarget && <ResetPasswordModal user={resetTarget} close={() => setResetTarget(null)} />}
    {changeMine && <ChangeMyPasswordModal user={changeMine} close={() => setChangeMine(null)} />}
    {removeTarget && <RemoveUserModal user={removeTarget} close={() => setRemoveTarget(null)} />}
  </div>;
}
