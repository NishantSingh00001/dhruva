import React, { useEffect, useState } from "react";
import { Card, Legend } from "../components.jsx";
import MapView from "../MapView.jsx";
import { api } from "../api.js";
import { demoFences, demoVehiclesGeo, fenceColors, CATEGORIES, categoryLabel } from "../demoGeo.js";

export default function Geofencing() {
  const [fences, setFences] = useState(demoFences);
  const [vehicles, setVehicles] = useState(demoVehiclesGeo);
  const [live, setLive] = useState(false);
  const [selected, setSelected] = useState(demoVehiclesGeo[0]?.id || "");
  const [mode, setMode] = useState("simulate"); // simulate | draw
  const [draft, setDraft] = useState([]);
  const [status, setStatus] = useState("Click the map to move the selected vehicle (demo mode — not sent to a backend)");

  async function refresh() {
    const [realFences, realVehicles] = await Promise.all([api.fences(), api.vehicles()]);
    let backendUp = false;
    if (realFences) {
      setFences(realFences);
      backendUp = true;
    }
    if (realVehicles) {
      setVehicles(realVehicles);
      if (realVehicles.length) setSelected((s) => s || realVehicles[0]?.id);
      backendUp = true;
    }
    setLive(backendUp);
    return backendUp;
  }

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const ok = await refresh();
      if (!cancelled && ok) setStatus("Connected to the live backend. Click the map to send a real location update.");
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleMapClick(lat, lon) {
    if (mode === "draw") {
      setDraft((d) => [...d, [lat, lon]]);
      return;
    }
    if (!selected) return;
    if (!live) {
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
      refresh();
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
            <div style={{ display: "flex", gap: 6 }}>
              <button
                onClick={() => setMode("simulate")}
                style={pillBtnStyle(mode === "simulate")}
              >
                Simulate movement
              </button>
              <button onClick={() => setMode("draw")} style={pillBtnStyle(mode === "draw")}>
                Draw geofence
              </button>
            </div>
            {mode === "simulate" ? (
              <>
                <select value={selected} onChange={(e) => setSelected(e.target.value)}>
                  {vehicles.map((v) => (
                    <option key={v.id} value={v.id}>
                      {v.vehicle_number}
                    </option>
                  ))}
                </select>
                <span className="hint">click the map to send a location update</span>
              </>
            ) : (
              <span className="hint">click to add points ({draft.length} so far, need 3+)</span>
            )}
          </div>

          <MapView fences={fences} vehicles={vehicles} draft={draft} onClick={handleMapClick} />

          <p style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 10 }}>{status}</p>

          <Legend
            items={CATEGORIES.map((c) => ({ label: categoryLabel(c), color: fenceColors[c] }))}
            total={vehicles.length}
          />
        </Card>

        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
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
                <span style={{ fontSize: 13, color: "var(--text-dim)" }}>{z.vehicleCount} inside</span>
              </div>
            ))}
          </Card>

          {mode === "draw" ? (
            <CreateGeofenceForm
              draft={draft}
              setDraft={setDraft}
              live={live}
              onCreated={(msg) => {
                setStatus(msg);
                refresh();
              }}
            />
          ) : (
            <RegisterVehicleForm
              live={live}
              onCreated={(msg) => {
                setStatus(msg);
                refresh();
              }}
            />
          )}
        </div>
      </div>
    </div>
  );
}

function pillBtnStyle(active) {
  return {
    border: "1px solid var(--card-border)",
    background: active ? "var(--green)" : "var(--card-raised)",
    color: active ? "#0a0a0a" : "var(--text-dim)",
    borderRadius: 999,
    padding: "8px 14px",
    fontSize: 13,
    fontWeight: 600,
    cursor: "pointer",
  };
}

function inputStyle() {
  return {
    width: "100%",
    background: "var(--card-raised)",
    border: "1px solid var(--card-border)",
    borderRadius: 10,
    padding: "9px 12px",
    color: "var(--text)",
    fontSize: 13,
    marginBottom: 10,
  };
}

