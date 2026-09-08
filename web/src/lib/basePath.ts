let cached: string | null = null;

/** The path Vessel is served under behind a reverse proxy (SPEC §3.1 --base-path). */
export function basePath(): string {
  if (cached !== null) return cached;
  const meta = document.querySelector('meta[name="vessel-base-path"]');
  const content = meta?.getAttribute("content") ?? "/";
  cached = content === "__VESSEL_BASE_PATH__" || content === "" ? "/" : content;
  return cached;
}
