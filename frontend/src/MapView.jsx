import React, { useEffect, useRef } from "react";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { fenceColors } from "./demoGeo.js";

const TILES = "https://tile.openstreetmap.org/{z}/{x}/{y}.png";

/**
 * Real Leaflet map: fence polygons + live vehicle markers.
 * onClick fires with (lat, lon) so the caller can simulate a location
 * update for the selected vehicle, same interaction northstar's map had.
 */
export default function MapView({ fences, vehicles, draft = [], onClick, height = 420 }) {
  const el = useRef(), map = useRef(), fitted = useRef(false);
  const layer = useRef({});
  const click = useRef(onClick);
  click.current = onClick;

  useEffect(() => {
    const m = L.map(el.current, { zoomControl: false }).setView([19.29, 73.05], 13);
    L.control.zoom({ position: "bottomright" }).addTo(m);
    L.tileLayer(TILES, {
      maxZoom: 19,
      className: "dhruva-dark-tiles",
      attribution: "© OpenStreetMap contributors",
    }).addTo(m);
    ["fences", "vehicles", "draft"].forEach((k) => (layer.current[k] = L.layerGroup().addTo(m)));
    m.on("click", (e) => click.current?.(+e.latlng.lat.toFixed(6), +e.latlng.lng.toFixed(6)));
    map.current = m;
    const ro = new ResizeObserver(() => m.invalidateSize());
    ro.observe(el.current);
    return () => {
      ro.disconnect();
      m.remove();
    };
  }, []);

  useEffect(() => {
    if (!map.current) return;
    const g = layer.current.fences.clearLayers();
    (fences || []).forEach((f) => {
      const c = fenceColors[f.category] || "#c6ff5e";
      L.polygon(f.coordinates, {
        color: c,
        weight: 2,
        fillColor: c,
        fillOpacity: 0.12,
        dashArray: "6 4",
      })
        .bindTooltip(`${f.name} · ${f.category}`, { sticky: true })
        .addTo(g);
    });
    if (!fitted.current && (fences || []).length) {
      map.current.fitBounds(L.featureGroup(g.getLayers()).getBounds().pad(0.5));
      fitted.current = true;
    }
  }, [fences]);

  useEffect(() => {
    if (!map.current) return;
    const g = layer.current.vehicles.clearLayers();
    (vehicles || [])
      .filter((v) => v.current_location)
      .forEach((v) => {
        const icon = L.divIcon({
          className: "veh-icon",
          html: `<span class="veh-pulse"></span><span class="veh-dot"></span><b>${v.vehicle_number}</b>`,
          iconSize: [0, 0],
        });
        L.marker([v.current_location.latitude, v.current_location.longitude], { icon })
          .bindTooltip(`${v.driver_name}`)
          .addTo(g);
      });
  }, [vehicles]);

  useEffect(() => {
    if (!map.current) return;
    const g = layer.current.draft.clearLayers();
    if (draft.length > 1) L.polyline(draft, { color: "#ffffff", weight: 2, dashArray: "5 5" }).addTo(g);
    draft.forEach((p, i) =>
      L.circleMarker(p, {
        radius: i === 0 ? 7 : 5,
        color: "#ffffff",
        weight: 2,
        fillColor: i === 0 ? "#c6ff5e" : "#ff8a3d",
        fillOpacity: 1,
      }).addTo(g)
    );
  }, [draft]);

  return <div ref={el} style={{ height, borderRadius: "var(--radius-md)" }} className="leaflet-shell" />;
}
