import type { ReactNode } from "react";

type Tone = "default" | "use" | "warn";

const TONE_CLASSES: Record<Tone, string> = {
  default: "border-line text-muted",
  use: "border-run text-run",
  warn: "border-pause text-pause",
};

export function Tag({ tone = "default", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={`inline-block rounded-sm border px-1.5 py-0.5 font-mono text-[11.5px] ${TONE_CLASSES[tone]}`}>
      {children}
    </span>
  );
}
