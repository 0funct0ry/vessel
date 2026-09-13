import { useMemo, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Pencil, Plus, RotateCw, Send, Trash2 } from "lucide-react";
import { Can } from "../auth/Can";
import { Button } from "../components/ui/Button";
import { EmptyState } from "../components/ui/EmptyState";
import { Modal } from "../components/ui/Modal";
import { Switch } from "../components/ui/Switch";
import { Tag } from "../components/ui/Tag";
import { useToast } from "../components/ui/Toast";
import { api } from "../lib/api";
import { ApiRequestError } from "../types/api";
import type { Delivery, DeliveryStatus, Webhook } from "../types/api";

const inputClass = "mt-1 block w-full rounded border border-line bg-panel px-2 py-1.5 text-[13px] font-mono";
const iconAction = "rounded p-1 text-muted hover:bg-paper hover:text-text disabled:cursor-not-allowed disabled:opacity-45";
const dangerIconAction = `${iconAction} hover:bg-fail/10 hover:text-fail`;
const DOCS_LINK = "/docs/webhooks#signature-verification";

// Docker's documented event vocabulary, grouped by the resources Vessel's
// webhook engine can match on (SPEC §8's four resource kinds). Docker itself
// doesn't enumerate this in the codebase (internal/webhook forwards whatever
// action string the Engine API sends), so this list is Docker's well-known
// event actions per resource rather than something read out of a Go file.
const EVENT_TAXONOMY: Record<string, string[]> = {
  container: ["create", "start", "stop", "restart", "die", "kill", "pause", "unpause", "rename", "destroy", "health_status", "oom", "update"],
  image: ["pull", "tag", "untag", "delete", "import", "push", "save", "load"],
  volume: ["create", "destroy", "mount", "unmount"],
  network: ["create", "destroy", "connect", "disconnect", "remove"],
};
const RESOURCES = Object.keys(EVENT_TAXONOMY);

function errMsg(err: unknown, fallback: string) {
  if (err instanceof ApiRequestError) return err.message;
  return err instanceof Error ? err.message : fallback;
}

function relativeTime(value: string): string {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

function hostOf(url: string): string {
  try {
    const u = new URL(url);
    return `${u.protocol}//${u.host}`;
  } catch {
    return url;
  }
}

const PRIVATE_HOST_RE = /^(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[0-1])\.|\[::1\]|0\.0\.0\.0)/i;
function isPrivateHost(url: string): boolean {
  try {
    return PRIVATE_HOST_RE.test(new URL(url).hostname);
  } catch {
    return false;
  }
}

