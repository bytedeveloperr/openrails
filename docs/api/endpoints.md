# Billing Service API Reference

The billing service exposes public catalog routes, authenticated user APIs, administrative endpoints, processor webhooks,
and a private service-to-service surface. All responses are JSON encoded. Unless otherwise stated, errors follow the
Stripe-style envelope:

```json
{
  "error": {
    "type": "invalid_request_error",
    "code": "invalid_parameter",
    "message": "Human readable description",
    "param": "optional_param_name"
  }
}
```

List endpoints use a Stripe-like envelope:

```json
{
  "object": "list",
  "data": [],
  "total": 0,
  "limit": 20,
  "offset": 0,
  "has_more": false,
  "url": "/v1/example"
}
```

`url` is included only on endpoints that use server-side pagination helpers; other list endpoints omit it.

## Authentication & Security

| Surface | How to authenticate |
|---------|---------------------|
| Public catalog (`/`, `/v1/products`, `/v1/prices`, `/v1/solana/tokens`, health probes) | No auth required |
| User routes (`/v1/checkout`, `/v1/me/*`) | `Authorization: Bearer <JWT>` issued by AuthKit |
| Admin routes (`/v1/admin/*`) | Same JWT header, user must have the `admin` role |
| Service API (private port 8060) | `X-API-KEY` header matching `config.api_key` |
| Webhooks (`/v1/webhooks/:provider`) | Provider-specific verification (see notes) |

## Health & Service Banner

### GET /
Returns a short JSON banner (`{"service":"billing","status":"ok","endpoints":[...]}`) so load balancers can
confirm the API is reachable.

### GET /health/live, /healthz
Unconditional liveness probes.

### GET /health/ready, /readyz
Runs readiness checks against Postgres, Redis, and the AuthKit verifier. Returns 200 when all checks pass,
or 503 with `{ status: "not_ready", checks: {...} }`.

## Public Catalog Endpoints

### GET /v1/products
Lists products with embedded active prices. Query parameters mirror Stripe's `/v1/products`:
`active` (default `true`, only honoured for admins), `limit` (1-100, default 20), `offset` (>=0).
Response: `ListResponse<Product>`.

### GET /v1/prices
Lists price objects. Query parameters: `active`, `currency`, `product` (product ID), `type`
(`recurring` or `one_time`), plus `limit`/`offset`. Response: `ListResponse<Price>`.

### GET /v1/prices?product=prod_xxx
Same endpoint; explicitly documenting that product filters accept either the `prod_` prefixed ID or a
raw UUID.

### GET /v1/solana/tokens
Returns the currently supported Solana tokens and live pricing:

```json
{
  "tokens": [
    { "symbol": "USDC", "name": "USD Coin", "mint": "...", "decimals": 6, "price": 1.0 }
  ]
}
```

### POST /v1/webhooks/{provider}
Receives processor webhooks. `provider` is `ccbill`, `mobius`, or `stripe` (legacy `nmi` is accepted as an alias for
`mobius`).
- `ccbill`: form-encoded payload, verified via source IP ranges (unless test mode).
- `mobius`/NMI-backed: JSON body with `X-Signature`/`X-NMI-Signature` header.
- `stripe`: JSON body with `Stripe-Signature` header (if configured).
Returns 200 with `{ status: "accepted" }` on success, 401/403 for auth failures, 400 for unknown provider.

## Checkout Sessions (Authenticated)

### POST /v1/checkout
Creates a checkout session for **new** subscriptions and one-off purchases.

> **Note:** Tier upgrades/downgrades are **not** supported via this endpoint. If the user already has
> an active subscription in the same tier group, the response will be `{ "status": "blocked" }` with a
> message directing the client to use `POST /v1/me/subscriptions/change-tier` instead.

