# Security review — Dhruva-Geofencing

Reviewed against the standard "vibe-code security" checklist before first push.

## 1. Secrets in frontend code
- `frontend/.env.example` only. No API keys, broker credentials, or tokens are
  hardcoded in `src/`. The frontend reads `VITE_API_BASE` / `VITE_API_KEY` from
  env at build time; `VITE_API_KEY`, if set, is sent as a header, never logged.
- **Action**: don't commit a real `.env` — it's gitignored. If `VITE_API_KEY`
  is ever set to a real key, remember Vite inlines `VITE_*` vars into the
  shipped JS bundle, so that key is public to anyone who loads the page. Treat
  it as a low-privilege, rate-limited, rotatable key, not a secret.

## 2. Data access without rules
- Every mutating backend route (`POST /geofences`, `POST /vehicles`,
  `POST /vehicles/location`, `POST /telemetry`, `POST /alerts/configure`,
  `DELETE /alerts/{id}`) goes through `a.authorized(r)`, which requires
  `API_KEY` to match when one is configured (carried over from
  northstar-geofencing's existing auth middleware).
- **Gap carried over, flagged not fixed**: if `API_KEY` is unset, the backend
  runs with no auth at all (same as the base project). Fine for local/demo use,
  not for a public deploy. Set `API_KEY` before deploying this anywhere
  reachable from the internet.
- New `/drivers/*` and `/telemetry` routes sit behind the same middleware —
  verified in `handlers.go`'s `routes()`.

## 3. Unlimited uploads
- No file upload endpoint exists in this project. N/A.

## 4. Paid features unchecked on the server
- No billing/paid-tier logic in this project. N/A.

## 5. Uncapped AI/agent endpoints
- No LLM/agent-calling endpoint in this project. N/A.
- The one thing that resembles an unbounded sink is `POST /telemetry`: it's a
  public ingestion endpoint with no payload rate limit of its own beyond the
  existing per-client limiter (`ratelimit.go`), which does apply to it since
  it's registered on the same mux. No further action needed, just noted.

## Hardening kit applied
- [x] `.gitignore` excludes `.env`, `.env.local`, build output, local state
- [x] `.github/workflows/gitleaks.yml` — scans every push/PR for committed secrets
- [x] This file
- [x] `CLAUDE.md` security-rules block

## Carried over from northstar-geofencing
The Kafka/Condense telemetry consumer (`kafka_consumer.go`) and its env-based
SASL/TLS config were written and reviewed on that repo's
`condense-kafka-ingestion` branch before being ported here. No changes to its
security posture were made during the port beyond renaming identifiers.
