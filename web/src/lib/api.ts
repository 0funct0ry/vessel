import { basePath } from "./basePath";
import { ApiRequestError, type ApiError } from "../types/api";

const TOKEN_KEY = "vessel_token";

export function getToken(): string | null {
  return sessionStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  sessionStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  sessionStorage.removeItem(TOKEN_KEY);
}

type SocketListener = (unreachable: boolean) => void;
const socketListeners = new Set<SocketListener>();

/** Subscribed to by SocketBanner; SPEC §5.2 says a 503 docker_unreachable is a banner, never a toast. */
export function onSocketStatus(fn: SocketListener): () => void {
  socketListeners.add(fn);
  return () => socketListeners.delete(fn);
}

function setSocketUnreachable(unreachable: boolean) {
  for (const fn of socketListeners) fn(unreachable);
}

function apiRoot(): string {
  const base = basePath();
  return (base === "/" ? "" : base) + "/api/v1";
}

export interface RequestOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers = new Headers(opts.headers);
  headers.set("Accept", "application/json");
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);

  let body: BodyInit | undefined;
  if (opts.body !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(opts.body);
  }

  const res = await fetch(apiRoot() + path, { ...opts, headers, body });

  if (res.status === 204) {
    setSocketUnreachable(false);
    return undefined as T;
  }

  if (!res.ok) {
    let err: ApiError = { code: "unknown", message: res.statusText };
    try {
      const parsed = await res.json();
      if (parsed?.error) err = parsed.error;
    } catch {
      // non-JSON error body; keep the fallback above
    }

    setSocketUnreachable(res.status === 503 && err.code === "docker_unreachable");

    if (res.status === 401 && err.code === "token_expired") {
      clearToken();
      const base = basePath();
      window.location.href = (base === "/" ? "" : base) + "/login";
    }

    throw new ApiRequestError(res.status, err);
  }

  setSocketUnreachable(false);
  return res.json() as Promise<T>;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: "POST", body }),
  patch: <T>(path: string, body?: unknown) => request<T>(path, { method: "PATCH", body }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};
