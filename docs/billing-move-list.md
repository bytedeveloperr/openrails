# Doujins Billing Migration: Move-Over Checklist (HTTP-only Boundary)

This is the authoritative checklist for moving billing logic from doujins-backend → doujins-billing. The backend and billing will not share code or tables — only HTTP.

Guiding Principles
- No shared libraries: backend and billing do not import each other.
- No shared tables: billing owns subscription/payment storage; backend owns entitlements/roles.
- Frontend talks to billing for billing UX; backend uses an HTTP call for premium checks and other admin features.

## 1) Processor Integrations (CCBill, Mobius)
- [x] Move CCBill integration
  - [x] `internal/integrations/ccbill/ccbill.go`
  - [x] `internal/integrations/ccbill/client.go`
  - [x] `internal/integrations/ccbill/datalink.go`
- [x] Move Mobius integration
  - [x] `internal/integrations/mobius/mobius.go`
- [x] Move IP verification helpers
  - [x] `internal/utils/ipverification/ccbill.go`
- [ ] Expose clean interfaces from billing (FlexForm URL, tokenization, DataLink/recon, webhook verification)

## 2) Webhook DTOs (processor payload types)
- [ ] Extract processor DTOs from backend
  - [ ] `internal/api/types/webhooks.go` (Mobius/CCBill payload structs only)
- [x] Add DTOs to billing (`internal/services/types.go` has Mobius/CCBill types; `billing_event_service.go` has WebhookEventData)
- [ ] Keep app-specific types (user/app metadata) in backend

## 3) Solana Payment Config / Token Map
- [x] Move token map/config from backend
  - [x] `internal/config/solana.go` (token list, devnet/mainnet mapping) — removed from backend
- [x] Add to billing (present in `config.Solana.SupportedTokens`; handler `internal/handlers/solana_supported_tokens.go`)
- [x] Add default helpers in billing (`config/solana_tokens.go`) for mainnet/devnet token maps

## 4) Billing Domain Models (to billing `internal/db/models`)
- [x] `payment.go`
- [x] `payment_method.go`
- [x] `product_catalog.go`
- [x] `subscription.go`
- [x] `processor.go` (defined in `models.go`)
- [x] `solana_wallet.go`

## 5) Data Access in Services (no repo layer in billing)
- [x] Ensure service-level data access exists for:
  - [x] Payments: create/fetch/list/refund (`internal/services/payment.go`)
  - [x] Payment Methods: CRUD, activate/deactivate, list by user (`internal/services/payment_method.go`)
  - [x] Prices: lookup by product/plan (`internal/services/price.go`)
  - [x] Products: catalog listing/lookup (`internal/services/product.go`)
  - [x] Subscriptions: create/cancel/get-active/history/by-processor (`internal/services/subscription.go`)
  - [x] Solana Wallet: link/list/verify/delete (`internal/services/solana_wallet.go`)
  
Notes: Implement in `internal/services/*` using `internal/db` directly; do not create a `repo` layer in billing.

## 6) Vault Service (Mobius Vault)
- [x] Move `internal/services/vault/vault_service.go` to billing and wire to service/db access (no repos)
  - Implemented at `internal/services/vault_service.go` using `PaymentMethodService` and `SubscriptionService`.

## 7) Webhook Tooling + Test Payloads
- [x] Move replay tool
  - [x] `internal/services/webhook/replay_service.go`
  - [x] `internal/services/webhook/replay_test.go`
- [x] Move test payloads
  - [x] `testdata/webhooks/ccbill/*`
  - [x] `testdata/webhooks/mobius/*`
- [x] Place tooling under `internal/devtools/webhook/` (or keep under `internal/services/webhook/)` — kept under `internal/services/webhook/`

## 8) ClickHouse Billing Event Schema
- [x] Extract billing-only tables from backend migration
  - [x] `subscription_events`
  - [x] `payment_events`
  - [x] `webhook_events`
  - [x] `chargeback_events`
- [x] Add migration in billing `migrations/clickhouse/00000_billing_events.up.sql` (present)
  - Additional file present: `migrations/clickhouse/00001_billing_tables.sql`

## 9) Configuration (Move to Billing; Remove from Backend)
- [x] Define billing-only configs in billing service
  - [x] CCBill: salt, form info, datalink credentials, webhook_secret, test_mode, base_flexform_url, success/decline URLs
  - [x] Mobius: security_key, tokenization_key, webhook_secret, test_mode
  - [x] Solana: rpc_endpoint, recipient_wallet, supported tokens
- [x] Remove from backend example envs and example config any variables now owned by billing (processor secrets/settings removed)
- [x] Backend keeps only `BILLING_SERVER_URL`; frontend keeps `VITE_BILLING_SERVER_URL`. The frontend will call billing directly via its public routes.

## 10) Admin Billing Analytics (API)
- [X] Ensure billing exposes
  - [X] `GET /admin/subscriptions/dashboard-metrics`
  - [X] `GET /admin/subscriptions/daily-metrics`
  - [X] `GET /admin/subscriptions/processor-metrics`
- [ ] Back these endpoints with ClickHouse billing data

## 11) Backend Cleanup (After Migration Complete)
- [ ] Remove `internal/integrations/{ccbill,mobius}` and `internal/utils/ipverification/ccbill.go`
- [ ] Remove billing models and any leftover repo-style code (sections 4–5)
- [x] Remove webhook tooling and `testdata/webhooks` from backend
- [ ] Remove CCBill/Mobius payment config from backend (example env/config cleaned; code removal pending)
- [x] Remove billing tables from ClickHouse migrations in backend
- [ ] Replace any remaining subscription/payment usage with billing HTTP client or role checks

## 12) Reconciliation / Unification Tasks
- [ ] Unify models with billing conventions (timestamps/enums/relations)
- [ ] Unify service data access with billing DB wrapper and module imports
- [ ] Add unit/integration tests in billing for moved logic
- [ ] Build and run: `go build ./...` (billing)
- [ ] Validate billing endpoints via test payloads (webhook replay tool)
