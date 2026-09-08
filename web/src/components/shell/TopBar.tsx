import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { api } from "../../lib/api";
import { usePrefersReducedMotion } from "../../lib/useReducedMotion";

interface HostInfo {
  server_version: string;
  api_version: string;
}

function breadcrumb(pathname: string): string {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length === 0) return "Dashboard";
  return segments
    .map((s) => s.charAt(0).toUpperCase() + s.slice(1))
    .join(" / ");
}

export function TopBar() {
  const location = useLocation();
  const [host, setHost] = useState<HostInfo | null>(null);
  const reducedMotion = usePrefersReducedMotion();

  useEffect(() => {
    api
      .get<HostInfo>("/host")
      .then(setHost)
      .catch(() => setHost(null));
  }, []);

  return (
    <header className="flex h-12 items-center border-b border-line bg-panel px-[18px]">
      <div className="text-[15px] text-text">{breadcrumb(location.pathname)}</div>

      <div className="ml-auto flex items-center gap-4 text-[11.5px] text-muted">
        {host && (
          <span className="font-mono">
            Engine {host.server_version} · API {host.api_version}
          </span>
        )}
        <span className="flex items-center gap-1.5">
          <i
            aria-hidden
            className={`inline-block h-[7px] w-[7px] rounded-full bg-run ${reducedMotion ? "" : "animate-pulse"}`}
          />
          events live
        </span>
      </div>
    </header>
  );
}
