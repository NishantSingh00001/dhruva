import React, { useState, useEffect } from "react";
import "./theme.css";
import Layout from "./Layout.jsx";
import Dashboard from "./pages/Dashboard.jsx";
import Geofencing from "./pages/Geofencing.jsx";
import DriverBehavior from "./pages/DriverBehavior.jsx";
import Alerts from "./pages/Alerts.jsx";

const PAGES = {
  dashboard: Dashboard,
  geofencing: Geofencing,
  driving: DriverBehavior,
  alerts: Alerts,
};

function initialTab() {
  const q = new URLSearchParams(window.location.search).get("tab");
  return PAGES[q] ? q : "dashboard";
}

export default function App() {
  const [tab, setTab] = useState(initialTab);
  const Page = PAGES[tab] || Dashboard;

  useEffect(() => {
    const url = new URL(window.location);
    url.searchParams.set("tab", tab);
    window.history.replaceState({}, "", url);
  }, [tab]);

  return (
    <Layout tab={tab} setTab={setTab}>
      <Page />
    </Layout>
  );
}
