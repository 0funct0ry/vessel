import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, clearToken, setToken } from "../lib/api";
import type { Capabilities, MeResponse, Role, User } from "../types/api";

interface AuthState {
  loading: boolean;
  authenticated: boolean;
  user: (Partial<User> & { role: Role }) | null;
  capabilities: Capabilities;
  login: (username: string, password: string) => Promise<void>;
  bootstrap: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [loading, setLoading] = useState(true);
  const [me, setMe] = useState<MeResponse | null>(null);

  const refresh = useCallback(async () => {
    try {
      const res = await api.get<MeResponse>("/auth/me");
      setMe(res);
    } catch {
      setMe(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(
    async (username: string, password: string) => {
      const res = await api.post<{ token: string; user: User }>("/auth/login", { username, password });
      setToken(res.token);
      await refresh();
    },
    [refresh],
  );

  const bootstrap = useCallback(
    async (username: string, password: string) => {
      const res = await api.post<{ token: string; user: User }>("/auth/bootstrap", { username, password });
      setToken(res.token);
      await refresh();
    },
    [refresh],
  );

  const logout = useCallback(async () => {
    try {
      await api.post("/auth/logout");
    } finally {
      clearToken();
      await refresh();
    }
  }, [refresh]);

  // A successful /auth/me response means the shell can render: either auth
  // is fully off (me.auth === false, capabilities pre-computed as admin) or
  // auth is on and the bearer token was valid. Only a request failure (401
  // when auth is on and no/expired token) means the login page is needed —
  // authMiddleware rejects unauthenticated requests before handleMe runs.
  const value: AuthState = {
    loading,
    authenticated: me !== null,
    user: me?.user ?? null,
    capabilities: me?.capabilities ?? {},
    login,
    bootstrap,
    logout,
    refresh,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
