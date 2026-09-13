import { useQuery } from "@tanstack/react-query";
import { EmptyState } from "../../components/ui/EmptyState";
import { api } from "../../lib/api";
import type { Host, VersionInfo } from "../../types/api";

function Row({ label, value }: { label: string; value: string }) {
  return <><dt className="text-muted">{label}</dt><dd className="min-w-0 break-all font-mono">{value}</dd></>;
}

export function AboutTab() {
  const version = useQuery({ queryKey: ["version"], queryFn: () => api.get<VersionInfo>("/version") });
  const host = useQuery({ queryKey: ["host"], queryFn: () => api.get<Host>("/host") });
  if (version.isLoading || host.isLoading) return <EmptyState title="Loading server info" action="…" />;
  const v = version.data; const h = host.data;
  return <div className="grid gap-3 md:grid-cols-2">
    <div className="rounded border border-line bg-panel p-4">
      <h2 className="m-0 mb-3 text-[14px]">This instance</h2>
      <dl className="grid grid-cols-[minmax(100px,auto)_1fr] gap-x-4 gap-y-2 text-[12px]">
        <Row label="Version" value={v ? `${v.version} (${v.commit})` : "—"} />
        <Row label="Build date" value={v?.date ?? "—"} />
        <Row label="Auth" value={v?.auth_mode ?? "—"} />
        <Row label="Read-only" value={v ? (v.read_only ? "on" : "off") : "—"} />
        <Row label="Console (exec)" value={v ? (v.allow_exec ? "enabled" : "disabled") : "—"} />
        <Row label="Store" value={v?.store_mode ?? "—"} />
      </dl>
    </div>
    <div className="rounded border border-line bg-panel p-4">
      <h2 className="m-0 mb-3 text-[14px]">Docker engine</h2>
      <dl className="grid grid-cols-[minmax(100px,auto)_1fr] gap-x-4 gap-y-2 text-[12px]">
        <Row label="Engine version" value={h?.server_version ?? "—"} />
        <Row label="API version" value={h?.api_version ?? "—"} />
        <Row label="OS" value={h ? `${h.operating_system} (${h.os_type})` : "—"} />
        <Row label="Architecture" value={h?.architecture ?? "—"} />
        <Row label="Kernel" value={h?.kernel_version ?? "—"} />
      </dl>
    </div>
    <p className="text-[12px] text-muted md:col-span-2">
      <a href="https://vessel.dev/docs" target="_blank" rel="noreferrer" className="text-link underline">Documentation</a>
    </p>
  </div>;
}
