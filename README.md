Doujins Billing Service — Operations Manual

Overview
- Purpose: Runs billing APIs and background workers, connects to Postgres for data, Garnet (Redis-compatible) for caching/rate limiting, and ClickHouse for analytics event logging.
- Zero‑config: Defaults are aligned with docker-compose. You can start the whole stack without writing config.

Stack
- Postgres: `postgres:17-bookworm` (DB `doujins_db`, app user `app_user` / `app_password`)
- Garnet (Redis-compatible): `ghcr.io/microsoft/garnet` on `6379`
- ClickHouse: `clickhouse/clickhouse-server` (DB `analytics`, user `analytics_user`, pass `analytics_password`)
- Billing service: this server exposing public API on `:2053` and a private/internal port `:8060` (exposed to the compose network only). Admin routes require an internal shared secret.

Quick Start
- Start services: `task docker-up` (or `docker-compose up -d`)
- Follow logs: `task docker-logs` (Ctrl+C to stop following)
- Stop services: `task docker-down`

What happens on first boot
- Postgres migrations: `migrations/postgres/*.sql` are mounted to `/docker-entrypoint-initdb.d` and applied automatically by the Postgres image when the data volume is empty.
- ClickHouse migrations: `clickhouse-init` job waits for ClickHouse to be healthy, then applies `migrations/clickhouse/*.sql` to the `analytics` database.
- Billing service connects using built-in defaults that match the compose network/service names.

Defaults (match docker-compose)
- Postgres: `postgres://app_user:app_password@postgres:5432/doujins_db?sslmode=disable`
- Redis (Garnet): `garnet:6379`, DB `0`
- ClickHouse: `http://clickhouse:8123` with `analytics_user/analytics_password` on DB `analytics`

Overriding configuration (optional)
- Config file: place `config.yaml` in repo root or `./config/config.yaml`.
- Env vars: common overrides include `DATABASE_URL`, `REDIS_URL`, `CLICKHOUSE_URL`, `CLICKHOUSE_DATABASE`, `CLICKHOUSE_USERNAME`, `CLICKHOUSE_PASSWORD`, `CASDOOR_SERVER_URL`.
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
- API base: `http://localhost:2053/api/v1`
- Auth: JWT-based; supply `Authorization: Bearer <token>` where required by routes.

Networking
- Public: port `2053` is published to the host.
- Private: port `8060` is exposed to the Docker network for intra-service communication. Optionally, you can enable a private TLS listener with client cert verification (mTLS) via config.

Admin access
- Shared secret: admin routes are protected by header `X-API-KEY: <token>`. Configure via env `BILLING_INTERNAL_API_KEY`.
- mTLS (optional): set `tls.private.enabled: true` and provide `tls.private.cert_file`, `tls.private.key_file`. To require client certs, also set `tls.private.client_ca_file` and `tls.private.require_client_cert: true`.

JWT verification (Casdoor)
- Public endpoints use JWTs issued by your IdP (Casdoor). The middleware validates signature and claims, extracts `sub` (user ID), `email`, optional `preferred_username`/`username`/`name`, and `roles` if present.
- Supported signing:
  - HS256/384/512 with `JWT_SECRET` (Casdoor Application Client Secret).
  - RS256 via either:
    - `JWT_PUBLIC_KEY_PEM`, or
    - OIDC discovery from `CASDOOR_SERVER_URL` and JWKS lookup (no extra config needed).
- Required claims:
  - `iss` must equal `CASDOOR_SERVER_URL` (e.g., `https://casdoor.example.com`).
  - `aud` must contain `CASDOOR_CLIENT_ID` (Casdoor Application Client ID).
  - `exp` must be valid.

Data stores and migrations
- Postgres
  - Data volume: `postgres_data` (Docker volume).
  - Re-run init SQL: remove the volume to force re-init: `docker volume rm doujins-billing_postgres_data` (volume name may differ; check with `docker volume ls`).
  - Custom SQL: place additional `.sql` files under `migrations/postgres/`.
- ClickHouse
  - Data volumes: `clickhouse_data`, `clickhouse_logs`.
  - Migrations live in `migrations/clickhouse/` and include tables for: `subscription_events`, `payment_events`, `webhook_events`, `acu_events`, `chargeback_events`.
  - To reapply, remove the data volume or re-run the `clickhouse-init` service after clearing state.
- Garnet
  - Data volume: `garnet_data` (optional for persistence). Used for caching/rate limiting.

Common operations
- Start fresh (wipe data):
  1) `task docker-down`
  2) `docker volume rm <project>_postgres_data <project>_clickhouse_data <project>_clickhouse_logs <project>_garnet_data`
  3) `task docker-up`
- Check health: `curl http://localhost:2053/health`
- Tail logs: `task docker-logs` or `docker-compose logs -f billing`

Troubleshooting
- Billing can’t connect to Postgres/Redis/ClickHouse:
  - Ensure services are healthy: `docker-compose ps` and `docker-compose logs <service>`.
  - Verify defaults weren’t overridden incorrectly (env/config). Remove overrides to return to zero‑config.
- Postgres init SQL didn’t run:
  - The Postgres image only runs `/docker-entrypoint-initdb.d` on first init of the data directory. Remove the `postgres_data` volume to reapply.
- ClickHouse tables missing:
  - Check `clickhouse-init` logs. Ensure `migrations/clickhouse/*.sql` exist and the database is `analytics`.

Notes
- This repository manages only the billing service operations. Application-specific integration (e.g., role management in your app DB) is out of scope here.
