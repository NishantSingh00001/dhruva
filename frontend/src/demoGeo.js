// Real lat/lon demo data for the map, used only when the live backend isn't
// reachable. Shape matches what the Go API actually returns, so swapping in
// live data is a no-op for the map component.

export const demoFences = [
  {
    id: "f_depot",
    name: "Depot — Bhiwandi",
    category: "depot",
    coordinates: [
      [19.297, 73.045],
      [19.301, 73.045],
      [19.301, 73.052],
      [19.297, 73.052],
    ],
  },
  {
    id: "f_restricted",
    name: "Restricted — NH48 bypass",
    category: "restricted",
    coordinates: [
      [19.285, 73.06],
      [19.29, 73.058],
      [19.292, 73.067],
      [19.286, 73.069],
    ],
  },
  {
    id: "f_customer",
    name: "Customer site — Whitefield Annexe",
    category: "customer",
    coordinates: [
      [19.272, 73.04],
      [19.276, 73.039],
      [19.277, 73.046],
      [19.273, 73.047],
    ],
  },
];

export const demoVehiclesGeo = [
  { id: "v1", vehicle_number: "MH-04-AX-3391", driver_name: "R. Yadav", current_location: { latitude: 19.299, longitude: 73.048 } },
  { id: "v2", vehicle_number: "GJ-01-BT-7712", driver_name: "A. Singh", current_location: { latitude: 19.288, longitude: 73.063 } },
  { id: "v3", vehicle_number: "DL-3C-UV-9043", driver_name: "S. Verma", current_location: { latitude: 19.282, longitude: 73.052 } },
  { id: "v4", vehicle_number: "KA-05-PL-2231", driver_name: "P. Nair", current_location: { latitude: 19.274, longitude: 73.043 } },
  { id: "v5", vehicle_number: "RJ-14-GH-5520", driver_name: "M. Iqbal", current_location: { latitude: 19.291, longitude: 73.035 } },
];

export const fenceColors = { depot: "#c6ff5e", restricted: "#ff8a3d", customer: "#ffffff" };
