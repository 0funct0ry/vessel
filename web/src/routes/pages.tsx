import { EmptyState } from "../components/ui/EmptyState";
import { DashboardPage, EventsPage } from "./dashboard";
export { NetworkDetailPage, NetworksPage, VolumeDetailPage, VolumesPage } from "./resources";
export { ContainersPage } from "./containers";
export { M156ContainerDetailPage as ContainerDetailPage } from "../components/containers/ContainerDetailM156";
export { ImageDetailPage, ImagesPage } from "./images";

// Placeholder pages for every SPEC §9.1 route. Real content lands in
// M13 (containers), M14 (logs), M15 (images), M16 (volumes/networks),
// M17 (dashboard/events), M18 (webhooks). Each stub still follows the
// "empty states name the fix" rule.

export { DashboardPage, EventsPage };

export function WebhooksPage() {
  return <EmptyState title="No webhooks configured yet" action="Webhook management lands in M18." />;
}

export function WebhookDetailPage() {
  return <EmptyState title="Webhook detail is not built yet" action="Lands in M18." />;
}

export function SettingsPage() {
  return <EmptyState title="Settings is not built yet" action="Users, tokens, and about info land in a later milestone." />;
}

export function NotFoundPage() {
  return <EmptyState title="Nothing here" action="Check the URL, or go back to the dashboard." />;
}
