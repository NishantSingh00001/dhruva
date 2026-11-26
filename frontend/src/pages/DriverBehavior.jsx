import React from "react";
import { Card, ScoreList, CapsuleChart, EventChip } from "../components.jsx";
import { demoDriverScores, demoEventCounts } from "../demoData.js";

const recentEvents = [
  { vehicle: "MH-04-AX-3391", type: "Overspeed", severity: "high", time: "2 min ago" },
  { vehicle: "DL-3C-UV-9043", type: "Harsh braking", severity: "medium", time: "6 min ago" },
  { vehicle: "KA-05-PL-2231", type: "Sharp turn", severity: "low", time: "11 min ago" },
  { vehicle: "MH-12-KD-0087", type: "Harsh acceleration", severity: "medium", time: "19 min ago" },
];

export default function DriverBehavior() {
  return (
    <div>
      <h1 className="page-title">Driver Behavior</h1>

      <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", alignItems: "start" }}>
        <Card title="Driver Score Leaderboard">
          <ScoreList rows={demoDriverScores.sort((a, b) => b.score - a.score)} />
        </Card>

        <Card title="Harsh Event Breakdown">
          <CapsuleChart data={demoEventCounts} />
        </Card>
      </div>

      <div className="grid" style={{ marginTop: 20 }}>
        <Card title="Recent Incidents">
          {recentEvents.map((e, i) => (
            <div
              key={i}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                padding: "12px 0",
                borderBottom: i < recentEvents.length - 1 ? "1px solid var(--card-border)" : "none",
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
                <EventChip severity={e.severity}>{e.severity}</EventChip>
                <span style={{ fontWeight: 600, fontSize: 14 }}>{e.type}</span>
                <span style={{ color: "var(--text-dim)", fontSize: 13 }}>{e.vehicle}</span>
              </div>
              <span style={{ fontSize: 12, color: "var(--text-faint)" }}>{e.time}</span>
            </div>
          ))}
        </Card>
      </div>
    </div>
  );
}
