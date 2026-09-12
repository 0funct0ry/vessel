export type FlagState = "running" | "paused" | "restarting" | "exited" | "stopped" | "created";

const LABEL: Record<FlagState, string> = {
  running: "Running",
  paused: "Paused",
  restarting: "Restarting",
  exited: "Exited",
  stopped: "Stopped",
  created: "Created",
};

/**
 * State is a square, not a pill (SPEC §9.2): shape + label together, never
 * color alone, so it reads under prefers-reduced-motion and for anyone who
 * can't distinguish the colors.
 */
export function Flag({ state, label }: { state: FlagState; label?: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-[13px]">
      <i
        aria-hidden
        className={{
          running: "inline-block h-2.5 w-2.5 bg-run",
          paused:
            "inline-block h-2.5 w-2.5 border border-pause bg-gradient-to-br from-pause from-50% to-transparent to-50%",
          restarting: "inline-block h-2.5 w-2.5 animate-pulse bg-pause",
          exited: "inline-block h-2.5 w-2.5 bg-fail",
          stopped: "inline-block h-2.5 w-2.5 border border-stop bg-transparent",
          created: "inline-block h-2.5 w-2.5 border border-link bg-transparent",
        }[state]}
      />
      {label ?? LABEL[state]}
    </span>
  );
}
