import { useEffect, useMemo, useRef, useState } from "react";
import { getToken } from "../../lib/api";
import { basePath } from "../../lib/basePath";
import { useSSE } from "../../lib/sse";
import { usePrefersReducedMotion } from "../../lib/useReducedMotion";
import type { LogLine } from "../../types/api";
import { Button } from "../ui/Button";

const MAX_LINES = 10_000;
const ROW_HEIGHT = 20;
const OVERSCAN = 12;

type Stream = "both" | "stdout" | "stderr";
type Tail = "100" | "500" | "2000" | "all";

function query(options: { follow: boolean; tail: Tail; stream: Stream; timestamps: boolean }) {
  const params = new URLSearchParams({ follow: String(options.follow), tail: options.tail, stream: options.stream, timestamps: String(options.timestamps) });
  return params.toString();
}

function highlight(text: string, needle: string) {
  if (!needle) return text;
  const parts: React.ReactNode[] = [];
  const lower = text.toLocaleLowerCase();
  const match = needle.toLocaleLowerCase();
  let from = 0;
  let at = lower.indexOf(match, from);
  while (at !== -1) {
    if (at > from) parts.push(text.slice(from, at));
    parts.push(<mark key={`${at}-${from}`}>{text.slice(at, at + match.length)}</mark>);
    from = at + match.length;
    at = lower.indexOf(match, from);
  }
  if (from < text.length) parts.push(text.slice(from));
  return parts;
}

function timestamp(value?: string) {
  if (!value) return "";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toISOString().slice(11, 23);
}

