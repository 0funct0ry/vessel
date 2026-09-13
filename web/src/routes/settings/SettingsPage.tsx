import { useState } from "react";
import { Tabs, type TabItem } from "../../components/ui/Tabs";
import { AboutTab } from "./About";
import { AppearanceTab } from "./Appearance";
import { TokensTab } from "./Tokens";
import { UsersTab } from "./Users";

const TABS: TabItem[] = [
  { key: "users", label: "Users" },
  { key: "tokens", label: "Tokens" },
  { key: "about", label: "About" },
  { key: "appearance", label: "Appearance" },
];

export function SettingsPage() {
  const [tab, setTab] = useState("users");
  return <section>
    <h1 className="m-0 text-lg">Settings</h1>
    <Tabs tabs={TABS} active={tab} onChange={setTab} />
    <div className="mt-4">
      {tab === "users" && <UsersTab />}
      {tab === "tokens" && <TokensTab />}
      {tab === "about" && <AboutTab />}
      {tab === "appearance" && <AppearanceTab />}
    </div>
  </section>;
}
