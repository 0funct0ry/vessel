import { useEffect, useRef, useState } from "react";
import { Check, ChevronDown } from "lucide-react";

export interface SelectOption {
  value: string;
  label: string;
}

/**
 * A Tailwind-styled dropdown matching this app's input/panel styling — used
 * instead of a native <select>, whose popup can't be themed and looks
 * inconsistent with the rest of a modal.
 */
export function Select({ value, onChange, options, disabled, className = "" }: { value: string; onChange: (value: string) => void; options: SelectOption[]; disabled?: boolean; className?: string }) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const current = options.find((o) => o.value === value);

  useEffect(() => {
    if (!open) return;
    const onDocClick = (e: MouseEvent) => { if (root.current && !root.current.contains(e.target as Node)) setOpen(false); };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", onDocClick);
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("mousedown", onDocClick); document.removeEventListener("keydown", onKey); };
  }, [open]);

  return <div ref={root} className={`relative ${className}`}>
    <button
      type="button"
      disabled={disabled}
      aria-haspopup="listbox"
      aria-expanded={open}
      onClick={() => setOpen((v) => !v)}
      className="mt-1 flex w-full items-center justify-between rounded border border-line bg-panel px-2 py-1.5 text-left text-[13px] disabled:cursor-not-allowed disabled:opacity-45"
    >
      <span>{current?.label ?? value}</span>
      <ChevronDown size={14} className={`text-muted transition-transform ${open ? "rotate-180" : ""}`} />
    </button>
    {open && <ul role="listbox" className="absolute z-10 mt-1 w-full overflow-hidden rounded border border-line bg-panel py-1 shadow-[0_8px_24px_rgba(11,31,42,.18)]">
      {options.map((option) => <li key={option.value}>
        <button
          type="button"
          role="option"
          aria-selected={option.value === value}
          onClick={() => { onChange(option.value); setOpen(false); }}
          className={`flex w-full items-center justify-between px-2 py-1.5 text-left text-[13px] hover:bg-paper ${option.value === value ? "font-medium" : ""}`}
        >
          {option.label}
          {option.value === value && <Check size={14} className="text-hull" />}
        </button>
      </li>)}
    </ul>}
  </div>;
}