function randomSecretHex(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

const STATUS_TONE: Record<DeliveryStatus, "default" | "use" | "warn"> = {
  pending: "default",
  success: "use",
  failed: "warn",
  dead: "warn",
};

function StatusTag({ status }: { status: DeliveryStatus }) {
  return <Tag tone={STATUS_TONE[status]}>{status}</Tag>;
}

/** Synthetic container.start payload matching internal/webhook/payload.go's
 * Payload/Container shape exactly, so the create/edit form can preview field
 * names without a real event. Rendered entirely client-side. */
function fixturePayload(webhookId: string, eventTypes: string[]) {
  const type = eventTypes[0] && eventTypes[0] !== "*" ? eventTypes[0].replace(/\*$/, "start") : "container.start";
  const isContainer = type.startsWith("container");
  return {
    id: "evt_fixture0001",
    delivery_id: "dl_fixture0001",
    webhook_id: webhookId || "wh_…",
    type,
    created_at: new Date().toISOString(),
    host: { name: "vessel-host", engine: "24.0.7" },
    ...(isContainer
      ? { container: { id: "3f2c1a9b8d7e", name: "web-1", image: "nginx:1.27-alpine", exit_code: null, labels: { "com.docker.compose.project": "acme" } } }
      : {}),
    raw: { synthetic: true },
  };
}

function CodeBlock({ value }: { value: unknown }) {
  return <pre className="max-h-64 overflow-auto rounded border border-line bg-paper p-2.5 text-[12px] leading-snug">{typeof value === "string" ? value : JSON.stringify(value, null, 2)}</pre>;
}

/** Shared request/response panel: used for both a real delivery row's
 * expansion and the "Test webhook" round trip, so operators learn one UI. */
function DeliveryPanel({ delivery }: { delivery: Delivery }) {
  return <div className="grid grid-cols-1 gap-3 border-t border-linesoft bg-paper p-3 md:grid-cols-2">
    <div>
      <div className="mb-1 text-[11px] uppercase tracking-wide text-muted">Request body</div>
      <CodeBlock value={delivery.payload} />
    </div>
    <div>
      <div className="mb-1 text-[11px] uppercase tracking-wide text-muted">Response</div>
      {delivery.error
        ? <p className="m-0 rounded border border-fail/40 bg-fail/5 p-2 text-[12px] text-fail">{delivery.error}</p>
        : <CodeBlock value={delivery.response_body ?? "(empty body)"} />}
      <div className="mt-1.5 flex gap-3 text-[11.5px] text-muted">
        {delivery.status_code != null && <span>HTTP {delivery.status_code}</span>}
        {delivery.response_ms != null && <span>{delivery.response_ms} ms</span>}
      </div>
    </div>
  </div>;
}

// ---------------------------------------------------------------------------
// Create / edit form
// ---------------------------------------------------------------------------

interface FormState {
  name: string;
  url: string;
  secret: string;
  eventTypes: string[];
  nameFilter: string;
  imageFilter: string;
  labelRows: { key: string; value: string }[];
  headerRows: { key: string; value: string }[];
  maxAttempts: string;
}

function emptyForm(): FormState {
  return { name: "", url: "", secret: "", eventTypes: [], nameFilter: "", imageFilter: "", labelRows: [], headerRows: [], maxAttempts: "" };
}
function formFromWebhook(w: Webhook): FormState {
  return {
    name: w.name,
    url: w.url,
    secret: "",
    eventTypes: w.event_types,
    nameFilter: w.filters?.name ?? "",
    imageFilter: w.filters?.image ?? "",
    labelRows: Object.entries(w.filters?.label ?? {}).map(([key, value]) => ({ key, value })),
    headerRows: Object.entries(w.headers ?? {}).map(([key, value]) => ({ key, value })),
    maxAttempts: w.max_attempts ? String(w.max_attempts) : "",
  };
}

const RESERVED_HEADER_RE = /^(user-agent|x-vessel-)/i;

function EventTypeSelect({ selected, onChange }: { selected: string[]; onChange: (next: string[]) => void }) {
  const has = (v: string) => selected.includes(v);
  function toggle(v: string) {
    onChange(has(v) ? selected.filter((s) => s !== v) : [...selected, v]);
  }
  return <div className="mt-1 rounded border border-line bg-panel p-2.5">
    <label className="flex items-center gap-2 border-b border-linesoft pb-2 text-[13px] font-medium">
      <input type="checkbox" checked={has("*")} onChange={() => toggle("*")} /> All events (*)
    </label>
    {!has("*") && <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
      {RESOURCES.map((resource) => <div key={resource}>
        <label className="flex items-center gap-2 text-[13px] font-medium capitalize">
          <input type="checkbox" checked={has(`${resource}.*`)} onChange={() => toggle(`${resource}.*`)} /> {resource}.* (any)
        </label>
        <div className="mt-1 ml-5 flex flex-col gap-1">
          {EVENT_TAXONOMY[resource].map((action) => <label key={action} className="flex items-center gap-2 text-[12.5px] text-muted">
            <input type="checkbox" disabled={has(`${resource}.*`)} checked={has(`${resource}.*`) || has(`${resource}.${action}`)} onChange={() => toggle(`${resource}.${action}`)} />
            <span className="font-mono">{resource}.{action}</span>
          </label>)}
        </div>
      </div>)}
    </div>}
  </div>;
}

function WebhookFormModal({ existing, close }: { existing?: Webhook; close: () => void }) {
  const queryClient = useQueryClient();
  const { push } = useToast();
  const [form, setForm] = useState<FormState>(() => (existing ? formFromWebhook(existing) : emptyForm()));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const urlValid = /^https?:\/\/.+/i.test(form.url.trim());
  const privateWarning = urlValid && isPrivateHost(form.url);
  const headerErrors = form.headerRows.filter((r) => r.key.trim() && RESERVED_HEADER_RE.test(r.key.trim()));
  const maxAttemptsNum = form.maxAttempts.trim() === "" ? undefined : Number(form.maxAttempts);
  const maxAttemptsValid = maxAttemptsNum === undefined || (Number.isInteger(maxAttemptsNum) && maxAttemptsNum >= 1 && maxAttemptsNum <= 10);
  const valid = form.name.trim() !== "" && urlValid && form.eventTypes.length > 0 && headerErrors.length === 0 && maxAttemptsValid;

  const preview = useMemo(() => fixturePayload(existing?.id ?? "", form.eventTypes), [existing, form.eventTypes]);

  function updateLabelRow(i: number, row: { key: string; value: string }) {
    setForm((f) => ({ ...f, labelRows: f.labelRows.map((r, idx) => (idx === i ? row : r)) }));
  }
  function updateHeaderRow(i: number, row: { key: string; value: string }) {
    setForm((f) => ({ ...f, headerRows: f.headerRows.map((r, idx) => (idx === i ? row : r)) }));
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true);
    setError(null);
    const label: Record<string, string> = {};
    for (const row of form.labelRows) if (row.key.trim()) label[row.key.trim()] = row.value;
    const headers: Record<string, string> = {};
    for (const row of form.headerRows) if (row.key.trim()) headers[row.key.trim()] = row.value;
    const filters = form.nameFilter.trim() || form.imageFilter.trim() || Object.keys(label).length
      ? { name: form.nameFilter.trim() || undefined, image: form.imageFilter.trim() || undefined, label: Object.keys(label).length ? label : undefined }
      : undefined;
    const body: Record<string, unknown> = {
      name: form.name.trim(),
      url: form.url.trim(),
      event_types: form.eventTypes,
      filters,
      headers: Object.keys(headers).length ? headers : undefined,
      max_attempts: maxAttemptsNum,
    };
    if (form.secret.trim()) body.secret = form.secret.trim();
    try {
      if (existing) await api.patch(`/webhooks/${existing.id}`, body);
      else await api.post("/webhooks", body);
      await queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      push(existing ? `Updated ${form.name.trim()}.` : `Created ${form.name.trim()}.`);
      close();
    } catch (err) {
      setError(errMsg(err, "Could not save webhook. Try again."));
      setBusy(false);
    }
  }

  return <Modal title={existing ? `Edit ${existing.name}` : "New webhook"} close={close} busy={busy} wide>
    <form onSubmit={(e) => void submit(e)} className="mt-3">
      <label className="block text-[13px]">Name<input autoFocus disabled={busy} value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} className={`${inputClass} font-sans`} /></label>
      <label className="mt-3 block text-[13px]">URL<input disabled={busy} value={form.url} onChange={(e) => setForm((f) => ({ ...f, url: e.target.value }))} placeholder="https://example.com/hooks/vessel" className={inputClass} /></label>
      {privateWarning && <p className="mb-0 mt-1 flex items-center gap-1.5 text-[12px] text-pause"><AlertTriangle size={13} /> This looks like a loopback or private-network host — fine for a local sink, but unusual for an external tool.</p>}

      <div className="mt-4 text-[13px] font-medium">Event types</div>
      <EventTypeSelect selected={form.eventTypes} onChange={(next) => setForm((f) => ({ ...f, eventTypes: next }))} />

      <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
        <label className="block text-[13px]">Name filter (glob)<input disabled={busy} value={form.nameFilter} onChange={(e) => setForm((f) => ({ ...f, nameFilter: e.target.value }))} placeholder="web-*" className={inputClass} /></label>
        <label className="block text-[13px]">Image filter (glob)<input disabled={busy} value={form.imageFilter} onChange={(e) => setForm((f) => ({ ...f, imageFilter: e.target.value }))} placeholder="nginx:*" className={inputClass} /></label>
      </div>

      <div className="mt-4 text-[13px] font-medium">Label filters</div>
      {form.labelRows.map((row, i) => <div key={i} className="mt-1.5 flex gap-2">
        <input disabled={busy} value={row.key} onChange={(e) => updateLabelRow(i, { ...row, key: e.target.value })} placeholder="key" className="w-1/2 rounded border border-line bg-panel px-2 py-1 text-[13px] font-mono" />
        <input disabled={busy} value={row.value} onChange={(e) => updateLabelRow(i, { ...row, value: e.target.value })} placeholder="value" className="w-1/2 rounded border border-line bg-panel px-2 py-1 text-[13px] font-mono" />
        <button type="button" onClick={() => setForm((f) => ({ ...f, labelRows: f.labelRows.filter((_, idx) => idx !== i) }))} className={dangerIconAction}><Trash2 size={14} /></button>
      </div>)}
      <button type="button" onClick={() => setForm((f) => ({ ...f, labelRows: [...f.labelRows, { key: "", value: "" }] }))} className="mt-1.5 text-[12.5px] text-hull hover:underline">+ Add label filter</button>

      <div className="mt-4 text-[13px] font-medium">Custom headers</div>
      {form.headerRows.map((row, i) => {
        const reserved = row.key.trim() && RESERVED_HEADER_RE.test(row.key.trim());
        return <div key={i}>
          <div className="mt-1.5 flex gap-2">
            <input disabled={busy} value={row.key} onChange={(e) => updateHeaderRow(i, { ...row, key: e.target.value })} placeholder="X-Custom-Header" className={`w-1/2 rounded border px-2 py-1 text-[13px] font-mono ${reserved ? "border-fail" : "border-line"} bg-panel`} />
            <input disabled={busy} value={row.value} onChange={(e) => updateHeaderRow(i, { ...row, value: e.target.value })} placeholder="value" className="w-1/2 rounded border border-line bg-panel px-2 py-1 text-[13px] font-mono" />
            <button type="button" onClick={() => setForm((f) => ({ ...f, headerRows: f.headerRows.filter((_, idx) => idx !== i) }))} className={dangerIconAction}><Trash2 size={14} /></button>
          </div>
          {reserved && <p className="mb-0 mt-1 text-[11.5px] text-fail">Reserved — the engine sets this header itself.</p>}
        </div>;
      })}
      <button type="button" onClick={() => setForm((f) => ({ ...f, headerRows: [...f.headerRows, { key: "", value: "" }] }))} className="mt-1.5 text-[12.5px] text-hull hover:underline">+ Add header</button>

      <label className="mt-4 block text-[13px]">
        Secret {existing?.secret_set && !form.secret && <Tag>secret set</Tag>}
        <div className="mt-1 flex gap-2">
          <input disabled={busy} type="text" value={form.secret} onChange={(e) => setForm((f) => ({ ...f, secret: e.target.value }))} placeholder={existing?.secret_set ? "•••• (unchanged — enter a new value to replace it)" : "Generate or paste a secret"} className={`${inputClass} flex-1`} />
          <Button type="button" disabled={busy} onClick={() => setForm((f) => ({ ...f, secret: randomSecretHex() }))}>Generate</Button>
        </div>
      </label>
      <p className="mb-0 mt-1 text-[11.5px] text-muted">Copy it now — it's never shown again. See the <a href={DOCS_LINK} className="text-hull hover:underline">signature verification guide</a>.</p>

      <label className="mt-4 block text-[13px]">Max attempts (1–10, blank = default 5-step schedule)<input disabled={busy} value={form.maxAttempts} onChange={(e) => setForm((f) => ({ ...f, maxAttempts: e.target.value.replace(/[^0-9]/g, "") }))} className={`${inputClass} w-32`} /></label>
      {!maxAttemptsValid && <p className="mb-0 mt-1 text-[12px] text-fail">Must be between 1 and 10.</p>}

      <div className="mt-4 text-[13px] font-medium">Live payload preview</div>
      <CodeBlock value={preview} />

      {error && <p className="mb-0 mt-3 text-[12px] text-fail">{error}</p>}
      <div className="mt-5 flex justify-end gap-2">
        <Button type="button" disabled={busy} onClick={close}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={!valid || busy}>{existing ? "Save" : "Create webhook"}</Button>
      </div>
    </form>
  </Modal>;
}

