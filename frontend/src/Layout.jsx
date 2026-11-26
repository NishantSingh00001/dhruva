import React from "react";
import {
  LuLayoutGrid,
  LuMapPin,
  LuGauge,
  LuBellRing,
  LuSettings,
  LuSearch,
} from "react-icons/lu";

const RAIL_ITEMS = [
  { key: "dashboard", icon: LuLayoutGrid },
  { key: "geofencing", icon: LuMapPin },
  { key: "driving", icon: LuGauge },
  { key: "alerts", icon: LuBellRing },
];

const TABS = [
  { key: "dashboard", label: "Dashboard" },
  { key: "geofencing", label: "Geofencing" },
  { key: "driving", label: "Driver Behavior" },
  { key: "alerts", label: "Alerts" },
];

function DhruvaMark() {
  return (
    <div className="brand-mark">
      <svg width="34" height="34" viewBox="0 0 34 34" fill="none">
        <circle cx="17" cy="17" r="15" stroke="var(--green)" strokeWidth="1.6" strokeDasharray="4 4" />
        <circle cx="17" cy="17" r="9" stroke="var(--orange)" strokeWidth="1.6" />
        <circle cx="17" cy="17" r="3" fill="var(--green)" />
      </svg>
      <span className="brand-word">DHRUVA</span>
    </div>
  );
}

export default function Layout({ tab, setTab, children }) {
  return (
    <div className="app-shell">
      <aside className="rail">
        <div className="logo">DG</div>
        {RAIL_ITEMS.map(({ key, icon: Icon }) => (
          <button
            key={key}
            className={`rail-btn ${tab === key ? "active" : ""}`}
            onClick={() => setTab(key)}
            title={key}
          >
            <Icon />
          </button>
        ))}
        <div className="rail-spacer" />
        <button className="rail-btn">
          <LuSettings />
        </button>
      </aside>

      <main className="main">
        <div className="topbar">
          <nav className="pill-nav">
            {TABS.map((t) => (
              <button
                key={t.key}
                className={tab === t.key ? "active" : ""}
                onClick={() => setTab(t.key)}
              >
                {t.label}
              </button>
            ))}
          </nav>

          <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
            <button className="rail-btn" style={{ width: 44, height: 44 }}>
              <LuSearch />
            </button>
            <DhruvaMark />
          </div>
        </div>

        {children}
      </main>
    </div>
  );
}
