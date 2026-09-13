import { EmptyState } from "../components/ui/EmptyState";
import { DashboardPage, EventsPage } from "./dashboard";
export { NetworkDetailPage, NetworksPage, VolumeDetailPage, VolumesPage } from "./resources";
export { ContainerDetailPage, ContainersPage } from "./containers";
export { ImageDetailPage, ImagesPage } from "./images";
export { StackDetailPage, StacksPage } from "./stacks";
export { SettingsPage } from "./settings/SettingsPage";
export { WebhookDetailPage, WebhooksPage } from "./webhooks";

// Placeholder pages for every SPEC §9.1 route not yet built.
// M13 (containers), M14 (logs), M15 (images), M16 (volumes/networks),
// M17 (dashboard/events), M18 (webhooks) are all real now. Each stub still
// follows the "empty states name the fix" rule.

export { DashboardPage, EventsPage };

export function NotFoundPage() {
  return <EmptyState title="Nothing here" action="Check the URL, or go back to the dashboard." />;
}
