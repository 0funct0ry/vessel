export interface TabItem {
  key: string;
  label: string;
  invalid?: boolean;
}

export function Tabs({ tabs, active, onChange }: { tabs: TabItem[]; active: string; onChange: (key: string) => void }) {
  return <div role="tablist" className="mt-4 flex gap-1 border-b border-line">
    {tabs.map((tab) => <button
      key={tab.key}
      type="button"
      role="tab"
      aria-selected={tab.key === active}
      onClick={() => onChange(tab.key)}
      className={`-mb-px rounded-t border border-b-0 px-3 py-1.5 text-[13px] ${tab.key === active ? "border-line bg-panel text-text" : "border-transparent text-muted hover:text-text"} ${tab.invalid ? "text-fail" : ""}`}
    >
      {tab.label}
    </button>)}
  </div>;
}
