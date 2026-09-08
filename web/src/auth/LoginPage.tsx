import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "./AuthProvider";
import { ApiRequestError } from "../types/api";

export function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await login(username, password);
      navigate("/", { replace: true });
    } catch (err) {
      if (err instanceof ApiRequestError) {
        setError("Invalid username or password.");
      } else {
        setError("Could not reach the server.");
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-paper">
      <form onSubmit={onSubmit} className="w-80 rounded-md border border-line bg-panel p-6 shadow-sm">
        <h1 className="mb-1 font-sans text-lg font-semibold text-ink">Vessel</h1>
        <p className="mb-4 text-[13px] text-muted">Sign in to manage this host.</p>

        <label className="mb-3 block text-[13px]">
          <span className="mb-1 block text-muted">Username</span>
          <input
            autoFocus
            className="w-full rounded border border-line px-2 py-1 text-[13px]"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
          />
        </label>

        <label className="mb-4 block text-[13px]">
          <span className="mb-1 block text-muted">Password</span>
          <input
            type="password"
            className="w-full rounded border border-line px-2 py-1 text-[13px]"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
        </label>

        {error && (
          <p role="alert" className="mb-3 text-[13px] text-fail">
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting || !username || !password}
          className="w-full rounded bg-hull py-1.5 text-[13px] font-medium text-white hover:bg-steel disabled:opacity-45"
        >
          {submitting ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </div>
  );
}
