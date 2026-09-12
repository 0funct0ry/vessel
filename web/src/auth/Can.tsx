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
	"containers.commit": "operator",
  "containers.exec": "operator",
  "containers.files.list": "operator",
  "containers.files.upload": "operator",
  "containers.files.mkdir": "operator",
  "containers.files.download": "operator",
  "containers.files.delete": "operator",
  "containers.files.rename": "operator",
  "containers.files.view": "operator",
  "containers.files.edit": "operator",
  "images.pull": "operator",
  "images.tag": "operator",
  "images.remove": "operator",
  "volumes.create": "operator",
  "volumes.remove": "operator",
  "volumes.clone": "operator",
  "volumes.files.list": "operator",
  "volumes.files.upload": "operator",
  "volumes.files.mkdir": "operator",
  "volumes.files.download": "operator",
  "volumes.files.delete": "operator",
  "volumes.files.rename": "operator",
  "volumes.files.view": "operator",
  "volumes.files.edit": "operator",
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

// Capabilities disabled by --allow-exec=false regardless of role, per M15.4's
// asymmetry: listing and folder creation go through exec, upload/download do not.
const EXEC_GATED = new Set(["containers.exec", "containers.files.list", "containers.files.mkdir", "containers.files.delete", "containers.files.rename"]);

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
  const { capabilities, user } = useAuth();
  const allowed = capabilities[capability] ?? false;

  if (allowed || !isValidElement(children)) return children;

  const requiredRole = REQUIRED_ROLE[capability] ?? "admin";
  const roleSatisfied = user != null && ROLE_RANK[user.role] >= ROLE_RANK[requiredRole];
  const title = EXEC_GATED.has(capability) && roleSatisfied
    ? "Unavailable: the server was started with --allow-exec=false"
    : `Requires the ${requiredRole} role`;
  return cloneElement(children as ReactElement<Record<string, unknown>>, {
    disabled: true,
    "aria-disabled": true,
    title,
  });
}

const ROLE_RANK: Record<string, number> = { viewer: 1, operator: 2, admin: 3 };
