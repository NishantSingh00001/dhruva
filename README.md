# Dhruva-Geofencing

Real-time geofencing, driver-behavior monitoring and alerting for fleet
operations — built for **Configure 2026** (Condense x Bosch Mobility
Platforms & Solutions mobility hackathon) by **Jobless AI Coder**.

A standalone project. Not a fork or branch of any other repo.

## What it does

- **Geofencing**: define zones (depots, restricted areas, customer sites),
  track vehicles, and fire entry/exit alerts in real time over a WebSocket
  stream (`/ws/alerts`).
- **Driver behavior monitoring**: ingests live telemetry (speed, heading) and
  scores each vehicle/driver on harsh braking, harsh acceleration, overspeed,
  and sharp turns — computed on the fly, not from an end-of-day batch report.
- **Condense/Kafka ingestion**: optional Kafka consumer
  (`backend/kafka_consumer.go`) feeds location updates straight into the
  geofencing pipeline, so Condense's mobility data plane can drive alerts
  without a custom bridge.
- **Dashboard-first UI**: dark, card-grid dashboard with a live alert
  timeline, driver-score leaderboard, and a zone map — not just a single map
  view.

## Stack

- Backend: Go (stdlib `net/http`, optional PostgreSQL + PostGIS persistence,
  `segmentio/kafka-go` for telemetry ingestion)
- Frontend: React + Vite, no UI framework — hand-built theme to match the
  project's own dashboard design

## Running locally

### Backend
```bash
cd backend
cp .env.example .env   # fill in only what you need; everything is optional
go run .
```
Key env vars (all optional, see `.env.example` and `main.go`):
- `API_KEY` — if set, required on every route except `/health`
- `DATABASE_URL` — enables Postgres/PostGIS persistence; without it, state is
  an in-memory + optional JSON-snapshot store
- `KAFKA_BROKERS` — set to enable the Condense/Kafka telemetry consumer

### Frontend
```bash
cd frontend
npm install
npm run dev
```
Set `VITE_API_BASE` (defaults to `http://localhost:8080`) and optionally
`VITE_API_KEY` in a `frontend/.env.local` if the backend requires one.

The dashboard runs standalone with built-in demo data if the backend isn't
reachable, so the UI is always presentable even without the full pipeline up.

## API surface (backend)

| Method | Path | Purpose |
|---|---|---|
| POST | `/geofences` | create a zone |
| GET | `/geofences` | list zones |
| POST | `/vehicles` | register a vehicle |
| GET | `/vehicles` | list vehicles |
| POST | `/vehicles/location` | push a location update (fires geofence events) |
| POST | `/telemetry` | push a speed/heading reading (fires driver-behavior events) |
| GET | `/drivers/scores` | leaderboard of driver-behavior scores |
| GET | `/drivers/{id}/score` | one vehicle's current score |
| GET | `/drivers/{id}/events` | one vehicle's harsh-event history |
| GET | `/violations/history` | geofence entry/exit history |
| GET | `/ws/alerts` | live alert stream |

## Security

See `SECURITY_REVIEW.md` for the audit against the standard vibe-code-security
checklist, and `CLAUDE.md` for the security rules this repo is built to.

## Team

**Jobless AI Coder** — Configure 2026, Condense x Bosch Mobility Platforms &
Solutions mobility hackathon.
