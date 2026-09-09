import { cloneElement, isValidElement, type ReactElement } from "react";
import { useAuth } from "./AuthProvider";

/**
 * Capability -> minimum role, mirrored by hand from internal/auth/rbac.go's
 * Policies table (Capability -> Role). Only used for the disabled-control
 * tooltip copy; the actual allow/deny decision always comes from the
 * capability map returned by GET /auth/me.
 */
const REQUIRED_ROLE: Record<string, "operator" | "admin"> = {
  "containers.start": "operator",
  "containers.stop": "operator",
  "containers.restart": "operator",
  "containers.pause": "operator",
  "containers.unpause": "operator",
  "containers.kill": "operator",
  "containers.rename": "operator",
	"containers.remove": "operator",
	"containers.create": "operator",
  "containers.exec": "operator",
  "images.pull": "operator",
  "images.tag": "operator",
  "images.remove": "operator",
  "volumes.create": "operator",
  "volumes.remove": "operator",
  "networks.create": "operator",
  "networks.remove": "operator",
  "networks.connect": "operator",
  "networks.disconnect": "operator",
  "prune.run": "admin",
  "webhooks.read": "admin",
  "webhooks.write": "admin",
  "users.read": "admin",
  "users.write": "admin",
};

interface CanProps {
  do: string;
  children: ReactElement;
}

/**
 * Renders its child normally when the current user has the named
 * capability; otherwise clones it disabled with a tooltip naming the
 * required role (SPEC §9: never hide controls silently).
 */
export function Can({ do: capability, children }: CanProps) {
  const { capabilities } = useAuth();
  const allowed = capabilities[capability] ?? false;

  if (allowed || !isValidElement(children)) return children;

  const requiredRole = REQUIRED_ROLE[capability] ?? "admin";
  return cloneElement(children as ReactElement<Record<string, unknown>>, {
    disabled: true,
    "aria-disabled": true,
    title: `Requires the ${requiredRole} role`,
  });
}
