import React, { useEffect, useState } from "react";
import { Card, ScoreList, CapsuleChart, EventChip } from "../components.jsx";
import { api } from "../api.js";
import { demoDriverScores, demoEventCounts } from "../demoData.js";
import { demoVehiclesGeo } from "../demoGeo.js";

const demoRecentEvents = [
  { vehicle: "MH-04-AX-3391", type: "overspeed", severity: "high", time: "2 min ago" },
  { vehicle: "DL-3C-UV-9043", type: "harsh_braking", severity: "medium", time: "6 min ago" },
  { vehicle: "KA-05-PL-2231", type: "sharp_turn", severity: "low", time: "11 min ago" },
  { vehicle: "MH-12-KD-0087", type: "harsh_acceleration", severity: "medium", time: "19 min ago" },
];

export default function DriverBehavior() {
  const [live, setLive] = useState(false);
  const [scores, setScores] = useState(demoDriverScores.map((d) => ({ vehicle: d.vehicle, score: d.score })));
  const [vehicles, setVehicles] = useState(demoVehiclesGeo);
  const [selected, setSelected] = useState(demoVehiclesGeo[0]?.id || "");
  const [events, setEvents] = useState(demoRecentEvents);
  const [breakdown, setBreakdown] = useState(demoEventCounts);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("");

  async function refresh() {
    const [realScores, realVehicles] = await Promise.all([api.driverScores(), api.vehicles()]);
    let backendUp = false;
    if (realVehicles) {
      setVehicles(realVehicles);
      setSelected((s) => s || realVehicles[0]?.id);
      backendUp = true;
    }
    if (realScores) {
      setScores(realScores.map((s) => ({ vehicle: s.vehicle_id, score: s.score })));
      backendUp = true;
    }
    setLive(backendUp);
    return backendUp;
  }

  useEffect(() => {
    refresh();
  }, []);

  useEffect(() => {
    if (!live || !selected) return;
    api.driverEvents(selected).then((evs) => {
      if (evs) {
        setEvents(
          evs.map((e) => ({
            vehicle: e.vehicle_id,
            type: e.event_type,
            severity: e.severity,
            time: new Date(e.timestamp).toLocaleTimeString(),
          }))
        );
      }
    });
    api.driverScore(selected).then((s) => {
      if (!s) return; // 404: no telemetry yet for this vehicle — keep demo breakdown
      setBreakdown([
        { label: "Harsh\nBraking", value: s.harsh_braking_count, color: "var(--orange)" },
        { label: "Harsh\nAccel.", value: s.harsh_acceleration_count, color: "var(--green)" },
        { label: "Overspeed", value: s.overspeed_count, color: "var(--orange)" },
        { label: "Sharp\nTurn", value: s.sharp_turn_count, color: "var(--green)" },
      ]);
    });
  }, [live, selected, status]);

  // Pushes a short telemetry sequence designed to trip harsh-braking,
  // harsh-acceleration and overspeed detection, then refreshes — a live
  // demo of the driver-behavior feature end to end.
  async function simulate() {
    if (!selected) return;
    if (!live) {
      setStatus("Demo mode: connect the backend to run a real telemetry simulation.");
      return;
    }
    setBusy(true);
    setStatus("Pushing telemetry…");
    const now = Date.now();
    const seq = [
      { speed: 20, heading: 90, t: 0 },
      { speed: 70, heading: 92, t: 2000 }, // hard acceleration
      { speed: 95, heading: 95, t: 4000 }, // overspeed
      { speed: 25, heading: 150, t: 5000 }, // hard braking + sharp turn
    ];
    for (const p of seq) {
      await api.pushTelemetry(selected, p.speed, p.heading);
      await new Promise((r) => setTimeout(r, 150));
    }
    setStatus("Telemetry sent. Refreshing scores…");
    await refresh();
    setBusy(false);
    setStatus("Done — check the score and incidents below.");
  }

  return (
    <div>
      <h1 className="page-title">Driver Behavior</h1>

      <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", alignItems: "start" }}>
        <Card title={`Driver Score Leaderboard ${live ? "(live)" : "(demo)"}`}>
          <ScoreList rows={[...scores].sort((a, b) => b.score - a.score)} />
        </Card>

        <Card title="Simulate Telemetry">
          <div style={{ display: "flex", gap: 10, alignItems: "center", marginBottom: 12 }}>
            <select
              value={selected}
              onChange={(e) => setSelected(e.target.value)}
              style={{
                background: "var(--card-raised)",
                border: "1px solid var(--card-border)",
                borderRadius: 999,
                padding: "8px 14px",
                color: "var(--text)",
                fontSize: 13,
              }}
            >
              {vehicles.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.vehicle_number}
                </option>
              ))}
            </select>
            <button
              onClick={simulate}
              disabled={busy}
              style={{
                border: "1px solid var(--card-border)",
                background: "var(--green)",
                color: "#0a0a0a",
                borderRadius: 999,
                padding: "8px 16px",
                fontSize: 13,
                fontWeight: 700,
                cursor: "pointer",
              }}
            >
              {busy ? "Running…" : "Push test drive"}
            </button>
          </div>
          <p style={{ fontSize: 12, color: "var(--text-dim)" }}>
            Sends a short speed/heading sequence built to trip harsh acceleration, overspeed, harsh
            braking and a sharp turn for the selected vehicle — a live run of the new driver-behavior
            pipeline, not canned data.
          </p>
          {status && <p style={{ fontSize: 12, color: "var(--green)", marginTop: 8 }}>{status}</p>}
          {!live && (
            <p style={{ fontSize: 11, color: "var(--text-faint)", marginTop: 8 }}>
              No backend reachable right now — this panel needs `go run .` in `backend/` with
              `VITE_API_BASE` pointed at it.
            </p>
          )}
        </Card>
      </div>

      <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", marginTop: 20, alignItems: "start" }}>
        <Card title="Harsh Event Breakdown">
          <CapsuleChart data={breakdown} />
        </Card>

        <Card title="Recent Incidents">
          {events.map((e, i) => (
            <div
              key={i}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                padding: "12px 0",
                borderBottom: i < events.length - 1 ? "1px solid var(--card-border)" : "none",
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
                <EventChip severity={severityOf(e.severity)}>{e.severity}</EventChip>
                <span style={{ fontWeight: 600, fontSize: 14 }}>{e.type.replace(/_/g, " ")}</span>
                <span style={{ color: "var(--text-dim)", fontSize: 13 }}>{e.vehicle}</span>
              </div>
              <span style={{ fontSize: 12, color: "var(--text-faint)" }}>{e.time}</span>
            </div>
          ))}
          {!events.length && <p style={{ fontSize: 13, color: "var(--text-dim)" }}>No incidents yet for this vehicle.</p>}
        </Card>
      </div>
    </div>
  );
}

function severityOf(s) {
  return ["high", "medium", "low"].includes(s) ? s : "low";
}
