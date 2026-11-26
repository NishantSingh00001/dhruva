# Dhruva-Geofencing

Real-time geofencing, driver-behavior monitoring and alerting platform.
Built for Configure 2026 (Condense x Bosch Mobility Platforms & Solutions
mobility hackathon) by team **Jobless AI Coder**.

This is a standalone project, intentionally separate from
`northstar-geofencing`. Do not merge, branch from, or push this project's
history into that repo, and vice versa.

## Layout
- `backend/` — Go HTTP API: geofencing, vehicle tracking, alert rules,
  Kafka/Condense telemetry ingestion, and driver-behavior scoring
  (`driver_behavior.go`).
- `frontend/` — React + Vite dashboard (dark, card-grid, pill-nav UI).

## Security rules
- Never commit `.env`, real API keys, database URLs, or Kafka broker
  credentials. `.env.example` files hold placeholders only.
- Every new mutating HTTP route must be registered behind the existing
  `a.authorized(r)` check in `handlers.go`'s `routes()` — don't add a route
  outside that pattern.
- Before deploying anywhere public, set `API_KEY` (backend) — an unset key
  means no auth at all.
- `VITE_*` env vars in the frontend are inlined into the public JS bundle at
  build time. Never put a real secret behind a `VITE_` prefix.
- Run `gitleaks` locally (or let the `.github/workflows/gitleaks.yml` CI job
  run on push/PR) before pushing anything that touched config or env files.
- See `SECURITY_REVIEW.md` for the full audit against the 5-mistakes
  checklist (secrets in frontend, data access without rules, unlimited
  uploads, unchecked paid features, uncapped AI/agent endpoints).

## Conventions
- Go backend: stdlib `net/http` mux (`mux.HandleFunc("METHOD /path", ...)`),
  no framework. In-memory `DrivingStore` for driver-behavior state (see
  `driver_behavior.go`) — separate from the Postgres-backed `pgStore`, by
  design, so it works standalone without a DB migration.
- Frontend: no router library — tabs are plain React state, mirrored to
  `?tab=` in the URL for deep links. Theme lives entirely in `theme.css`
  using CSS custom properties (`--green`, `--orange`, `--bg`, etc.) — change
  the palette there, not per-component.
- PR workflow: one PR per logical change, reviewed and merged by the repo
  owner (not direct pushes to `main`).