function RemoveWebhookModal({ webhook, close }: { webhook: Webhook; close: () => void }) {
  const queryClient = useQueryClient();
  const { push } = useToast();
  const navigate = useNavigate();
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const allowed = typed === webhook.name;

  async function remove() {
    setBusy(true);
    setError(null);
    try {
      await api.delete(`/webhooks/${webhook.id}`);
      await queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      push(`Removed ${webhook.name}.`);
      close();
      navigate("/webhooks");
    } catch (err) {
      setError(errMsg(err, "Could not remove webhook. Try again."));
      setBusy(false);
    }
  }

  return <Modal title={`Remove ${webhook.name}?`} close={close} busy={busy}>
    <p className="mt-3 text-[13px] text-muted">A webhook being removed mid-retry-backoff silently drops any deliveries still pending. This cannot be undone.</p>
    <label className="mt-3 block text-[13px]">Type <b>{webhook.name}</b> to confirm<input autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} className="mt-1 block w-full rounded border border-line px-2 py-1" /></label>
    {error && <p className="mb-0 mt-3 text-[12px] text-fail">{error}</p>}
    <div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={!allowed || busy} onClick={() => void remove()}>Remove</Button></div>
  </Modal>;
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

function DisableAllModal({ webhooks, close }: { webhooks: Webhook[]; close: () => void }) {
  const queryClient = useQueryClient();
  const { push } = useToast();
  const [busy, setBusy] = useState(false);
  const enabledOnes = webhooks.filter((w) => w.enabled);

  async function disableAll() {
    setBusy(true);
    try {
      await Promise.all(enabledOnes.map((w) => api.patch(`/webhooks/${w.id}`, { enabled: false })));
      await queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      push(`Disabled ${enabledOnes.length} webhook(s).`);
      close();
    } catch (err) {
      push(errMsg(err, "Could not disable all webhooks."), "error");
      setBusy(false);
    }
  }

  return <Modal title="Disable all webhooks?" close={close} busy={busy}>
    <p className="mt-3 text-[13px] text-muted">This disables {enabledOnes.length} enabled webhook(s). Reversible — turn any of them back on individually afterward.</p>
    <div className="mt-5 flex justify-end gap-2"><Button disabled={busy} onClick={close}>Cancel</Button><Button variant="danger" disabled={busy} onClick={() => void disableAll()}>Disable all</Button></div>
  </Modal>;
}

