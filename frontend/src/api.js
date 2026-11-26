// Thin client for the Dhruva-Geofencing Go backend.
// Falls back to null on any failure so the dashboard can show demo data
// when the backend / Kafka pipeline isn't running (hackathon demo mode).

const BASE = import.meta.env.VITE_API_BASE || "http://localhost:8080";

async function get(path) {
  try {
    const res = await fetch(BASE + path, { headers: authHeaders() });
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}

async function post(path, body) {
  try {
    const res = await fetch(BASE + path, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeaders() },
      body: JSON.stringify(body),
    });
    if (!res.ok) return { ok: false, status: res.status };
    return { ok: true, data: await res.json() };
  } catch (e) {
    return { ok: false, error: e.message };
  }
}

function authHeaders() {
  const key = import.meta.env.VITE_API_KEY;
  return key ? { "X-API-Key": key } : {};
}

export const api = {
  health: () => get("/health"),
  vehicles: async () => (await get("/vehicles"))?.vehicles ?? null,
  fences: async () => (await get("/geofences"))?.geofences ?? null,
  violations: async () => (await get("/violations/history?limit=20"))?.violations ?? null,
  driverScores: async () => (await get("/drivers/scores?limit=20"))?.scores ?? null,
  driverEvents: async (vehicleId) => (await get(`/drivers/${vehicleId}/events?limit=20`))?.events ?? null,
  pushLocation: (vehicleId, lat, lon) =>
    post("/vehicles/location", { vehicle_id: vehicleId, latitude: lat, longitude: lon, timestamp: new Date().toISOString() }),
  base: BASE,
};
