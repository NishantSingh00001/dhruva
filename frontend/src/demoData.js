// Sample data shown when the live API (Go backend + Kafka pipeline) isn't
// reachable, so the dashboard still looks alive for a hackathon demo/judging.

// Real counts (not percentages-for-their-own-sake): a 48-vehicle fleet,
// status as of now, and today's geofence event mix. The dot grids below
// render these counts directly, one dot per vehicle / per event.
export const demoFleet = {
  active: 32,
  idle: 9,
  offline: 7,
  activeTrend: "up",
  idleTrend: "down",
};
export const fleetDots = [
  { count: demoFleet.active, color: "g" },
  { count: demoFleet.idle, color: "o" },
  { count: demoFleet.offline, color: "" },
];

export const demoGeofenceTraffic = {
  entries: 26,
  exits: 18,
  restrictedHits: 4,
  entriesTrend: "up",
  exitsTrend: "down",
};
export const geofenceDots = [
  { count: demoGeofenceTraffic.entries, color: "g" },
  { count: demoGeofenceTraffic.exits, color: "w" },
  { count: demoGeofenceTraffic.restrictedHits, color: "o" },
];

export const demoAlertTimeline = [
  { date: "11.27", label: "Overspeed — MH-04-AX-3391", type: "overspeed", value: 24, pip: "⚠" },
  { date: "11.26", label: "Depot geofence exit — GJ-01-BT-7712", type: "exit", value: 31, pip: "⟲" },
  { date: "11.26", label: "Harsh braking — DL-3C-UV-9043", type: "harsh_braking", value: 12, pip: "●" },
  { date: "11.25", label: "Restricted zone entry — MH-12-KD-0087", type: "restricted", value: 18, pip: "✕" },
  { date: "11.25", label: "Sharp turn — KA-05-PL-2231", type: "sharp_turn", value: 9, pip: "↻" },
  { date: "11.24", label: "Customer site entry — RJ-14-GH-5520", type: "entry", value: 22, pip: "✓" },
];

export const alertColors = {
  overspeed: "var(--orange)",
  exit: "var(--green)",
  harsh_braking: "var(--orange)",
  restricted: "#ffffff",
  sharp_turn: "var(--green-dim)",
  entry: "var(--green)",
};

export const demoDriverScores = [
  { vehicle: "MH-04-AX-3391", driver: "R. Yadav", score: 58 },
  { vehicle: "DL-3C-UV-9043", driver: "S. Verma", score: 71 },
  { vehicle: "GJ-01-BT-7712", driver: "A. Singh", score: 84 },
  { vehicle: "KA-05-PL-2231", driver: "P. Nair", score: 92 },
  { vehicle: "RJ-14-GH-5520", driver: "M. Iqbal", score: 76 },
  { vehicle: "MH-12-KD-0087", driver: "J. Thomas", score: 64 },
];

export const demoEventCounts = [
  { label: "Harsh\nBraking", value: 34, color: "var(--orange)" },
  { label: "Harsh\nAccel.", value: 21, color: "var(--green)" },
  { label: "Overspeed", value: 48, color: "var(--orange)" },
  { label: "Sharp\nTurn", value: 16, color: "var(--green)" },
  { label: "Geofence\nExit", value: 27, color: "#ffffff" },
];

export const demoZones = [
  { name: "Depot — Bhiwandi", category: "depot", vehicles: 6 },
  { name: "Restricted — NH48 bypass", category: "restricted", vehicles: 2 },
  { name: "Customer site — Whitefield", category: "customer", vehicles: 3 },
  { name: "Customer site — Okhla Phase II", category: "customer", vehicles: 1 },
];

export const demoVehicleDots = [
  { x: 22, y: 30, color: "var(--green)" },
  { x: 58, y: 18, color: "var(--orange)" },
  { x: 71, y: 54, color: "var(--green)" },
  { x: 40, y: 66, color: "var(--orange)" },
  { x: 85, y: 72, color: "var(--green)" },
];