- Auth: bearer token
- Optional header: `X-Idempotency-Key` to dedupe create requests
- Body:
  - `price_id` (required)
  - `mode` (optional) – `one_off` or `subscription`; if omitted, resolved from the price
  - `payment` (required):
    - `processor` (required) – `mobius`, `ccbill`, `solana`, or `stripe`
    - `payment_method_id` or `payment_token` for `mobius`/`stripe`
    - `token_symbol` for `solana`
    - `flow` for `solana` – `transfer_request` (default) or `transaction_request`
    - `wallet` required for `transaction_request`
    - Billing details for `ccbill`/`stripe`: `email`, `first_name`, `last_name`, `address1`, `city`, `state`, `zip`, `country`
  - `metadata` (optional string map)
- Response: `CheckoutSessionResponse` with `payment` details, `next_action` (redirect/solana), and optional
  `payment_id` / `subscription_id` once completed.

### GET /v1/checkout/{id}
Retrieves a checkout session by ID. Returns `CheckoutSessionResponse`. Responds with 403 if the session
belongs to another user.

### POST /v1/checkout/{id}/confirm
Confirms a Solana checkout session.
- Body: `{ payment: { processor: "solana", signature: "...", wallet?: "..." } }`
- Response: `CheckoutSessionResponse`
- Errors: 400 validation, 403 forbidden, 404 not found, 409 conflict, 410 expired

## Authenticated User API (`/v1/me`)

Every endpoint in this section requires a valid JWT for the current user.

### GET /v1/me/status
Aggregated premium status: whether the user currently has an active membership, the enriched
subscription object, the next renewal timestamp, and all entitlement records.
Response includes `has_active_subscription`, `subscription`, `next_renewal_at`, and `entitlements`.

### GET /v1/me/subscriptions
History of the caller's subscriptions. Query params: `status` (`pending`, `active`, `past_due`, `cancelled`, or `all`),
`limit`, `offset`. Response: `ListResponse<UserSubscription>` (with `product`, `price`, `access`).

### GET /v1/me/subscriptions/{id}
Retrieves a single subscription by ID with enriched product, price, and access data.
Returns `UserSubscriptionResponse`. Returns 404 if subscription is not found or doesn't belong to the user.

### PUT /v1/me/subscriptions/{id}/payment-method
Request body `{ "payment_method_id": "pm_uuid" }`. Updates the card vault entry
used for an NMI-backed subscription (CCBill/Solana subscriptions cannot be reassigned). Returns:
`{ success, message, subscription_id, payment_method_id }`.

### POST /v1/me/subscriptions/{id}/cancel
Body `{ "feedback": "optional text" }`. Cancels the specified subscription and returns
`202 { "status": "queued" }`. For CCBill subscriptions, returns
`422 { error, support_url, code }` because cancellation must be performed via the CCBill portal.

### POST /v1/me/subscriptions/{id}/resume
Queues a resume for a cancelled Stripe subscription. Returns `202 { "status": "queued" }`.
Returns 400 if the subscription is not cancelled or not a Stripe subscription.

### POST /v1/me/subscriptions/{id}/change-tier
Unified tier change endpoint for upgrades and downgrades across all processors.

**Request:**
- Body: `{ "price_id": "price_..." }`
- Optional header: `X-Idempotency-Key` for retry safety

**Response:** `TierChangeResponse`
```json
{
  "object": "tier_change",
  "status": "succeeded|requires_action|blocked",
  "mode": "tier_change",
  "action": "upgrade|downgrade",
  "price_id": "price_...",
  "payment": { "processor": "stripe|mobius|ccbill" },
  "subscription_id": "sub_...",
  "next_action": { "type": "redirect_to_url", "redirect_to_url": { "url": "..." } },
  "message": "...",
  "delayed_start": "2024-02-15T00:00:00Z"
}
```