function EventChips({ types }: { types: string[] }) {
  const shown = types.slice(0, 3);
  const rest = types.length - shown.length;
  return <span className="flex flex-wrap gap-1">
    {shown.map((t) => <Tag key={t}>{t}</Tag>)}
    {rest > 0 && <Tag>+{rest} types</Tag>}
  </span>;
}

export function WebhooksPage() {
  const queryClient = useQueryClient();
  const { push } = useToast();
  const [creating, setCreating] = useState(false);
  const [disableAll, setDisableAll] = useState(false);
  const webhooks = useQuery({ queryKey: ["webhooks"], queryFn: () => api.get<{ webhooks: Webhook[] }>("/webhooks") });
  const rows = webhooks.data?.webhooks ?? [];

  async function toggle(w: Webhook) {
    // Optimistic — per SPEC §9.2 an enable/disable toggle is immediate, no confirmation.
    queryClient.setQueryData<{ webhooks: Webhook[] }>(["webhooks"], (data) =>
      data ? { webhooks: data.webhooks.map((x) => (x.id === w.id ? { ...x, enabled: !x.enabled } : x)) } : data);
    try {
      await api.patch(`/webhooks/${w.id}`, { enabled: !w.enabled });
    } catch (err) {
      push(errMsg(err, `Could not update ${w.name}.`), "error");
      await queryClient.invalidateQueries({ queryKey: ["webhooks"] });
    }
  }

  if (webhooks.isLoading) return <EmptyState title="Loading webhooks" action="…" />;
  if (webhooks.isError) return <EmptyState title="Could not load webhooks" action={errMsg(webhooks.error, "Refresh the page and try again.")} />;

  return <section>
    <div className="mb-3 flex items-center gap-2">
      <h1 className="m-0 text-lg">Webhooks</h1>
      <span className="flex-1" />
      <Can do="webhooks.update"><Button disabled={!rows.some((w) => w.enabled)} onClick={() => setDisableAll(true)}>Disable all</Button></Can>
      <Can do="webhooks.create"><Button variant="primary" onClick={() => setCreating(true)}><Plus size={14} className="mr-1 inline" />New webhook</Button></Can>
    </div>
    <div className="overflow-x-auto rounded border border-line bg-panel">
      {rows.length === 0 ? <EmptyState title="No webhooks configured yet" action={<>Create one to notify an external tool on container, image, volume, or network events. See the <a href={DOCS_LINK} className="text-hull hover:underline">docs</a>.</>} /> : <table className="w-full min-w-[820px] border-collapse text-[13px]">
        <thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted">
          <tr><th className="px-3">Name</th><th className="px-3">Host</th><th className="px-3">Events</th><th className="px-3">Enabled</th><th className="px-3">Last delivery</th><th className="px-3">24h success</th><th className="w-[90px] px-3" /></tr>
        </thead>
        <tbody>{rows.map((w) => <tr key={w.id} className="group h-row border-b border-linesoft last:border-0 hover:bg-paper">
          <td className="px-3 font-medium"><Link to={`/webhooks/${w.id}`} className="no-underline hover:underline">{w.name}</Link></td>
          <td className="px-3 font-mono text-[12px] text-muted" title="Host only — the full URL may embed a token">{hostOf(w.url)}</td>
          <td className="px-3"><EventChips types={w.event_types} /></td>
          <td className="px-3 align-middle">
            <Can do="webhooks.update"><Switch checked={w.enabled} onChange={() => void toggle(w)} aria-label={`${w.enabled ? "Disable" : "Enable"} ${w.name}`} /></Can>
          </td>
          <td className="px-3 font-mono text-[12px] text-muted">{w.last_delivery ? <span className={w.last_delivery.status === "success" ? "text-run" : "text-fail"}>{w.last_delivery.status} · {relativeTime(w.last_delivery.created_at)}</span> : "—"}</td>
          <td className="px-3 font-mono text-[12px] text-muted">{w.stats_24h.sent > 0 ? `${Math.round(w.stats_24h.success_rate * 100)}% (${w.stats_24h.sent})` : "—"}</td>
          <td className="px-3 text-right">
            <span className="flex items-center justify-end gap-0.5 opacity-0 group-hover:opacity-100 focus-within:opacity-100">
              <Link to={`/webhooks/${w.id}`} title="Open" aria-label={`Open ${w.name}`} className={iconAction}><Pencil size={15} /></Link>
            </span>
          </td>
        </tr>)}</tbody>
      </table>}
    </div>
    {rows.some((w) => w.webhook_dropped_total > 0) && <p className="mt-3 text-[12.5px] text-pause">
      {rows.find((w) => w.webhook_dropped_total > 0)?.webhook_dropped_total} events dropped — the delivery queue was full. Webhooks firing too slowly or too often for the event volume.
    </p>}
    {creating && <WebhookFormModal close={() => setCreating(false)} />}
    {disableAll && <DisableAllModal webhooks={rows} close={() => setDisableAll(false)} />}
  </section>;
}

