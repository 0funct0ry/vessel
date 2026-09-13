// Client-only appearance preferences (SPEC's M17.7 "Appearance" panel).
// These are per-browser conveniences, not server state, so they live in
// localStorage rather than going through Store/RBAC.

export type Theme = "light" | "dark" | "system";
export type ContainerFilter = "all" | "running";
export type Density = "comfortable" | "compact";

interface Preferences {
  theme: Theme;
  containerFilter: ContainerFilter;
  density: Density;
}

const KEY = "vessel.preferences";

const DEFAULTS: Preferences = { theme: "system", containerFilter: "all", density: "comfortable" };

function readAll(): Preferences {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return DEFAULTS;
    return { ...DEFAULTS, ...JSON.parse(raw) };
  } catch {
    return DEFAULTS;
  }
}

function writeAll(prefs: Preferences) {
  try {
    localStorage.setItem(KEY, JSON.stringify(prefs));
  } catch {
    // private mode or storage disabled; preferences simply don't persist
  }
}

export function getPreferences(): Preferences {
  return readAll();
}

export function setPreference<K extends keyof Preferences>(key: K, value: Preferences[K]): Preferences {
  const next = { ...readAll(), [key]: value };
  writeAll(next);
  if (key === "theme") applyTheme(next.theme);
  if (key === "density") applyDensity(next.density);
  return next;
}

export function applyTheme(theme: Theme) {
  const root = document.documentElement;
  if (theme === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);
}

export function applyDensity(density: Density) {
  const root = document.documentElement;
  if (density === "compact") root.setAttribute("data-density", "compact");
  else root.removeAttribute("data-density");
}

/** Call once at startup so the stored theme/density apply before first paint. */
export function applyStoredPreferences() {
  const prefs = readAll();
  applyTheme(prefs.theme);
  applyDensity(prefs.density);
}
