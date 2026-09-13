import { useEffect, useRef, useState } from "react";
import type { Terminal as XTerm } from "@xterm/xterm";
import type { FitAddon as XFitAddon } from "@xterm/addon-fit";
import { api } from "../../lib/api";
import { basePath } from "../../lib/basePath";
import { usePrefersReducedMotion } from "../../lib/useReducedMotion";
import { Button } from "../ui/Button";

export type ConsoleStatus = "idle" | "connecting" | "connected" | "disconnected";

const RESIZE_DEBOUNCE_MS = 100;

function wsURL(containerID: string, params: URLSearchParams): string {
  const base = basePath();
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  const prefix = base === "/" ? "" : base;
  return `${proto}//${window.location.host}${prefix}/api/v1/containers/${encodeURIComponent(containerID)}/exec?${params.toString()}`;
}

/** Maps M11's WS close-path info messages 1:1 to the banners M19 requires. */
function disconnectBanner(message: string): string {
  switch (message) {
    case "container exited, session closed":
      return "[vessel] container exited, session closed";
    case "session closed after 15 minutes of inactivity":
      return "[vessel] disconnected after 15m idle";
    case "console stream closed":
    case "resize failed":
      return "[vessel] connection lost — check the Docker socket";
    default:
      return `[vessel] ${message}`;
  }
}

export default function Console({ containerID, authOn, onStatusChange }: { containerID: string; authOn: boolean; onStatusChange?: (status: ConsoleStatus) => void }) {
  const [status, setStatus] = useState<ConsoleStatus>("idle");
  const [banner, setBanner] = useState<string | null>(null);
  const [fallback, setFallback] = useState<string | null>(null);
  const [cmd, setCmd] = useState("/bin/sh");
  const [customCmd, setCustomCmd] = useState("");
  const [user, setUser] = useState("");
  const reducedMotion = usePrefersReducedMotion();

  const boxRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<XFitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const openedRef = useRef(false);
  const manualCloseRef = useRef(false);
  const resizeTimer = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => { onStatusChange?.(status); }, [status, onStatusChange]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [{ Terminal }, { FitAddon }] = await Promise.all([import("@xterm/xterm"), import("@xterm/addon-fit")]);
      await import("@xterm/xterm/css/xterm.css");
      if (cancelled || !boxRef.current) return;
      const term = new Terminal({ cursorBlink: !reducedMotion, fontSize: 13, fontFamily: "ui-monospace, SFMono-Regular, monospace", theme: { background: "#0B1F2A" } });
      const fit = new FitAddon();
      term.loadAddon(fit);
      term.open(boxRef.current);
      fit.fit();
      term.onData((data) => { if (wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(data); });
      term.onSelectionChange(() => { const sel = term.getSelection(); if (sel) void navigator.clipboard?.writeText(sel).catch(() => undefined); });
      term.onResize(({ cols, rows }) => {
        if (resizeTimer.current) clearTimeout(resizeTimer.current);
        resizeTimer.current = setTimeout(() => {
          if (wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(JSON.stringify({ type: "resize", cols, rows }));
        }, RESIZE_DEBOUNCE_MS);
      });
      termRef.current = term;
      fitRef.current = fit;
    })();
    return () => {
      cancelled = true;
      if (resizeTimer.current) clearTimeout(resizeTimer.current);
      wsRef.current?.close();
      termRef.current?.dispose();
      termRef.current = null;
      fitRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [containerID]);

  useEffect(() => {
    const term = termRef.current;
    if (term) term.options.cursorBlink = !reducedMotion;
  }, [reducedMotion]);

  useEffect(() => {
    const el = boxRef.current;
    if (!el) return;
    const observer = new ResizeObserver(() => fitRef.current?.fit());
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  async function connect() {
    const effectiveCmd = (cmd === "custom" ? customCmd : cmd).trim() || "/bin/sh";
    setStatus("connecting");
    setBanner(null);
    setFallback(null);
    manualCloseRef.current = false;
    openedRef.current = false;
    termRef.current?.clear();
    try {
      const params = new URLSearchParams({ cmd: effectiveCmd, tty: "1" });
      if (authOn) {
        const { ticket } = await api.post<{ ticket: string; expires_in: number }>("/auth/ws-ticket");
        params.set("ticket", ticket);
      }
      if (user.trim()) params.set("user", user.trim());
      const ws = new WebSocket(wsURL(containerID, params));
      wsRef.current = ws;
      ws.onopen = () => {
        openedRef.current = true;
        setStatus("connected");
        termRef.current?.focus();
      };
      ws.onmessage = (event) => {
        if (typeof event.data !== "string") return;
        let msg: { type?: string; message?: string } | null = null;
        try {
          msg = JSON.parse(event.data);
        } catch {
          msg = null;
        }
        if (msg && msg.type === "info" && typeof msg.message === "string") {
          if (msg.message.startsWith("using ")) setFallback(`[vessel] ${msg.message}`);
          else setBanner(disconnectBanner(msg.message));
          return;
        }
        termRef.current?.write(event.data);
      };
      ws.onclose = () => {
        setStatus("disconnected");
        if (!openedRef.current && !manualCloseRef.current) setBanner("[vessel] session rejected — ticket expired, reconnect to get a new one");
      };
      ws.onerror = () => {
        if (!openedRef.current) setBanner("[vessel] connection lost — check the Docker socket");
      };
    } catch {
      setStatus("disconnected");
      setBanner("[vessel] connection lost — check the Docker socket");
    }
  }

  function disconnect() {
    manualCloseRef.current = true;
    wsRef.current?.close(1000, "client disconnect");
    wsRef.current = null;
  }

  const live = status === "connecting" || status === "connected";

  return <div>
    <div className="mb-2 flex flex-wrap items-end gap-2">
      <label className="text-[12px] text-muted">Shell<br />
        <select disabled={live} value={cmd} onChange={(e) => setCmd(e.target.value)} className="mt-1 rounded border border-line bg-panel px-2 py-1 text-[12px] text-text disabled:opacity-60">
          <option value="/bin/sh">/bin/sh</option>
          <option value="/bin/bash">/bin/bash</option>
          <option value="custom">custom…</option>
        </select>
      </label>
      {cmd === "custom" && <label className="text-[12px] text-muted">Command<br />
        <input disabled={live} value={customCmd} onChange={(e) => setCustomCmd(e.target.value)} placeholder="/bin/zsh" className="mt-1 rounded border border-line bg-panel px-2 py-1 text-[12px] text-text disabled:opacity-60" />
      </label>}
      <label className="text-[12px] text-muted">User<br />
        <input disabled={live} value={user} onChange={(e) => setUser(e.target.value)} placeholder="root" className="mt-1 rounded border border-line bg-panel px-2 py-1 text-[12px] text-text disabled:opacity-60" />
      </label>
      {live && <span className="pb-1 text-[12px] italic text-muted">Disconnect to change</span>}
      <span className="flex-1" />
      {live
        ? <Button onClick={disconnect}>Disconnect</Button>
        : <Button variant="primary" onClick={() => void connect()}>{status === "disconnected" ? "Reconnect" : "Connect"}</Button>}
    </div>
    {fallback && <div className="mb-2 rounded border border-pause/40 bg-pause/10 px-3 py-1.5 font-mono text-[12px] text-[#8A6A1E]">{fallback}</div>}
    {banner && <div className="mb-2 rounded border border-line bg-paper px-3 py-1.5 font-mono text-[12px] text-muted">{banner}</div>}
    <div ref={boxRef} className="h-[420px] overflow-hidden rounded border border-line bg-[#0B1F2A] p-2" />
  </div>;
}
