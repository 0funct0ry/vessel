import type { ReactNode } from "react";

/**
 * SPEC §9.2: empty and error states name the fix, never a bare "no data".
 */
export function EmptyState({ title, action }: { title: string; action: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-1 py-[34px] text-center text-[13px] text-muted">
      <b className="text-[14px] text-text">{title}</b>
      <p>{action}</p>
    </div>
  );
}
