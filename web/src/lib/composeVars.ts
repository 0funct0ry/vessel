/**
 * Client-side mirror of internal/compose/parse.go's ${VAR} detection, used
 * only to drive the stack editor's environment-variables panel. The server
 * remains the source of truth for what a saved file actually means —
 * ParseCompose runs again on create/update and its warnings are what's shown
 * once a stack is saved. This is a UI convenience so the panel updates as the
 * user types, before anything is sent to the API.
 */

const VARIABLE_RE = /\$(\$|\{[^{}]*\}|[A-Za-z_][A-Za-z0-9_]*)/g;
const OPERATORS = [":-", ":?", ":+", "-", "?", "+"] as const;

export type VarKind = "required" | "optional" | "error";

export interface DetectedVar {
  name: string;
  kind: VarKind;
  /** The ${VAR:-default} default, when kind is "optional". */
  fallback?: string;
  /** The ${VAR:?message} message, when kind is "error". */
  message?: string;
}

function splitVariable(body: string): { name: string; operator?: string; argument: string } {
  for (const operator of OPERATORS) {
    const at = body.indexOf(operator);
    if (at > 0) return { name: body.slice(0, at), operator, argument: body.slice(at + operator.length) };
  }
  return { name: body, argument: "" };
}

/** Detects every ${VAR} / ${VAR:-default} / ${VAR:?msg} / $NAME reference, in
 * first-occurrence order, deduped by name (first form seen wins). */
export function detectVariables(composeYAML: string): DetectedVar[] {
  const seen = new Map<string, DetectedVar>();
  for (const match of composeYAML.matchAll(VARIABLE_RE)) {
    const body = match[1];
    if (body === "$") continue;
    const braced = body.startsWith("{");
    const { name, operator, argument } = splitVariable(braced ? body.slice(1, -1) : body);
    if (!name || seen.has(name)) continue;
    if (operator === ":-" || operator === "-") seen.set(name, { name, kind: "optional", fallback: argument });
    else if (operator === ":?" || operator === "?") seen.set(name, { name, kind: "error", message: argument || "is required" });
    else seen.set(name, { name, kind: "required" });
  }
  return [...seen.values()];
}

/** Parses a stored .env blob into a plain key/value map. Mirrors
 * compose.ParseEnvFile: blank lines and #-comments are skipped, `export ` is
 * stripped, quoted values are unwrapped. */
export function parseEnvContent(content: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (let line of content.split("\n")) {
    line = line.trim();
    if (!line || line.startsWith("#")) continue;
    line = line.replace(/^export\s+/, "");
    const at = line.indexOf("=");
    if (at <= 0) continue;
    const key = line.slice(0, at).trim();
    let value = line.slice(at + 1).trim();
    if (value.length >= 2 && (value[0] === '"' || value[0] === "'") && value[value.length - 1] === value[0]) {
      value = value.slice(1, -1);
    }
    out[key] = value;
  }
  return out;
}

/** Serializes an ordered set of key/value pairs back into a .env blob. */
export function serializeEnvContent(entries: { key: string; value: string }[]): string {
  return entries
    .filter((entry) => entry.key.trim() !== "")
    .map((entry) => `${entry.key}=${entry.value}`)
    .join("\n");
}
