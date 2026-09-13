import { useState } from "react";
import { Select, type SelectOption } from "../../components/ui/Select";
import { getPreferences, setPreference, type ContainerFilter, type Density, type Theme } from "../../lib/preferences";

const THEME_OPTIONS: SelectOption[] = [{ value: "system", label: "Match system" }, { value: "light", label: "Light" }, { value: "dark", label: "Dark" }];
const FILTER_OPTIONS: SelectOption[] = [{ value: "all", label: "All containers" }, { value: "running", label: "Running only" }];
const DENSITY_OPTIONS: SelectOption[] = [{ value: "comfortable", label: "Comfortable" }, { value: "compact", label: "Compact" }];

function Field({ label, hint, children }: { label: string; hint: string; children: React.ReactNode }) {
  return <div className="mt-4"><label className="block text-[13px]">{label}</label><p className="m-0 mb-1 text-[12px] text-muted">{hint}</p>{children}</div>;
}

export function AppearanceTab() {
  const [prefs, setPrefs] = useState(getPreferences());
  return <div className="max-w-md rounded border border-line bg-panel p-4">
    <p className="m-0 text-[12px] text-muted">These preferences are saved in this browser only — they don't sync across devices and aren't visible to other users.</p>
    <Field label="Theme" hint="Follow the OS, or pin to light/dark.">
      <Select value={prefs.theme} onChange={(v) => setPrefs(setPreference("theme", v as Theme))} options={THEME_OPTIONS} />
    </Field>
    <Field label="Default container filter" hint="What the containers list shows when you first open it.">
      <Select value={prefs.containerFilter} onChange={(v) => setPrefs(setPreference("containerFilter", v as ContainerFilter))} options={FILTER_OPTIONS} />
    </Field>
    <Field label="Table density" hint="Row height across tables.">
      <Select value={prefs.density} onChange={(v) => setPrefs(setPreference("density", v as Density))} options={DENSITY_OPTIONS} />
    </Field>
  </div>;
}
