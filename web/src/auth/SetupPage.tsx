import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "./AuthProvider";
import { ApiRequestError } from "../types/api";

const MIN_PASSWORD = 8;

export function SetupPage() {
  const { bootstrap } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (password.length < MIN_PASSWORD) {
      setError(`Password must be at least ${MIN_PASSWORD} characters.`);
      return;
    }
    if (password !== confirmPassword) {
      setError("Passwords do not match.");
      return;
    }
    setSubmitting(true);
    try {
      await bootstrap(username, password);
      navigate("/", { replace: true });
    } catch (err) {
      if (err instanceof ApiRequestError && err.code === "already_bootstrapped") {
        setError("An admin account already exists. Reload to sign in.");
      } else if (err instanceof ApiRequestError) {
        setError(err.message);
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
        <h1 className="mb-1 font-sans text-lg font-semibold text-ink">Welcome to Vessel</h1>
        <p className="mb-4 text-[13px] text-muted">Create the first admin account to get started.</p>

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

        <label className="mb-3 block text-[13px]">
          <span className="mb-1 block text-muted">Password</span>
          <input
            type="password"
            className="w-full rounded border border-line px-2 py-1 text-[13px]"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
          />
        </label>

        <label className="mb-4 block text-[13px]">
          <span className="mb-1 block text-muted">Confirm password</span>
          <input
            type="password"
            className="w-full rounded border border-line px-2 py-1 text-[13px]"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            autoComplete="new-password"
          />
        </label>

        {error && (
          <p role="alert" className="mb-3 text-[13px] text-fail">
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting || !username || !password || !confirmPassword}
          className="w-full rounded bg-hull py-1.5 text-[13px] font-medium text-white hover:bg-steel disabled:opacity-45"
        >
          {submitting ? "Creating account…" : "Create admin account"}
        </button>
      </form>
    </div>
  );
}
