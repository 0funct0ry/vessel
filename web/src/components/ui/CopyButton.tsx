import { Copy } from "lucide-react";
import { useToast } from "./Toast";

export function CopyButton({ value, label, className = "" }: { value: string; label: string; className?: string }) {
  const { push } = useToast();
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      push(`${label} copied.`);
    } catch {
      push(`Could not copy ${label.toLowerCase()}. Select it and copy manually.`, "error");
    }
  };
  return (
    <button
      onClick={() => void copy()}
      aria-label={`Copy ${label}`}
      title={`Copy ${label}`}
      className={`rounded p-1 text-muted hover:bg-paper hover:text-text ${className}`}
    >
      <Copy size={13} />
    </button>
  );
}
