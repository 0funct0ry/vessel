import { useEffect, useState } from "react";
import { useSSE } from "../../lib/sse";
import { bytes } from "../../lib/containers";
import { EmptyState } from "../ui/EmptyState";
import type { Stats as StatsSample } from "../../types/api";

// Server default is a 2s interval (clamped 1s-30s); 90 samples is ~3 minutes
// of history — plenty for a trend on a detail-page tab, far cheaper than
// LogViewer's 10,000-line cap (which exists for cheap, fast-arriving lines;
// each stats sample here renders into 4 sparklines).
const MAX_SAMPLES = 90;

function latestNonNull(samples: StatsSample[]): number | null {
  for (let i = samples.length - 1; i >= 0; i--) {
    const v = samples[i].cpu_pct;
    if (v !== null) return v;
  }
  return null;
}

/** A hand-rolled sparkline — no charting library in this repo (see SPEC §9). */
function sparkline(points: number[], color: string, max?: number) {
  const w = 240, h = 40;
  if (points.length < 2) {
    return <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} preserveAspectRatio="none" aria-hidden="true" />;
  }
  const ceiling = Math.max(max ?? 0, ...points, 0.0001) * 1.25;
  const step = w / (points.length - 1);
  const line = points.map((p, i) => `${i * step},${h - (p / ceiling) * h}`).join(" ");
  return (
    <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} preserveAspectRatio="none" aria-hidden="true">
      <polygon points={`0,${h} ${line} ${w},${h}`} fill={color} opacity=".13" />
      <polyline points={line} fill="none" stroke={color} strokeWidth="1.5" />
    </svg>
  );
}

/** Same as sparkline(), but two series sharing one Y-axis (rx/tx, read/write). */
function dualSparkline(a: number[], colorA: string, b: number[], colorB: string) {
  const w = 240, h = 40;
  const n = Math.max(a.length, b.length);
  if (n < 2) {
    return <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} preserveAspectRatio="none" aria-hidden="true" />;
  }
  const ceiling = Math.max(...a, ...b, 0.0001) * 1.25;
  const step = w / (n - 1);
  const toLine = (points: number[]) => points.map((p, i) => `${i * step},${h - (p / ceiling) * h}`).join(" ");
  const lineA = toLine(a);
  return (
    <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} preserveAspectRatio="none" aria-hidden="true">
      <polygon points={`0,${h} ${lineA} ${w},${h}`} fill={colorA} opacity=".13" />
      <polyline points={lineA} fill="none" stroke={colorA} strokeWidth="1.5" />
      <polyline points={toLine(b)} fill="none" stroke={colorB} strokeWidth="1.5" />
    </svg>
  );
}

function Card({ title, subtitle, children }: { title: string; subtitle: string; children: React.ReactNode }) {
  return (
    <div className="rounded border border-line bg-panel p-4">
      <h2 className="m-0 mb-3 text-[14px]">{title}</h2>
      <p className="mb-2 font-mono text-[12.5px] text-muted">{subtitle}</p>
      {children}
    </div>
  );
}

export function Stats({ containerID, running, cpus, memoryLimit }: { containerID: string; running: boolean; cpus: number; memoryLimit: number }) {
  const [samples, setSamples] = useState<StatsSample[]>([]);

  useEffect(() => {
    setSamples([]);
  }, [containerID]);

  const path = running ? `/containers/${encodeURIComponent(containerID)}/stats` : null;
  const status = useSSE(path, {
    stats: (raw) => {
      try {
        const parsed = JSON.parse(raw) as StatsSample;
        setSamples((current) => [...current, parsed].slice(-MAX_SAMPLES));
      } catch {
        // ignore malformed frame
      }
    },
  }, { reconnect: true });

  if (!running) {
    return <EmptyState title="Stats are only available for running containers." action="Start the container, then return to this tab." />;
  }

  const latest = samples[samples.length - 1];
  const cpuSeries = samples.map((s) => s.cpu_pct).filter((v): v is number => v !== null);
  const memSeries = samples.map((s) => s.mem.used);
  const netRxSeries = samples.map((s) => s.net.rx);
  const netTxSeries = samples.map((s) => s.net.tx);
  const blkReadSeries = samples.map((s) => s.blk.read);
  const blkWriteSeries = samples.map((s) => s.blk.write);

  const cpuPct = latestNonNull(samples);
  const cpuSubtitle = `CPU · ${cpuPct !== null ? cpuPct.toFixed(1) : "—"}%${cpus ? ` of ${cpus.toFixed(2)} CPUs` : ""}`;
  const memSubtitle = latest ? `Memory · ${bytes(latest.mem.used)} / ${bytes(memoryLimit || latest.mem.limit)}` : "Memory · —";
  const netSubtitle = latest ? `Net · RX ${bytes(latest.net.rx)} / TX ${bytes(latest.net.tx)}` : "Net · —";
  const blkSubtitle = latest ? `Block I/O · Read ${bytes(latest.blk.read)} / Write ${bytes(latest.blk.write)}` : "Block I/O · —";

  return (
    <div>
      <div className="grid gap-3 md:grid-cols-2">
        <Card title="CPU" subtitle={cpuSubtitle}>{sparkline(cpuSeries, "var(--c-hull)", 100)}</Card>
        <Card title="Memory" subtitle={memSubtitle}>{sparkline(memSeries, "var(--c-run)")}</Card>
        <Card title="Net" subtitle={netSubtitle}>{dualSparkline(netRxSeries, "var(--c-link)", netTxSeries, "var(--c-pause)")}</Card>
        <Card title="Block I/O" subtitle={blkSubtitle}>{dualSparkline(blkReadSeries, "var(--c-link)", blkWriteSeries, "var(--c-pause)")}</Card>
      </div>
      {status !== "open" && <p className="mt-3 text-[12px] text-muted">Stream: {status}</p>}
    </div>
  );
}
