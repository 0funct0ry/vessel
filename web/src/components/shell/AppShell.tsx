import { Outlet } from "react-router-dom";
import { Rail } from "./Rail";
import { TopBar } from "./TopBar";
import { SocketBanner } from "../ui/SocketBanner";

export function AppShell() {
  return (
    <div className="grid h-screen grid-cols-1 overflow-hidden md:grid-cols-[216px_1fr]">
      <Rail />
      <div className="flex min-h-0 min-w-0 flex-col">
        <TopBar />
        <SocketBanner />
        <main className="min-h-0 flex-1 overflow-y-auto p-[18px]">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
