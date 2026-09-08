import { NavLink } from "react-router-dom";
import { useAuth } from "../../auth/AuthProvider";

interface NavItem {
  to: string;
  label: string;
}

const MAIN: NavItem[] = [
  { to: "/", label: "Dashboard" },
  { to: "/containers", label: "Containers" },
  { to: "/images", label: "Images" },
  { to: "/volumes", label: "Volumes" },
  { to: "/networks", label: "Networks" },
];

const ACTIVITY: NavItem[] = [
  { to: "/events", label: "Events" },
  { to: "/webhooks", label: "Webhooks" },
];

const HOST: NavItem[] = [{ to: "/settings", label: "Settings" }];

function NavGroup({ label, items }: { label?: string; items: NavItem[] }) {
  return (
    <>
      {label && (
        <div className="px-2.5 pb-[5px] pt-3 text-[11px] tracking-wide text-[#5F7C88]">{label}</div>
      )}
      {items.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.to === "/"}
          className={({ isActive }) =>
            `flex items-center gap-2.5 rounded px-2.5 py-1.5 text-[13.5px] text-railtext no-underline ${
              isActive ? "bg-steel text-white shadow-[inset_2px_0_0_theme(colors.pause)]" : "hover:bg-white/[.06] hover:text-white"
            }`
          }
        >
          {item.label}
        </NavLink>
      ))}
    </>
  );
}

export function Rail() {
  const { user, logout } = useAuth();

  return (
    <nav aria-label="Primary" className="hidden h-full min-h-0 flex-col bg-ink text-railtext md:flex">
      <div className="border-b border-white/[.08] px-[18px] pb-3.5 pt-[18px]">
        <h1 className="m-0 font-sans text-[19px] font-semibold tracking-[-0.01em] text-white">Vessel</h1>
        <p className="mt-1 font-mono text-[11.5px] text-[#7F9BA6]">host</p>
      </div>

      <div className="flex-1 overflow-auto px-2 py-2.5">
        <NavGroup items={MAIN} />
        <NavGroup label="Activity" items={ACTIVITY} />
        <NavGroup label="Host" items={HOST} />
      </div>

      <div className="border-t border-white/[.08] px-[18px] py-3 text-[11.5px] text-[#6C8894]">
        <span className="mb-0.5 block text-[#C8DAE1]">{user?.username ?? "admin"} · {user?.role ?? "admin"}</span>
        <span>
          v0.1.0 ·{" "}
          <button onClick={() => void logout()} className="text-[#6C8894] underline hover:text-white">
            sign out
          </button>
        </span>
      </div>
    </nav>
  );
}
