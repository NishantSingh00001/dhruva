import React from "react";
import { Card, StatPair, PulseDots, TimelineFeed, CapsuleChart } from "../components.jsx";
import {
  demoFleet,
  fleetDots,
  demoGeofenceTraffic,
  geofenceDots,
  demoAlertTimeline,
  alertColors,
  demoEventCounts,
} from "../demoData.js";

export default function Dashboard() {
  return (
    <div>
      <h1 className="page-title">Fleet Overview</h1>

      <div
        className="grid"
        style={{ gridTemplateColumns: "1fr 1fr 1.4fr", alignItems: "start" }}
      >
        <Card title="Vehicle Activity">
          <StatPair
            a={{ value: demoFleet.active, unit: "", label: "Active Now", trend: demoFleet.activeTrend }}
            b={{ value: demoFleet.idle, unit: "", label: "Idle > 15min", trend: demoFleet.idleTrend }}
          />
          <PulseDots segments={fleetDots} />
          <p style={{ fontSize: 11, color: "var(--text-faint)", marginTop: 10 }}>
            One dot per vehicle · {demoFleet.offline} offline (gray)
          </p>
        </Card>

        <Card title="Geofence Traffic (Today)">
          <StatPair
            a={{ value: demoGeofenceTraffic.entries, unit: "", label: "Zone Entries", trend: demoGeofenceTraffic.entriesTrend }}
            b={{ value: demoGeofenceTraffic.exits, unit: "", label: "Zone Exits", trend: demoGeofenceTraffic.exitsTrend }}
          />
          <PulseDots segments={geofenceDots} />
          <p style={{ fontSize: 11, color: "var(--text-faint)", marginTop: 10 }}>
            One dot per event · {demoGeofenceTraffic.restrictedHits} restricted-zone hits (orange)
          </p>
        </Card>

        <Card title="Live Alert Feed">
          <TimelineFeed rows={demoAlertTimeline} colorMap={alertColors} />
        </Card>
      </div>

      <div
        className="grid"
        style={{ gridTemplateColumns: "1.1fr 1fr", marginTop: 20, alignItems: "start" }}
      >
        <Card title="Driving Events This Week">
          <CapsuleChart data={demoEventCounts} />
        </Card>

        <Card title="Why Dhruva Is Different">
          <ul style={{ margin: 0, paddingLeft: 18, color: "var(--text-dim)", lineHeight: 2, fontSize: 14 }}>
            <li>Real-time geofence alerts streamed over Kafka/Condense, not polled</li>
            <li>Per-driver behavior scoring from live telemetry, not after-the-fact reports</li>
            <li>Harsh braking, acceleration, overspeed and sharp-turn detection in one pipeline</li>
            <li>Dashboard built for dispatch, not just a map with pins</li>
          </ul>
        </Card>
      </div>
    </div>
  );
}