// ---------------------------------------------------------------------------
// Detail: config summary + delivery log
// ---------------------------------------------------------------------------

function DeliveryRow({ delivery, webhookId }: { delivery: Delivery; webhookId: string }) {
  const queryClient = useQueryClient();
  const { push } = useToast();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  async function redeliver() {
    setBusy(true);
    try {
      await api.post(`/deliveries/${delivery.id}/redeliver`);
      await queryClient.invalidateQueries({ queryKey: ["deliveries", webhookId] });
      push("Redelivery queued.");
    } catch (err) {
      push(errMsg(err, "Could not redeliver."), "error");
    } finally {
      setBusy(false);
    }
  }

  return <>
    <tr className="h-row cursor-pointer border-b border-linesoft last:border-0 hover:bg-paper" onClick={() => setOpen((v) => !v)}>
      <td className="px-3 font-mono text-[12px] text-muted">{delivery.attempt}</td>
      <td className="px-3"><StatusTag status={delivery.status} /></td>
      <td className="px-3 font-mono text-[12px] text-muted">{delivery.status_code ?? "—"}</td>
      <td className="px-3 font-mono text-[12px] text-muted">{delivery.response_ms != null ? `${delivery.response_ms} ms` : "—"}</td>
      <td className="px-3 font-mono text-[12px] text-muted">{relativeTime(delivery.created_at)}</td>
      <td className="px-3 text-right">
        <Can do="deliveries.redeliver"><button disabled={busy} title="Redeliver" aria-label="Redeliver" onClick={(e) => { e.stopPropagation(); void redeliver(); }} className={iconAction}><RotateCw size={14} /></button></Can>
      </td>
    </tr>
    {open && <tr><td colSpan={6} className="p-0"><DeliveryPanel delivery={delivery} /></td></tr>}
  </>;
}

