### Doujins Billing Service — Operations Manual

#### Scope
- Provides a billing-related API server for the frontend to use (signups, cancellations, etc.), and an admin-API server for the backend to use (admin cancellations).
- Handles webhooks from supported payment providers, and updates corresponding subscriptions / entitlements.
- Runs periodic jobs to update subscriptions / entitlements.

#### Interactions with other services (Intended Contract)
- Entitlements (app reads from): Billing owns the `billing.entitlements` table and writes premium access windows when memberships start/renew and revokes them on cancel/expiry. The main Doujins app can read this table to decide if a user is “premium” at a given point in time (current time ∈ [start_at, end_at) and `revoked_at IS NULL`).

- Profiles (billing reads from): When emailing users (e.g., subscription started/renewed/ended, payment failures, one‑off receipts), Billing reads the current email address from `profiles.users`. We treat user IDs as UUIDs; the service performs a direct, schema‑qualified lookup: `SELECT username, email, email_verified, is_active FROM profiles.users WHERE id = $1`.

---

#### Stack
- Postgres: shared `postgres:17-bookworm` container from the Doujins backend stack (DB `doujins_db`, user `admin` / `admin_password`)
- Garnet (Redis-compatible): `ghcr.io/microsoft/garnet` on `6379`
- ClickHouse: `clickhouse/clickhouse-server` (DB `analytics`, user `analytics_user`, pass `analytics_password`)
- Billing service: this server exposing public API on `:2053` and a private/internal port `:8060` (exposed to the compose network only). Admin routes require an internal shared secret.

#### Quick Start
- Ensure the shared Postgres container from `doujins-backend` is running and attached to the `local-doujins` network
- Start services: `task docker-up` (or `docker-compose up -d`)
- Follow logs: `task docker-logs` (Ctrl+C to stop following)
- Stop services: `task docker-down`

- Postgres bootstrap: `db-bootstrap` runs `migrations/bootstrap/*.sql` against the shared database to (re)create roles, schemas, and extensions.
- ClickHouse migrations: `clickhouse-init` job waits for ClickHouse to be healthy, then applies `migrations/clickhouse/*.sql` to the `analytics` database.
- Billing service connects using built-in defaults that match the compose network/service names.

- Postgres: `postgres://admin:admin_password@postgres:5432/doujins_db?sslmode=disable`
- Redis (Garnet): `garnet:6379`, DB `0`
- ClickHouse: `http://clickhouse:8123` with `analytics_user/analytics_password` on DB `analytics`

#### Overriding configuration (optional)
- Config file: place `config.yaml` in repo root or `./config/config.yaml`.
- Env vars: common overrides include `DB_URL`, `REDIS_ADDR`, `CLICKHOUSE_HTTP_ADDR`, `CLICKHOUSE_DATABASE`, `CLICKHOUSE_USERNAME`, `CLICKHOUSE_PASSWORD`, `AUTH_ISSUER`, `AUTH_AUDIENCE`.
- If not provided, the service uses the defaults above.

Developer tasks
- Build: `task build` → outputs `bin/billing`
- Run (binary): `task run` → builds then runs `billing server`
- Dev (no build): `task dev` → `go run ./ server`
- Test: `task test`
- Format: `task fmt`
- Clean: `task clean`

Service endpoints
- Health: `GET http://localhost:2053/health` → `{ "status": "ok", "service": "billing-private" }`
- API base: `http://localhost:2053/v1`
- Auth: JWT-based; supply `Authorization: Bearer <token>` where required by routes.

Networking
- Public: port `2053` is published to the host.
- Private: port `8060` is exposed to the Docker network for intra-service communication (same host, no TLS needed).

Admin access
- Shared secret: admin routes are protected by header `X-API-KEY: <token>`.
- Default (dev): `change-me-in-dev`.
- Override via env `BILLING_API_KEY` or config `admin.api_key`.

JWT verification (Verifier Only)
- Billing acts as a **JWT verifier**, not an issuer. It verifies tokens issued by doujins and/or hentai0.
- The middleware validates signature and claims, extracting `sub` (user ID), `email`, optional `preferred_username`/`username`/`name`, and `roles` if present.
- Configuration requirements:
  - `BILLING_AUTH_ISSUERS`: JSON array of token issuer URLs (e.g., `["http://localhost:2052", "http://localhost:4000"]` for local dev, or `["https://doujins.com", "https://hentai0.com"]` for production)
  - `AUTH_AUDIENCE`: The expected audience claim in JWTs (must be `billing-app`)
  - Public keys are automatically fetched from each `{issuer}/.well-known/jwks.json` per OIDC spec
- Signature verification:
  - RS256 only (RSA signatures)
  - Public keys fetched via JWKS discovery from each configured issuer (supports automatic key rotation)
  - Keys are cached for 15 minutes and refreshed automatically
- Required JWT claims:
  - `iss` must equal one of the configured issuers
  - `aud` must contain the configured audience (`billing-app`)
  - `exp` must be valid (not expired, with 60-second clock skew tolerance)
  - `sub` must be present (user ID)

- Postgres
  - Shared container: provided by `doujins-backend` compose (service name `postgres`).
  - Bootstrap SQL lives under `migrations/bootstrap/` and is applied by the `db-bootstrap` job; rerun by restarting that service.
- ClickHouse
  - Data volumes: `clickhouse_data`, `clickhouse_logs`.
  - Migrations live in `migrations/clickhouse/` and include tables for: `subscription_events`, `payment_events`, `webhook_events`, `acu_events`, `chargeback_events`.
  - To reapply, remove the data volume or re-run the `clickhouse-init` service after clearing state.
- Garnet
  - Data volume: `garnet_data` (optional for persistence). Used for caching/rate limiting.

Common operations
- Start fresh (wipe local analytics/cache data):
  1) `task docker-down`
  2) `docker volume rm <project>_clickhouse_data <project>_clickhouse_logs <project>_garnet_data`
  3) `task docker-up`
  4) (Optional) if you also need a fresh Postgres, reset it from the Doujins backend repository.
- Check health: `curl http://localhost:2053/health`
- Tail logs: `task docker-logs` or `docker-compose logs -f billing`

Troubleshooting
- Billing can’t connect to Postgres/Redis/ClickHouse:
  - Ensure services are healthy: `docker-compose ps` and `docker-compose logs <service>`.
  - Verify defaults weren’t overridden incorrectly (env/config). Remove overrides to return to zero‑config.
- Postgres bootstrap didn’t run:
  - Restart the `db-bootstrap` service. If the shared Postgres container was reset, ensure the `local-doujins` network still exists before bringing billing back up.
- ClickHouse tables missing:
  - Check `clickhouse-init` logs. Ensure `migrations/clickhouse/*.sql` exist and the database is `analytics`.

Container usage
- Runtime configs (`config.yaml`, `config.docker.yaml`, etc.) are not baked into the image. Mount the desired file and point the CLI at it, e.g. `docker run -v $(pwd)/config.docker.yaml:/app/config.docker.yaml:ro doujins/billing:latest -c /app/config.docker.yaml server`.
- The image entrypoint is the billing CLI. To launch workers only, override the command: `docker run ... doujins/billing:latest worker`.

Notes
- This repository manages only the billing service operations. Application-specific integration (e.g., role management in your app DB) is out of scope here.
 - Premium checks in the Doujins app should come from `billing.entitlements` (not from subscription rows). Email addresses should come from `profiles.users` (not denormalized into billing records).

