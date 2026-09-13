interface SwitchProps {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
  label?: string;
  "aria-label"?: string;
}

export function Switch({ checked, onChange, disabled, label, "aria-label": ariaLabel }: SwitchProps) {
  const control = (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label ? undefined : ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-block h-5 w-9 shrink-0 appearance-none rounded-full border-0 p-0 outline-none transition-colors ${checked ? "bg-hull" : "bg-line"} disabled:cursor-not-allowed`}
    >
      <span className={`absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-panel shadow transition-transform ${checked ? "translate-x-4" : "translate-x-0"}`} />
    </button>
  );

  if (!label) return control;

  return (
    <label className={`mt-3 flex items-center justify-between rounded border border-line px-2.5 py-1.5 text-[13px] ${disabled ? "opacity-45" : ""}`}>
      <span>{label}</span>
      {control}
    </label>
  );
}
