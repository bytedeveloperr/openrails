Doujins Billing Service — Operations Manual

Overview
- Purpose: Runs billing APIs and background workers, connects to Postgres for data, Garnet (Redis-compatible) for caching/rate limiting, and ClickHouse for analytics event logging.
- Zero‑config: Defaults are aligned with docker-compose. You can start the whole stack without writing config.

Stack
- Postgres: `supabase/postgres` (DB `supadb`, user `supabase_admin`, pass `password`)
- Garnet (Redis-compatible): `ghcr.io/microsoft/garnet` on `6379`
- ClickHouse: `clickhouse/clickhouse-server` (DB `analytics`, user `analytics_user`, pass `analytics_password`)
- Billing service: this server exposing public API on `:2052` and a private/internal port `:8060` (exposed to the compose network only). Admin routes require an internal shared secret.

Quick Start
- Start services: `task docker-up` (or `docker-compose up -d`)
- Follow logs: `task docker-logs` (Ctrl+C to stop following)
- Stop services: `task docker-down`

What happens on first boot
- Postgres migrations: `migrations/postgres/*.sql` are mounted to `/docker-entrypoint-initdb.d` and applied automatically by the Postgres image when the data volume is empty.
- ClickHouse migrations: `clickhouse-init` job waits for ClickHouse to be healthy, then applies `migrations/clickhouse/*.sql` to the `analytics` database.
- Billing service connects using built-in defaults that match the compose network/service names.

Defaults (match docker-compose)
- Postgres: `postgres://supabase_admin:password@postgres:5432/supadb?sslmode=disable`
- Redis (Garnet): `garnet:6379`, DB `0`
- ClickHouse: `http://clickhouse:8123` with `analytics_user/analytics_password` on DB `analytics`

Overriding configuration (optional)
- Config file: place `config.yaml` in repo root or `./config/config.yaml`.
- Env vars: common overrides include `DATABASE_URL`, `REDIS_URL`, `CLICKHOUSE_URL`, `CLICKHOUSE_DATABASE`, `CLICKHOUSE_USERNAME`, `CLICKHOUSE_PASSWORD`, `JWT_SECRET`, `JWT_ISSUER`.
- If not provided, the service uses the defaults above.

Developer tasks
- Build: `task build` → outputs `bin/billing`
- Run (binary): `task run` → builds then runs `billing server`
- Dev (no build): `task dev` → `go run ./ server`
- Test: `task test`
- Format: `task fmt`
- Clean: `task clean`

Service endpoints
- Health: `GET http://localhost:2052/health` → `{ "status": "ok", "service": "billing-private" }`
- API base: `http://localhost:2052/api/v1`
- Auth: JWT-based; supply `Authorization: Bearer <token>` where required by routes.

Networking
- Public: port `2052` is published to the host.
- Private: port `8060` is exposed to the Docker network for intra-service communication. Optionally, you can enable a private TLS listener with client cert verification (mTLS) via config.

Admin access
- Shared secret: admin routes are protected by header `X-Internal-Token: <token>`. Configure with env `INTERNAL_ADMIN_TOKEN`.
- mTLS (optional): set `tls.private.enabled: true` and provide `tls.private.cert_file`, `tls.private.key_file`. To require client certs, also set `tls.private.client_ca_file` and `tls.private.require_client_cert: true`.

JWT verification (Zitadel)
- The server verifies JWTs and extracts only `sub` (user ID) and `email`. Roles/claims are not used for authorization.
- For development, HMAC (`JWT_SECRET`) is supported. For Zitadel RS256, set `JWT_PUBLIC_KEY_PEM` to the issuer's RSA public key PEM.

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
- Check health: `curl http://localhost:2052/health`
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
