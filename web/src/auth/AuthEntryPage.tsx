import { useEffect, useState } from "react";
import { api } from "../lib/api";
import type { BootstrapStatus } from "../types/api";
import { LoginPage } from "./LoginPage";
import { SetupPage } from "./SetupPage";

// Decides, once per load, whether to show the first-run setup screen (no
// users exist yet) or the normal login form. `/api/v1/auth/bootstrap` is
// reachable without a token specifically so this check can run before the
// user has any credentials to offer.
export function AuthEntryPage() {
  const [setupAvailable, setSetupAvailable] = useState<boolean | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .get<BootstrapStatus>("/auth/bootstrap")
      .then((res) => {
        if (!cancelled) setSetupAvailable(res.available);
      })
      .catch(() => {
        if (!cancelled) setSetupAvailable(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (setupAvailable === null) return null;
  return setupAvailable ? <SetupPage /> : <LoginPage />;
}