**Processor behavior:**
| Processor | Upgrade | Downgrade |
|-----------|---------|-----------|
| Stripe | `succeeded` (immediate with proration) | `succeeded` (immediate, no proration) |
| Mobius/NMI | `succeeded` (immediate proration charge) | `succeeded` + `delayed_start` (scheduled) |
| CCBill | `requires_action` + redirect to FlexForm | `blocked` + message |
| Solana | HTTP 400 (not supported) | HTTP 400 (not supported) |

**Notes:**
- Target price must be in the same tier group as current subscription
- For CCBill upgrades, client must redirect to `next_action.redirect_to_url.url`
- For scheduled downgrades, the change takes effect at `delayed_start`

### GET /v1/me/payments
Lists one-off payments. Query params: `type` (processor filter), `limit`, `offset`.
Response: list of `PaymentRecord` entries (raw payment model with optional `price` and `subscription`).

### GET /v1/me/payment-methods
Query params: `limit`, `offset`, `include_inactive`. Response: list of stored payment methods.

### POST /v1/me/payment-methods
Body includes `payment_token` (Collect.js token) plus billing details (`first_name`, `last_name`, `address1`, `city`,
`state`, `zip`, `country`, optional `email`, `phone`, `company`, `address2`, `provider`). Creates and activates an NMI
vault record. Response: payment method object.

### PUT /v1/me/payment-methods/{id}
Body accepts a new `payment_token` and optional billing fields (all pointers). Replaces the stored vault card for the
referenced method. Returns updated payment method.

### DELETE /v1/me/payment-methods/{id}
Soft-deletes the stored method. Response `{ success, message }`.

### PUT /v1/me/payment-methods/{id}/activate
Re-verifies and marks the method active. Response `{ success, message }`.

### GET /v1/me/notifications
Query params: `limit` (1-100), `offset`, `seen` (`true`/`false`). Response list of
notifications `{ id, event_type, data, seen, created_at }`.

### GET /v1/me/notifications/unread-count
Returns `{ unread_count: <int> }`.

### POST /v1/me/notifications/{id}/read
Marks the notification as read. Response `{ message: "notification marked as read" }`.

### GET /v1/me/credits
Returns all active credit balances for the current user (promo + purchased).
Each entry: `{ type, display_name, unit, decimal_places, balance, held_balance, permanent_balance, expiring_balance, earliest_expiry? }`.

### GET /v1/me/credits/{type}
Returns the credit balance for a single credit type (e.g. `api_credits`).

### GET /v1/me/credits/{type}/transactions
Lists credit ledger entries for the credit type. Query params: `limit`, `offset`.

### POST /v1/me/portal
Creates a Stripe customer portal session. Response `{ redirect_url }`.

## Service API (Private Port 8060)

All endpoints require `X-API-KEY` and run on the private port.

### GET /v1/users/{user_id}/entitlements
Returns active entitlements for the user at the current time. Optional query param `at` (RFC3339) to query
entitlements at a specific time. Response: array of entitlement records.

### GET /v1/users/{user_id}/credits
Returns credit balance summary for a user. Optional query param `type` (defaults to `api_credits`).
Response: `{ type, balance, held_balance, permanent_balance, expiring_balance }`.

### POST /v1/credits/withdraw
Withdraw credits. Body: `{ user_id, credit_type, amount, source, source_id? }`.
Returns a `CreditTransaction`. On insufficient credits, returns 402 with `insufficient_credits`.

### POST /v1/credits/hold
Reserve credits for long-running jobs. Body: `{ user_id, credit_type, amount, source, source_id, expires_at }` (epoch seconds).
Returns a `CreditHold`. On insufficient credits, returns 402.

### POST /v1/credits/hold/{id}/capture
Capture a hold: `{ amount }` (amount <= hold). Returns a `CreditTransaction`.

### POST /v1/credits/hold/{id}/release
Release a hold without spending credits. Response `{ ok: true }`.

## Admin API (`/v1/admin`, JWT + `admin` role)

