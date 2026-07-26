# StatusPing

A service-uptime monitor. A Go backend polls a list of HTTP endpoints on a
fixed cycle, pushes live status to a React dashboard over WebSockets, and keeps
30 days of history. Latency is exported to Prometheus/Grafana; Sentry pages
after 3 consecutive failed checks. The whole stack runs behind Nginx via Docker
Compose.

## Architecture

```
                      ┌──────────── Nginx (:8080) ────────────┐
browser ── HTTP ──▶   │  /            → React dashboard        │
        ── WS   ──▶   │  /ws, /api/*  → Go backend            │
                      └───────────────────┬───────────────────┘
                                          │
                          ┌───────────────▼───────────────┐
   endpoints.json ──▶     │        Go backend (:8080)      │
   (20 targets, 15s)      │  poller → WS hub / metrics     │
                          └──┬──────────┬──────────┬───────┘
                             │          │          │
                     Redis (current)  Mongo     Prometheus ─▶ Grafana
                                    (30d TTL)              Sentry (paging)
```

- **Redis** — hot current-status cache per endpoint (fast reads, survives restart).
- **Mongo** — durable history, auto-expired by a 30-day TTL index.
- **WS fanout** — in-memory (single backend instance).

## Run

```bash
docker compose up --build
```

Then open:

- Dashboard — http://localhost:8080
- Prometheus — http://localhost:9090
- Grafana — http://localhost:3000 (anonymous admin; "StatusPing" dashboard is pre-provisioned)

To enable Sentry paging, set `SENTRY_DSN` on the `backend` service in
`docker-compose.yml`.

## Configure

- **Endpoints:** edit `endpoints.json` (`[{ "name", "url" }]`).
- **Poll interval:** `POLL_INTERVAL` env (default `15s`).

## Run the backend locally (no Docker)

```bash
go mod tidy                          # generates go.sum
go test ./...
MONGO_URI=mongodb://localhost:27017 REDIS_ADDR=localhost:6379 go run ./cmd/statusping
```

## Layout

```
cmd/statusping/main.go   entrypoint: config, wiring, HTTP server
internal/monitor/        poller, WS hub, store, metrics, endpoint loading
web/                     static React dashboard
grafana/ prometheus.yml  observability config
endpoints.json           the 20 targets
```

`internal/` means those packages can't be imported by other modules — the
compiler enforces it.

## Endpoints

- `GET /ws` — WebSocket stream of the full status snapshot each cycle.
- `GET /api/status` — current snapshot (JSON; initial-load fallback).
- `GET /metrics` — Prometheus metrics (`endpoint_up`, `endpoint_latency_seconds`).
- `GET /healthz` — liveness.
