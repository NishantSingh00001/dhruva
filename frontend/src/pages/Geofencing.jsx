import React, { useEffect, useState } from "react";
import { Card, Legend } from "../components.jsx";
import MapView from "../MapView.jsx";
import { api } from "../api.js";
import { demoFences, demoVehiclesGeo, fenceColors } from "../demoGeo.js";

export default function Geofencing() {
  const [fences, setFences] = useState(demoFences);
  const [vehicles, setVehicles] = useState(demoVehiclesGeo);
  const [live, setLive] = useState(false);
  const [selected, setSelected] = useState(demoVehiclesGeo[0]?.id || "");
  const [status, setStatus] = useState("Click the map to move the selected vehicle (demo mode — not sent to a backend)");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [realFences, realVehicles] = await Promise.all([api.fences(), api.vehicles()]);
      if (cancelled) return;
      if (realFences?.length) {
        setFences(realFences);
        setLive(true);
      }
      if (realVehicles?.length) {
        setVehicles(realVehicles);
        setSelected(realVehicles[0]?.id);
        setLive(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleMapClick(lat, lon) {
    if (!selected) return;
    if (!live) {
      // demo mode: move the marker locally, no backend call
      setVehicles((vs) =>
        vs.map((v) => (v.id === selected ? { ...v, current_location: { latitude: lat, longitude: lon } } : v))
      );
      setStatus(`Moved ${selected} to ${lat}, ${lon} (demo — not persisted)`);
      return;
    }
    setStatus("Sending location update…");
    const res = await api.pushLocation(selected, lat, lon);
    if (res.ok) {
      const inside = res.data?.current_geofences?.length ?? 0;
      setStatus(`Vehicle moved. Now inside ${inside} geofence(s).`);
      const realVehicles = await api.vehicles();
      if (realVehicles?.length) setVehicles(realVehicles);
    } else {
      setStatus(`Update failed (${res.status || res.error}).`);
    }
  }

  const zoneCounts = fences.map((f) => ({
    ...f,
    vehicleCount: vehicles.filter(
      (v) => v.current_location && pointRoughlyInside(f.coordinates, v.current_location)
    ).length,
  }));

  return (
    <div>
      <h1 className="page-title">Geofencing</h1>

      <div className="grid" style={{ gridTemplateColumns: "1.6fr 1fr", alignItems: "start" }}>
        <Card title={`Live Map ${live ? "(connected)" : "(demo data)"}`} className="map-card">
          <div className="map-toolbar">
            <span className="hint">Simulate movement:</span>
            <select value={selected} onChange={(e) => setSelected(e.target.value)}>
              {vehicles.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.vehicle_number}
                </option>
              ))}
            </select>
            <span className="hint">click the map to send a location update</span>
          </div>

          <MapView fences={fences} vehicles={vehicles} onClick={handleMapClick} />

          <p style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 10 }}>{status}</p>

          <Legend
            items={[
              { label: "Depot", color: fenceColors.depot },
              { label: "Restricted", color: fenceColors.restricted },
              { label: "Customer site", color: fenceColors.customer },
            ]}
            total={vehicles.length}
          />
        </Card>

        <Card title="Active Zones">
          {zoneCounts.map((z, i) => (
            <div
              key={z.id || i}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                padding: "12px 0",
                borderBottom: i < zoneCounts.length - 1 ? "1px solid var(--card-border)" : "none",
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <span
                  style={{
                    width: 10,
                    height: 10,
                    borderRadius: "50%",
                    background: fenceColors[z.category] || "#999",
                    display: "inline-block",
                  }}
                />
                <span style={{ fontSize: 14, fontWeight: 600 }}>{z.name}</span>
              </div>
              <span style={{ fontSize: 13, color: "var(--text-dim)" }}>{z.vehicleCount} vehicles inside</span>
            </div>
          ))}
        </Card>
      </div>
    </div>
  );
}

// Rough bounding-box containment for the zone-count sidebar (good enough for
// the demo grid; the backend does exact point-in-polygon for real alerts).
function pointRoughlyInside(coords, loc) {
  const lats = coords.map((c) => c[0]);
  const lons = coords.map((c) => c[1]);
  const pad = 0.004;
  return (
    loc.latitude >= Math.min(...lats) - pad &&
    loc.latitude <= Math.max(...lats) + pad &&
    loc.longitude >= Math.min(...lons) - pad &&
    loc.longitude <= Math.max(...lons) + pad
  );
}