function DeliveryLog({ webhookId }: { webhookId: string }) {
  const [status, setStatus] = useState<DeliveryStatus | "">("");
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const deliveries = useQuery({
    queryKey: ["deliveries", webhookId, status, cursor],
    queryFn: () => api.get<{ deliveries: Delivery[] }>(`/webhooks/${webhookId}/deliveries?limit=50${status ? `&status=${status}` : ""}${cursor ? `&cursor=${cursor}` : ""}`),
  });
  const rows = deliveries.data?.deliveries ?? [];

  return <div>
    <div className="mb-2 flex items-center gap-2">
      <h2 className="m-0 text-[14px] font-semibold">Delivery log</h2>
      <select value={status} onChange={(e) => { setStatus(e.target.value as DeliveryStatus | ""); setCursor(undefined); }} aria-label="Filter by status" className="ml-3 rounded border border-line bg-panel px-2 py-1 text-[12.5px]">
        <option value="">All statuses</option>
        <option value="pending">Pending</option>
        <option value="success">Success</option>
        <option value="failed">Failed</option>
        <option value="dead">Dead</option>
      </select>
    </div>
    <div className="overflow-x-auto rounded border border-line bg-panel">
      {rows.length === 0 ? <EmptyState title="No deliveries yet" action={<>Deliveries appear here once a matching event fires, or after a "Test webhook" run. See the <a href={DOCS_LINK} className="text-hull hover:underline">signature verification docs</a>.</>} /> : <table className="w-full min-w-[560px] border-collapse text-[13px]">
        <thead className="border-b border-line bg-paper text-left text-[11.5px] uppercase tracking-wide text-muted"><tr><th className="px-3">Attempt</th><th className="px-3">Status</th><th className="px-3">HTTP</th><th className="px-3">Latency</th><th className="px-3">Time</th><th className="w-[70px] px-3" /></tr></thead>
        <tbody>{rows.map((d) => <DeliveryRow key={d.id} delivery={d} webhookId={webhookId} />)}</tbody>
      </table>}
    </div>
    {rows.length >= 50 && <button className="mt-2 text-[12.5px] text-hull hover:underline" onClick={() => setCursor(rows[rows.length - 1].id)}>Load older deliveries</button>}
  </div>;
}

