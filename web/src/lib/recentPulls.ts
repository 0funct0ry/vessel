const KEY = "vessel_recent_pulls";
const MAX = 10;

export function recentPulls(): string[] {
  try {
    const raw = sessionStorage.getItem(KEY);
    const parsed = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed.filter((v) => typeof v === "string") : [];
  } catch {
    return [];
  }
}

export function rememberPull(reference: string): void {
  try {
    const next = [reference, ...recentPulls().filter((r) => r !== reference)].slice(0, MAX);
    sessionStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    // sessionStorage unavailable (private mode, quota) — recent list is a convenience only
  }
}
