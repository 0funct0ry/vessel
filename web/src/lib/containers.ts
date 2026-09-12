import type { Container, ContainerPort } from "../types/api";

export function containerQuery(input: { all: boolean; q: string; status: string; sort: string }): string {
  const p = new URLSearchParams();
  if (input.all) p.set("all", "true");
  if (input.q.trim()) p.set("q", input.q.trim());
  if (input.status) p.set("status", input.status);
  if (input.sort) p.set("sort", input.sort);
  const query = p.toString();
  return `/containers${query ? `?${query}` : ""}`;
}
export function uptime(created: number): string { const seconds = Math.max(0, Math.floor(Date.now() / 1000 - created)); if (seconds < 60) return `${seconds}s`; if (seconds < 3600) return `${Math.floor(seconds / 60)}m`; if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`; return `${Math.floor(seconds / 86400)}d`; }
export function bytes(value: number): string { if (!value) return "0 B"; const units = ["B", "KiB", "MiB", "GiB", "TiB"]; const i = Math.min(units.length - 1, Math.floor(Math.log(value) / Math.log(1024))); return `${(value / 1024 ** i).toFixed(i ? 1 : 0)} ${units[i]}`; }
export function flagState(state: string): "running" | "paused" | "restarting" | "exited" | "stopped" | "created" { if (state === "running" || state === "paused" || state === "restarting" || state === "created") return state; return state === "exited" || state === "dead" ? "exited" : "stopped"; }
export function httpPort(port: ContainerPort): string | null { if (port.type !== "tcp" || !port.public_port || ![80, 443, 3000, 3001, 4000, 5000, 5173, 8000, 8080, 8081, 8088, 8443, 8888, 9000].includes(port.private_port)) return null; return `http://${window.location.hostname}:${port.public_port}`; }
export function canStop(container: Container): boolean { return container.state === "running" || container.state === "paused" || container.state === "restarting"; }