export function WebhookDetailPage() {
  const { id = "" } = useParams();
  const { push } = useToast();
  const [editing, setEditing] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [testResult, setTestResult] = useState<Delivery | null>(null);
  const [testing, setTesting] = useState(false);
  const webhook = useQuery({ queryKey: ["webhooks", id], queryFn: () => api.get<Webhook>(`/webhooks/${id}`) });

  async function runTest() {
    setTesting(true);
    setTestResult(null);
    try {
      await api.post(`/webhooks/${id}/test`);
      push("Test delivery queued — check the delivery log below shortly.");
    } catch (err) {
      push(errMsg(err, "Could not run test."), "error");
    } finally {
      setTesting(false);
    }
  }

  if (webhook.isLoading) return <EmptyState title="Loading webhook" action="…" />;
  if (webhook.isError || !webhook.data) return <EmptyState title="Webhook not found" action={<Link to="/webhooks">Back to webhooks</Link>} />;
  const w = webhook.data;

  return <section>
    <div className="mb-3 flex items-center gap-2">
      <Link to="/webhooks" className="text-[12.5px] text-muted hover:underline">← Webhooks</Link>
    </div>
    <div className="mb-4 flex items-center gap-3">
      <h1 className="m-0 text-lg">{w.name}</h1>
      <span className={`h-2 w-2 rounded-full ${w.enabled ? "bg-run" : "bg-line"}`} />
      <span className="flex-1" />
      <Can do="webhooks.test"><Button disabled={testing} onClick={() => void runTest()}><Send size={14} className="mr-1 inline" />Test webhook</Button></Can>
      <Can do="webhooks.update"><Button onClick={() => setEditing(true)}>Edit</Button></Can>
      <Can do="webhooks.remove"><Button variant="danger" onClick={() => setRemoving(true)}>Remove</Button></Can>
    </div>

    <div className="mb-5 grid grid-cols-1 gap-3 rounded border border-line bg-panel p-3.5 text-[13px] sm:grid-cols-2">
      <div><span className="text-muted">URL host</span><div className="font-mono">{hostOf(w.url)}</div></div>
      <div><span className="text-muted">Event types</span><div><EventChips types={w.event_types} /></div></div>
      <div><span className="text-muted">Max attempts</span><div>{w.max_attempts || "default (5-step schedule)"}</div></div>
      <div><span className="text-muted">Secret</span><div>{w.secret_set ? <Tag>secret set</Tag> : <span className="text-muted">not set</span>} — <a href={DOCS_LINK} className="text-hull hover:underline">verify signatures</a></div></div>
      {w.webhook_dropped_total > 0 && <div className="sm:col-span-2 text-pause">{w.webhook_dropped_total} events dropped — the delivery queue was full. Webhooks firing too slowly or too often for the event volume.</div>}
    </div>

    {testResult && <div className="mb-5"><h2 className="m-0 mb-1 text-[14px] font-semibold">Test result</h2><DeliveryPanel delivery={testResult} /></div>}

    <DeliveryLog webhookId={w.id} />

    {editing && <WebhookFormModal existing={w} close={() => setEditing(false)} />}
    {removing && <RemoveWebhookModal webhook={w} close={() => setRemoving(false)} />}
  </section>;
}
