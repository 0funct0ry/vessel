import { useState } from "react";
import { Maximize2, Minimize2 } from "lucide-react";

/** Shared Tailwind modal shell. Every dialog in the app — confirmations,
 * forms, multi-step flows — uses this instead of a native window.prompt/
 * confirm/alert, which can't be themed, block the main thread, and can't
 * hold more than one field. A `fullscreen` modal may also pass
 * `maximizable` to offer a maximize/restore toggle in the header that
 * expands it to the full browser viewport (no rounding/margin) and back. */
export function Modal({ title, subtitle, icon, children, close, busy = false, wide = false, fullscreen = false, maximizable = false }: { title: string; subtitle?: string; icon?: React.ReactNode; children: React.ReactNode; close: () => void; busy?: boolean; wide?: boolean; fullscreen?: boolean; maximizable?: boolean }) {
  const [maximized, setMaximized] = useState(false);
  const sizeClass = fullscreen
    ? maximized
      ? "flex h-screen w-screen max-w-none flex-col overflow-hidden rounded-none p-0"
      : "flex h-[92vh] w-[96vw] max-w-[1400px] flex-col overflow-hidden p-0"
    : `max-h-[90vh] overflow-auto p-5 ${wide ? "max-w-3xl" : "max-w-2xl"}`;
  return <div role="dialog" aria-modal="true" aria-label={title} className={`fixed inset-0 z-40 grid place-items-center bg-ink/45 ${maximized ? "" : "p-4"}`} onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) close(); }}>
    <div className={`w-full rounded border border-line bg-panel shadow-lg ${sizeClass}`}>
      <div className={`flex items-center gap-3 ${fullscreen ? "border-b border-line px-5 py-4" : ""}`}>
        {icon}
        <div>
          <h2 className="m-0 text-lg leading-tight">{title}</h2>
          {subtitle && <p className="m-0 text-[12px] text-muted">{subtitle}</p>}
        </div>
        <span className="ml-auto flex items-center gap-1">
          {fullscreen && maximizable && <button type="button" aria-label={maximized ? `Restore ${title}` : `Maximize ${title}`} title={maximized ? "Restore" : "Maximize"} onClick={() => setMaximized((v) => !v)} className="rounded p-1.5 text-muted hover:bg-paper hover:text-text">
            {maximized ? <Minimize2 size={16} /> : <Maximize2 size={16} />}
          </button>}
          <button type="button" aria-label={`Close ${title}`} disabled={busy} onClick={close} className="rounded px-2 text-xl text-muted hover:bg-paper hover:text-text">×</button>
        </span>
      </div>
      {children}
    </div>
  </div>;
}