### GET /v1/admin/subscriptions
List subscriptions with filtering. Common query params include `user_id`, `status`, `price_id`, `processor`,
`created_after`, `created_before`, `cancelled_after`, `cancelled_before`, `expires_before`, `sort_by`, `sort_order`,
plus `limit`/`offset`. Response: list of admin subscription records (raw subscription + product/price).

### GET /v1/admin/subscriptions/{id}
Detailed subscription record including linked payments.

### POST /v1/admin/subscriptions/{id}/cancel
Immediate cancellation of the referenced subscription. Body `{ "reason": "optional" }`.
Currently only supports NMI-backed processors (Mobius). Subscription must be active.
Cancels with payment processor, updates local record, and immediately revokes entitlements.

### GET /v1/admin/payments
List of payments with filtering by processor, status, date range, etc. Response: list envelope of Stripe-style
`PaymentObject` entries.

### GET /v1/admin/payments/{id}
Full payment detail including refund history.

### POST /v1/admin/payments/{id}/refund
Body `{ "amount": 1234, "refund_transaction_id": "..." }`. Initiates a refund via the underlying processor.
Returns the refund payment object.

### GET /v1/admin/users/{user_id}
Returns the user's billing profile: `{ user_id, subscription, entitlements, payments }`.

### GET /v1/admin/users/{user_id}/payments
Lists all payments for the user. Query params: `limit`, `offset`.

### POST /v1/admin/users/{user_id}/payments/off-channel
Records an off-channel/manual purchase (cash, bank transfer, etc.) and grants entitlements/credits.
Body:
```json
{
  "price_id": "price_...",
  "transaction_id": "unique-reference",
  "amount": 1000,
  "currency": "usd",
  "purchased_at": "2024-01-15T00:00:00Z",
  "discount_code": "PROMO10",
  "discount_reason": "Staff discount"
}
```
Returns `{ payment_id, entitlements, delayed_start, eligibility }`. Idempotent on `transaction_id`.

### GET /v1/admin/users/{user_id}/entitlements
Lists all entitlements. Optional `at` query param (RFC3339) for point-in-time lookup.

### POST /v1/admin/users/{user_id}/entitlements
Body `{ "entitlement": "premium", "days": 30 }`. Grants an entitlement for the requested duration (or indefinite
if `days` is omitted).

### DELETE /v1/admin/users/{user_id}/entitlements/{id}
Revokes the referenced admin entitlement.

### POST /v1/admin/users/{user_id}/grants
Creates a structured grant record. Body `{ price_id, reason, duration_days?, amount?, currency?, transaction_id? }`.

### GET /v1/admin/users/{user_id}/grants
Lists grants issued to the user.

### GET /v1/admin/grants/{id}
Fetches a single grant record.

### GET /v1/admin/metrics/summary
Returns KPI card data (MRR, ARR, total revenue, churn, ARPU). Query params: `start`, `end`, `period`, `currency`.

### GET /v1/admin/metrics/revenue
Time-series revenue buckets. Query params: `start`, `end`, optional `granularity` (`day`, `week`, `month`), `currency`.

### GET /v1/admin/metrics/subscriptions
Subscription activity series (new subs, cancels, reactivations, net change). Supports `start`, `end`, `granularity`,
`currency`.

### GET /v1/admin/metrics/processors
Aggregated revenue + counts by processor for a date range (defaults to last 30 days). Query params: `start`, `end`,
`currency`.

### GET /v1/admin/metrics/churn
Monthly churn summary plus cancellation reason counts and coarse cohort retention info. Accepts `start`, `end`,
`currency`.

## Webhook Notes

- **CCBill**: Must originate from the published IP ranges; the handler also validates `formName`/`flexId`
  against the price metadata.
- **Mobius/NMI-backed**: Supply `X-Signature`/`X-NMI-Signature`. When test mode is enabled via config the
  signature check is bypassed.
- **Stripe**: Uses the `Stripe-Signature` header with the configured webhook secret.
