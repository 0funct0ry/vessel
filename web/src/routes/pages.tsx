import { EmptyState } from "../components/ui/EmptyState";

// Placeholder pages for every SPEC §9.1 route. Real content lands in
// M13 (containers), M14 (logs), M15 (images), M16 (volumes/networks),
// M17 (dashboard/events), M18 (webhooks). Each stub still follows the
// "empty states name the fix" rule.

export function DashboardPage() {
  return <EmptyState title="Dashboard is not built yet" action="This lands in M17." />;
}

export function ContainersPage() {
  return <EmptyState title="No containers to show yet" action="The container list lands in M13." />;
}

export function ContainerDetailPage() {
  return <EmptyState title="Container detail is not built yet" action="Lands in M13." />;
}

export function ImagesPage() {
  return <EmptyState title="No images to show yet" action="The image list lands in M15." />;
}

export function ImageDetailPage() {
  return <EmptyState title="Image detail is not built yet" action="Lands in M15." />;
}

export function VolumesPage() {
  return <EmptyState title="No volumes to show yet" action="The volume list lands in M16." />;
}

export function VolumeDetailPage() {
  return <EmptyState title="Volume detail is not built yet" action="Lands in M16." />;
}

export function NetworksPage() {
  return <EmptyState title="No networks to show yet" action="The network list lands in M16." />;
}

export function NetworkDetailPage() {
  return <EmptyState title="Network detail is not built yet" action="Lands in M16." />;
}

export function EventsPage() {
  return <EmptyState title="No events yet" action="The live event feed lands in M17." />;
}

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
