import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { useAuth } from "./auth/AuthProvider";
import { LoginPage } from "./auth/LoginPage";
import { AppShell } from "./components/shell/AppShell";
import {
  ContainerDetailPage,
  ContainersPage,
  DashboardPage,
  EventsPage,
  ImageDetailPage,
  ImagesPage,
  NetworkDetailPage,
  NetworksPage,
  NotFoundPage,
  SettingsPage,
  VolumeDetailPage,
  VolumesPage,
  WebhookDetailPage,
  WebhooksPage,
} from "./routes/pages";

function RequireAuth({ children }: { children: JSX.Element }) {
  const { loading, authenticated } = useAuth();
  const location = useLocation();

  if (loading) return null;
  if (!authenticated) return <Navigate to="/login" state={{ from: location }} replace />;
  return children;
}

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <RequireAuth>
            <AppShell />
          </RequireAuth>
        }
      >
        <Route path="/" element={<DashboardPage />} />
        <Route path="/containers" element={<ContainersPage />} />
        <Route path="/containers/:id" element={<ContainerDetailPage />} />
        <Route path="/images" element={<ImagesPage />} />
        <Route path="/images/:id" element={<ImageDetailPage />} />
        <Route path="/volumes" element={<VolumesPage />} />
        <Route path="/volumes/:name" element={<VolumeDetailPage />} />
        <Route path="/networks" element={<NetworksPage />} />
        <Route path="/networks/:id" element={<NetworkDetailPage />} />
        <Route path="/events" element={<EventsPage />} />
        <Route path="/webhooks" element={<WebhooksPage />} />
        <Route path="/webhooks/:id" element={<WebhookDetailPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}