function CreateGeofenceForm({ draft, setDraft, live, onCreated }) {
  const [name, setName] = useState("");
  const [category, setCategory] = useState(CATEGORIES[0]);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  async function submit(e) {
    e.preventDefault();
    if (!name.trim()) return setMsg("Name is required.");
    if (draft.length < 3) return setMsg("Click at least 3 points on the map first.");
    const closed = closeRing(draft);
    if (!live) {
      setMsg(`Demo mode: would create "${name}" (${category}) with ${closed.length} points. Connect a backend to persist it.`);
      setDraft([]);
      return;
    }
    setBusy(true);
    const res = await api.createGeofence({ name, category, coordinates: closed });
    setBusy(false);
    if (res.ok) {
      setName("");
      setDraft([]);
      onCreated(`Geofence "${name}" created.`);
    } else {
      setMsg(`Failed to create geofence (${res.status || res.error}).`);
    }
  }

  return (
    <Card title="Create Geofence">
      <form onSubmit={submit}>
        <input style={inputStyle()} placeholder="Zone name" value={name} onChange={(e) => setName(e.target.value)} />
        <select style={inputStyle()} value={category} onChange={(e) => setCategory(e.target.value)}>
          {CATEGORIES.map((c) => (
            <option key={c} value={c}>
              {categoryLabel(c)}
            </option>
          ))}
        </select>
        <button type="submit" disabled={busy} style={{ ...pillBtnStyle(true), width: "100%" }}>
          {busy ? "Creating…" : "Create geofence"}
        </button>
      </form>
      {msg && <p style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 10 }}>{msg}</p>}
    </Card>
  );
}

function RegisterVehicleForm({ live, onCreated }) {
  const [f, setF] = useState({ vehicle_number: "", driver_name: "", vehicle_type: "truck", phone: "" });
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });

  async function submit(e) {
    e.preventDefault();
    if (!f.vehicle_number || !f.driver_name || !f.phone) return setMsg("All fields are required.");
    if (!live) {
      setMsg(`Demo mode: would register ${f.vehicle_number}. Connect a backend to persist it.`);
      return;
    }
    setBusy(true);
    const res = await api.createVehicle(f);
    setBusy(false);
    if (res.ok) {
      setF({ vehicle_number: "", driver_name: "", vehicle_type: "truck", phone: "" });
      onCreated(`Vehicle ${f.vehicle_number} registered.`);
    } else {
      setMsg(`Failed to register (${res.status || res.error}).`);
    }
  }

  return (
    <Card title="Register Vehicle">
      <form onSubmit={submit}>
        <input style={inputStyle()} placeholder="Registration, e.g. KA-01-AB-1234" value={f.vehicle_number} onChange={set("vehicle_number")} />
        <input style={inputStyle()} placeholder="Driver name" value={f.driver_name} onChange={set("driver_name")} />
        <select style={inputStyle()} value={f.vehicle_type} onChange={set("vehicle_type")}>
          {["truck", "car", "van", "motorcycle"].map((t) => (
            <option key={t}>{t}</option>
          ))}
        </select>
        <input style={inputStyle()} placeholder="Phone" value={f.phone} onChange={set("phone")} />
        <button type="submit" disabled={busy} style={{ ...pillBtnStyle(true), width: "100%" }}>
          {busy ? "Registering…" : "Register vehicle"}
        </button>
      </form>
      {msg && <p style={{ fontSize: 12, color: "var(--text-dim)", marginTop: 10 }}>{msg}</p>}
    </Card>
  );
}

function closeRing(pts) {
  if (!pts.length) return pts;
  const [a0, a1] = pts[0];
  const [b0, b1] = pts[pts.length - 1];
  if (a0 === b0 && a1 === b1) return pts;
  return [...pts, [a0, a1]];
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