export function LogViewer({ containerID }: { containerID: string }) {
  const [follow, setFollow] = useState(true);
  const [tail, setTail] = useState<Tail>("500");
  const [stream, setStream] = useState<Stream>("both");
  const [timestamps, setTimestamps] = useState(true);
  const [filter, setFilter] = useState("");
  const [wrap, setWrap] = useState(true);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [scrollTop, setScrollTop] = useState(0);
  const [height, setHeight] = useState(420);
  const [width, setWidth] = useState(800);
  const [atLatest, setAtLatest] = useState(true);
  const [unseen, setUnseen] = useState(0);
  const pending = useRef<LogLine[]>([]);
  const frame = useRef<number>();
  const box = useRef<HTMLDivElement>(null);
  const reducedMotion = usePrefersReducedMotion();

  const path = `/containers/${encodeURIComponent(containerID)}/logs?${query({ follow, tail, stream, timestamps })}`;
  const status = useSSE(path, {
    log: (raw) => {
      try {
        pending.current.push(JSON.parse(raw) as LogLine);
      } catch {
        return;
      }
      if (frame.current !== undefined) return;
      frame.current = requestAnimationFrame(() => {
        frame.current = undefined;
        const next = pending.current.splice(0);
        if (!next.length) return;
        setLines((current) => [...current, ...next].slice(-MAX_LINES));
        if (!atLatest || reducedMotion) setUnseen((count) => count + next.length);
      });
    },
  }, { reconnect: follow });

  useEffect(() => () => { if (frame.current !== undefined) cancelAnimationFrame(frame.current); }, []);
  useEffect(() => { setLines([]); setUnseen(0); setScrollTop(0); setAtLatest(true); }, [containerID, follow, tail, stream, timestamps]);
  useEffect(() => {
    const element = box.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => { setHeight(entry.contentRect.height); setWidth(entry.contentRect.width); });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const filtered = useMemo(() => {
    const match = filter.trim().toLocaleLowerCase();
    return match ? lines.filter((line) => line.line.toLocaleLowerCase().includes(match)) : lines;
  }, [filter, lines]);
  const rowHeights = useMemo(() => filtered.map((line) => {
    if (!wrap) return ROW_HEIGHT;
    const chars = Math.max(24, Math.floor((width - (timestamps ? 100 : 32)) / 7.5));
    return ROW_HEIGHT * Math.max(1, Math.ceil(line.line.length / chars));
  }), [filtered, timestamps, width, wrap]);
  const offsets = useMemo(() => {
    const result = [0];
    for (const rowHeight of rowHeights) result.push(result[result.length - 1] + rowHeight);
    return result;
  }, [rowHeights]);
  const totalHeight = offsets[offsets.length - 1];
  const first = useMemo(() => {
    let low = 0; let high = filtered.length;
    while (low < high) { const middle = Math.floor((low + high) / 2); if (offsets[middle + 1] < scrollTop) low = middle + 1; else high = middle; }
    return Math.max(0, low - OVERSCAN);
  }, [filtered.length, offsets, scrollTop]);
  const last = useMemo(() => {
    let index = first;
    while (index < filtered.length && offsets[index] < scrollTop + height) index += 1;
    return Math.min(filtered.length, index + OVERSCAN);
  }, [filtered.length, first, height, offsets, scrollTop]);

  useEffect(() => {
    if (!follow || reducedMotion || !atLatest || !box.current) return;
    box.current.scrollTop = box.current.scrollHeight;
  }, [atLatest, filtered.length, follow, reducedMotion, totalHeight]);

  function onScroll() {
    const element = box.current;
    if (!element) return;
    setScrollTop(element.scrollTop);
    const latest = element.scrollHeight - element.scrollTop - element.clientHeight < 8;
    setAtLatest(latest);
    if (latest) setUnseen(0);
  }
  function jumpToLatest() {
    box.current?.scrollTo({ top: box.current.scrollHeight, behavior: reducedMotion ? "auto" : "smooth" });
    setAtLatest(true);
    setUnseen(0);
  }
  async function download() {
    const token = getToken();
    const root = (basePath() === "/" ? "" : basePath()) + "/api/v1";
    const response = await fetch(`${root}/containers/${encodeURIComponent(containerID)}/logs?${query({ follow: false, tail, stream, timestamps })}`, { headers: { Accept: "text/plain", ...(token ? { Authorization: `Bearer ${token}` } : {}) } });
    if (!response.ok) return;
    const href = URL.createObjectURL(await response.blob());
    const link = document.createElement("a"); link.href = href; link.download = "container.log"; link.click(); URL.revokeObjectURL(href);
  }

  return <div>
    <div className="mb-2 flex flex-wrap items-center gap-2">
      <Button variant={follow ? "primary" : "default"} onClick={() => setFollow((value) => !value)}>{follow ? "Following" : "Paused"}</Button>
      <label className="text-[12px] text-muted">Tail <select value={tail} onChange={(event) => setTail(event.target.value as Tail)} className="ml-1 rounded border border-line bg-panel px-1 py-1 text-text"><option value="100">100</option><option value="500">500</option><option value="2000">2,000</option><option value="all">all</option></select></label>
      <div aria-label="Log stream" className="flex overflow-hidden rounded border border-line">{(["both", "stdout", "stderr"] as const).map((value) => <button key={value} aria-pressed={stream === value} onClick={() => setStream(value)} className={`px-2 py-1 text-[12px] capitalize ${stream === value ? "bg-hull text-white" : "bg-panel text-text"}`}>{value}</button>)}</div>
      <input type="search" value={filter} onChange={(event) => setFilter(event.target.value)} placeholder="Filter lines" aria-label="Filter log lines" className="min-w-[180px] rounded border border-line bg-panel px-2 py-1 text-[12px]" />
      <span aria-live="polite" className="font-mono text-[12px] text-muted">{filtered.length.toLocaleString()} lines</span>
      <span className="flex-1" />
      <label className="flex items-center gap-1 text-[12px]"><input type="checkbox" checked={timestamps} onChange={(event) => setTimestamps(event.target.checked)} /> timestamps</label>
      <label className="flex items-center gap-1 text-[12px]"><input type="checkbox" checked={wrap} onChange={(event) => setWrap(event.target.checked)} /> wrap</label>
      <Button onClick={() => void download()}>Download</Button><Button onClick={() => { setLines([]); setUnseen(0); }}>Clear</Button>
    </div>
    <div className="relative"><div ref={box} tabIndex={0} role="log" aria-label="Container logs" onScroll={onScroll} className="h-[420px] overflow-auto rounded bg-ink py-2 font-mono text-[12.5px] leading-5 text-[#CBDCE3]">
      <div style={{ height: totalHeight, position: "relative" }}>{filtered.slice(first, last).map((line, index) => {
        const position = first + index; const warning = line.stream === "vessel" || line.line.startsWith("[vessel] dropped ");
        return <div key={`${position}-${line.ts ?? ""}-${line.line}`} style={{ position: "absolute", top: offsets[position], height: rowHeights[position], left: 0, right: 0 }} className={`px-3 ${wrap ? "whitespace-pre-wrap break-words" : "truncate whitespace-pre"} ${warning ? "bg-pause/15 text-[#E8C878] shadow-[inset_3px_0_0_theme(colors.pause)]" : line.stream === "stderr" ? "bg-fail/10 text-[#F3B7AE] shadow-[inset_3px_0_0_theme(colors.fail)]" : ""}`}>{timestamps && <span className="mr-2 text-[#5F7C88]">{timestamp(line.ts)}</span>}{highlight(line.line, filter.trim())}</div>;
      })}</div>
    </div>{(!atLatest || reducedMotion) && unseen > 0 && <button onClick={jumpToLatest} className="absolute bottom-3 left-1/2 -translate-x-1/2 rounded border border-line bg-panel px-3 py-1 text-[12px] shadow">Jump to latest ({unseen.toLocaleString()} new)</button>}</div>
    <p className="mt-2 text-[12px] text-muted">Stream: {status === "open" ? "live" : status}</p>
  </div>;
}
