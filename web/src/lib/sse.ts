import { useEffect, useRef, useState } from "react";
import { basePath } from "./basePath";
import { getToken } from "./api";

export type SSEStatus = "connecting" | "open" | "reconnecting" | "closed";

type Handlers = Record<string, (data: string) => void>;

function apiUrl(path: string): string {
  const base = basePath();
  return (base === "/" ? "" : base) + "/api/v1" + path;
}

/**
 * Wraps EventSource with typed named events and backoff reconnect. Every
 * streaming page (logs, stats, events, pull progress) shares this hook.
 */
export function useSSE(path: string | null, handlers: Handlers, options: { reconnect?: boolean } = {}): SSEStatus {
  const [status, setStatus] = useState<SSEStatus>("connecting");
  const handlersRef = useRef(handlers);
  handlersRef.current = handlers;

  useEffect(() => {
    if (!path) {
      setStatus("closed");
      return;
    }

    let cancelled = false;
    let es: EventSource | null = null;
    let attempt = 0;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;

    const token = getToken();
    const url = apiUrl(path) + (token ? (path.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(token) : "");

    function connect() {
      if (cancelled) return;
      setStatus(attempt === 0 ? "connecting" : "reconnecting");
      es = new EventSource(url, { withCredentials: true });

      es.onopen = () => {
        attempt = 0;
        setStatus("open");
      };

      es.onerror = () => {
        es?.close();
        if (cancelled) return;
        if (options.reconnect === false) {
          setStatus("closed");
          return;
        }
        attempt += 1;
        const delay = Math.min(30_000, 500 * 2 ** attempt);
        setStatus("reconnecting");
        retryTimer = setTimeout(connect, delay);
      };

      for (const event of Object.keys(handlersRef.current)) {
        es.addEventListener(event, (e) => handlersRef.current[event]?.((e as MessageEvent).data));
      }
    }

    connect();

    return () => {
      cancelled = true;
      if (retryTimer) clearTimeout(retryTimer);
      es?.close();
      setStatus("closed");
    };
  }, [path, options.reconnect]);

  return status;
}
