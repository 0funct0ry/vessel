import { getToken } from "./api";
import { basePath } from "./basePath";

function apiRoot(): string {
  const base = basePath();
  return (base === "/" ? "" : base) + "/api/v1";
}

/**
 * POST /images/pull streams SSE in response to a POST body, so it cannot use
 * EventSource (GET-only). This does the framing by hand over a fetch()
 * ReadableStream, which is also what makes cancellation via AbortController
 * possible.
 */
export async function streamSSE(path: string, body: BodyInit, headers: HeadersInit, onEvent: (name: string, data: unknown) => void, signal: AbortSignal): Promise<void> {
  const token = getToken();
  const response = await fetch(`${apiRoot()}${path}`, {
    method: "POST",
    signal,
    headers: {
      Accept: "text/event-stream",
      ...headers,
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body,
  });

  if (!response.ok || !response.body) {
    let message = response.statusText;
    try {
      const parsed = await response.json();
      if (parsed?.error?.message) message = parsed.error.message;
    } catch {
      // non-JSON error body; keep statusText
    }
    throw new Error(message || "stream failed");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });

    let at = buffer.indexOf("\n\n");
    while (at !== -1) {
      const frame = buffer.slice(0, at);
      buffer = buffer.slice(at + 2);
      let name = "message";
      let data = "";
      for (const line of frame.split("\n")) {
        if (line.startsWith("event:")) name = line.slice(6).trim();
        else if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (data) {
        try {
          onEvent(name, JSON.parse(data));
        } catch {
          // malformed frame; skip
        }
      }
      at = buffer.indexOf("\n\n");
    }
  }
}

export async function pullImage(reference: string, onEvent: (name: string, data: unknown) => void, signal: AbortSignal): Promise<void> {
  return streamSSE("/images/pull", JSON.stringify({ reference }), { "Content-Type": "application/json" }, onEvent, signal);
}
