import React, { useState } from "react";
import { Card, TimelineFeed } from "../components.jsx";
import { demoAlertTimeline, alertColors } from "../demoData.js";

const FILTERS = ["all", "overspeed", "harsh_braking", "sharp_turn", "exit", "entry", "restricted"];

export default function Alerts() {
  const [filter, setFilter] = useState("all");
  const rows = filter === "all" ? demoAlertTimeline : demoAlertTimeline.filter((r) => r.type === filter);

  return (
    <div>
      <h1 className="page-title">Alerts</h1>

      <div style={{ display: "flex", gap: 8, marginBottom: 20, flexWrap: "wrap" }}>
        {FILTERS.map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            style={{
              border: "1px solid var(--card-border)",
              background: filter === f ? "var(--green)" : "var(--card)",
              color: filter === f ? "#0a0a0a" : "var(--text-dim)",
              borderRadius: 999,
              padding: "8px 16px",
              fontSize: 13,
              fontWeight: 600,
              cursor: "pointer",
              textTransform: "capitalize",
            }}
          >
            {f.replace("_", " ")}
          </button>
        ))}
      </div>

      <div className="grid">
        <Card title={`Alert Timeline (${rows.length})`}>
          <TimelineFeed rows={rows} colorMap={alertColors} />
        </Card>
      </div>
    </div>
  );
}
