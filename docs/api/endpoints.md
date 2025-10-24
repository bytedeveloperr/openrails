# Billing Service API Reference

This document describes the HTTP endpoints exposed by the billing service. Routes are grouped by
purpose. Unless otherwise noted, responses are JSON encoded and errors follow the form:

```json
{
  "error": "human_readable_code",
  "message": "optional detail"
}
```

## Authentication & Headers

- **User endpoints** require a bearer token issued by your IdP (see `jwt.*` config). Pass using
  `Authorization: Bearer <token>`.
- **Admin endpoints** listen on the private handler and require `X-API-KEY` to match
  `admin.api_key`. These routes do **not** establish a user context; operations act directly on the
  supplied identifiers.
- **Webhooks** are public; callers must supply the appropriate processor secrets
  (e.g., IP allow-list for CCBill, HMAC signature for NMI).

## Rate Limiting

Public routes are throttled by the configured `rate_limits` block. Typical defaults:

- `SubscribeLimit`: 10 requests/minute (burst 3)
- `PaymentLimit`: 20 requests/minute (burst 5)
- `WebhookLimit`: 100 requests/minute (burst 20)

Admin routes are not rate limited by the application but should live behind network controls.

## Subscriptions (Public API)

### GET /v1/subscriptions/products
- **Auth:** none
- **Description:** Lists purchasable products with active prices.
- **Response:**
  ```json
  [
    {
      "id": "64ba7b37-3d1d-4e38-9de0-1f209d148c51",
      "name": "Premium",
      "description": "Unlimited access",
      "prices": [
        {
          "id": "3a58fb2f-0bec-4f0f-98a4-6aa45723b67d",
          "amount": 9.99,
          "currency": "USD",
          "billing_cycle_days": 30
        }
      ]
    }
  ]
  ```

### GET /v1/subscriptions/page-data
- **Auth:** none
- **Description:** Aggregate payload used by the subscription landing page (product catalog, feature flags, etc.).
- **Response:**
  ```json
  {
    "products": [...],
    "promo_banner": {
      "title": "Limited offer",
      "cta": "Subscribe now"
    }
  }
  ```

### POST /v1/subscriptions/process/:processor
- **Auth:** bearer token
- **Description:** Starts a subscription checkout for the requested processor (`nmi` or `ccbill`).
- **Body:**
  ```json
  {
    "price_id": "uuid",
    "processor": "nmi",
    "provider": "mobius",
    "email": "user@example.com",
    "first_name": "Jane",
    "last_name": "Doe",
    "address1": "...",
    "city": "...",
    "state": "...",
    "zip": "...",
    "country": "...",
    "payment_token": "collectjs token (nmi)"
  }
  ```
- **Notes:** `provider` is optional and selects the configured NMI account when multiple providers exist. When omitted, the price configuration or the default provider (`mobius`) is used.
- **Response:**
  ```json
  {
    "status": "redirect_required",
    "message": "CCBill payments now use FlexForm integration",
    "flexform_endpoint": "/v1/subscriptions/ccbill/flexform-url"
  }
  ```
  ```json
  {
    "subscription_id": "1d65d4f2-12d0-42ed-aaa6-1c93a98a84bb",
    "status": "pending"
  }
  ```

### POST /v1/subscriptions/ccbill/flexform-url
- **Auth:** bearer token
- **Description:** Generates an iframe URL for CCBill FlexForm.
- **Body:** `{"price_id": "uuid", "first_name": "...", "last_name": "...", "address1": "...", "city": "...", "state": "...", "zip_code": "...", "country": "US"}`
- **Response:**
  ```json
  {
    "iframe_url": "https://api.ccbill.com/wap-frontflex/flexforms/...",
    "width": "100%",
    "height": "600px",
    "success_url": "https://example.com/billing/success",
    "decline_url": "https://example.com/billing/decline"
  }
  ```

### POST /v1/subscriptions/cancel
- **Auth:** bearer token
- **Description:** Cancels the caller's active subscription.
- **Body:** `{ "feedback": "optional notes" }`
- **Response:**
  ```json
  {
    "success": true,
    "message": "subscription cancelled successfully"
  }
  ```

### GET /v1/subscriptions/active
- **Auth:** bearer token
- **Description:** Returns the enriched subscription record for the authenticated user. If none exists, a message is returned and HTTP 200.
- **Response:**
  ```json
  {
    "subscription": {
      "id": "dc8f2cee-b9bb-4a91-9e84-5bcc1a4fd0ba",
      "status": "active",
      "processor": "nmi",
      "current_period_starts_at": "2025-01-01T12:00:00Z",
      "current_period_ends_at": "2025-02-01T12:00:00Z"
    },
    "price": { "id": "3a58fb2f-0bec-4f0f-98a4-6aa45723b67d", "amount": 9.99, "currency": "USD" },
    "product": { "id": "64ba7b37-3d1d-4e38-9de0-1f209d148c51", "name": "Premium" }
  }
  ```

### GET /v1/subscriptions/history
- **Auth:** bearer token
- **Query:** `limit` (1-100, default 10), `offset` (>=0), `status` (optional filter).
- **Description:** Paginated subscription history.
- **Response:**
  ```json
  {
    "data": [
      {
        "id": "dc8f2cee-b9bb-4a91-9e84-5bcc1a4fd0ba",
        "status": "cancelled",
        "processor": "nmi",
        "current_period_starts_at": "2024-11-01T12:00:00Z",
        "current_period_ends_at": "2024-12-01T12:00:00Z"
      }
    ],
    "total_items": 4
  }
  ```

### GET /v1/subscriptions/purchases
- **Auth:** bearer token
- **Query:** `limit` (1-100, default 10), `offset` (>=0), `type` (processor filter, e.g., `solana`).
- **Response:**
  ```json
  {
    "data": [
      {
        "id": "5a76f178-4a9e-4fa2-8554-0cc6e0c0bfe4",
        "processor": "solana",
        "transaction_id": "3zY..aP",
        "amount": 4.99,
        "currency": "USD",
        "purchased_at": "2025-01-10T08:30:12Z"
      }
    ],
    "total_items": 2
  }
  ```

### POST /v1/subscriptions/webhook/:processor
- **Auth:** none (processor security applies)
- **Path:**
  - `POST /v1/subscriptions/webhook/:processor` where `processor` = `ccbill` or `nmi`.
  - `POST /v1/subscriptions/webhook/:processor/:provider` when addressing a specific NMI provider (e.g., `mobius`). A matching `provider` query string value is also accepted.
- **Description:** Ingests processor webhook callbacks. Payload format depends on processor:
  - `ccbill`: `application/x-www-form-urlencoded` with IP allow-list checking.
  - `nmi`: JSON body with HMAC signature (`X-Signature`, `X-NMI-Signature`, or legacy `X-Mobius-Signature`).
- **Response:** 200 on success, 401 for missing/invalid NMI signatures, 403 if a CCBill request fails IP validation, 400 for unrecognised processors or providers.

## Payment Methods

### GET /v1/payment-methods
- **Auth:** bearer token
- **Query:** `page`, `page_size` (1-500, default 20), `include_inactive` (bool).
- **Response:**
  ```json
  {
    "data": [
      {
        "id": "6d073ea2-12ac-4a35-8d39-7affc3439c99",
        "processor": "nmi",
        "vault_id": "cust-123",
        "last_four": "4242",
        "is_active": true
      }
    ],
    "page": 1,
    "page_size": 20,
    "total_items": 1,
    "total_pages": 1
  }
  ```

### DELETE /v1/payment-methods/:id
- **Auth:** bearer token
- **Path:** `id` = payment method UUID.
- **Response:**
  ```json
  {
    "success": true,
    "message": "Payment method deleted successfully"
  }
  ```

### PUT /v1/payment-methods/:id/activate
- **Auth:** bearer token
- **Description:** Marks the specified method as active after validation.
- **Response:**
  ```json
  {
    "success": true,
    "message": "Payment method activated successfully"
  }
  ```

## Notifications

### GET /v1/notifications
- **Auth:** bearer token
- **Query:** `page` (>=1), `page_size` (1-100, default 20), `seen` (`true`/`false`).
- **Response:**
  ```json
  {
    "data": [
      {
        "id": "6afefc80-76e5-4955-9d0d-4cb90f1a7381",
        "event_type": "premium_renewed",
        "data": { "period": "monthly" },
        "seen": false,
        "created_at": "2025-01-01T12:00:00Z"
      }
    ],
    "total_items": 5,
    "page": 1,
    "page_size": 20
  }
  ```

### GET /v1/notifications/unread-count
- **Auth:** bearer token
- **Response:**
  ```json
  { "unread_count": 3 }
  ```

### POST /v1/notifications/:id/read
- **Auth:** bearer token
- **Description:** Marks the notification as read. Body is empty.
- **Response:**
  ```json
  { "message": "notification marked as read" }
  ```

## Solana Wallets & Payments

### GET /v1/solana/tokens
- **Auth:** none
- **Description:** Returns configured Solana token metadata.
- **Response:** `{ "tokens": [{ "symbol", "name", "mint", "decimals", "price" }] }`

### POST /v1/solana/generate
- **Auth:** bearer token
- **Body:** `{ "price_id": "uuid", "token": "SOL", "user_wallet": "base58" }`
- **Description:** Builds a direct wallet payment intent and pre-signs a transaction scaffold.
- **Response:** `GeneratePaymentResponse` (base64 transaction, fiat/token amounts, intent ID, expiry).

### POST /v1/solana/submit
- **Auth:** bearer token
- **Body:** `{ "signed_transaction": "base64", "price_id": "uuid", "intent_id": "uuid", "memo": "optional" }`
- **Description:** Verifies/broadcasts the signed transaction, settles it via the subscription lifecycle, and records the payment.
- **Response:**
  ```json
  {
    "purchase_id": "5a76f178-4a9e-4fa2-8554-0cc6e0c0bfe4",
    "transaction_id": "3zY..aP",
    "status": "confirmed",
    "amount": 9.99,
    "currency": "USD",
    "processed_at": "2025-01-10T08:30:12Z",
    "intent_id": "9645f7c7-4d05-4d74-94f5-32e602a880a3",
    "message": "Payment recorded"
  }
  ```

### POST /v1/solana/qr
- **Auth:** bearer token
- **Body:** `{ "price_id": "uuid", "token": "USDC", "user_wallet": "base58" }`
- **Description:** Creates a Solana Pay intent and returns the QR-compatible URL plus metadata.
- **Response:**
  ```json
  {
    "url": "solana:...",
    "amount": 9.99,
    "token_amount": "9.990000",
    "token_symbol": "USDC",
    "label": "Doujins Purchase",
    "message": "",
    "expires_at": 1736205600,
    "reference": "4vW...",
    "intent_id": "b4f3b659-8a10-4603-b77c-4d9c8cb49a6c"
  }
  ```

### GET /v1/solana/check
- **Auth:** bearer token
- **Query:** `reference` (required), `memo` (optional).
- **Description:** Polls for a Solana Pay transfer referencing the supplied key. If confirmed, the intent is processed and the payment is recorded.
- **Response:**
  ```json
  {
    "status": "confirmed",
    "payment_id": "5a76f178-4a9e-4fa2-8554-0cc6e0c0bfe4",
    "intent_id": "b4f3b659-8a10-4603-b77c-4d9c8cb49a6c",
    "transaction": "3zY..aP"
  }
  ```

### POST /v1/wallet/solana/challenge
- **Auth:** bearer token
- **Body:** `{ "wallet": "base58" }`
- **Description:** Ensures a wallet record exists and returns a challenge (`message`, `nonce`, expiry) for signature verification.
- **Response:**
  ```json
  {
    "message": "Sign this to verify",
    "expires_at": 1736205600,
    "wallet": "AT3...",
    "nonce": "c9d3f2"
  }
  ```

### POST /v1/wallet/solana/verify
- **Auth:** bearer token
- **Body:** `{ "wallet": "base58", "signature": "base58", "message": "optional" }`
- **Description:** Validates the signature and marks the wallet as verified.
- **Response:**
  ```json
  {
    "verified": true,
    "wallet": "AT3...",
    "verified_at": "2025-01-10T08:30:12Z"
  }
  ```

### GET /v1/wallet/solana
- **Auth:** bearer token
- **Description:** Lists wallets linked to the account.
- **Response:**
  ```json
  {
    "wallets": [
      {
        "address": "AT3...",
        "is_verified": true,
        "linked_at": "2025-01-09T12:00:00Z"
      }
    ]
  }
  ```

### GET /v1/wallet/solana/linked
- **Auth:** bearer token
- **Description:** Returns the primary (most recently linked) wallet.
- **Response:**
  ```json
  {
    "wallet": {
      "address": "AT3...",
      "is_verified": true,
      "linked_at": "2025-01-09T12:00:00Z"
    }
  }
  ```

### DELETE /v1/wallet/solana
- **Auth:** bearer token
- **Query:** `wallet` (required base58 address).
- **Description:** Removes a linked wallet record.
- **Response:**
  ```json
  {
    "deleted": true,
    "wallet": "AT3..."
  }
  ```

## Billing Status

### GET /v1/me/billing-status
- **Auth:** bearer token
- **Description:** Summarises premium status, subscription record, next renewal date, and active entitlements.
- **Response:**
  ```json
  {
    "is_premium": true,
    "subscription": {
      "id": "dc8f2cee-b9bb-4a91-9e84-5bcc1a4fd0ba",
      "status": "active"
    },
    "next_renewal_at": "2025-02-01T12:00:00Z",
    "entitlements": [
      {
        "entitlement": "premium",
        "start_at": "2025-01-01T12:00:00Z",
        "end_at": null
      }
    ]
  }
  ```

## Webhook Replay Tooling

The service includes internal tooling (`internal/services/webhook/replay_service.go`) for replaying archived events. These are not exposed over HTTP but are valuable when triaging webhook issues.

## Health Probes

- `GET /health/live` → `{"status": "ok", "service": "billing"}`
- `GET /health/ready` → checks Postgres (and Redis if configured); returns 200 or 503.

Admin handler also exposes `GET/HEAD /health` for internal monitoring.

## Admin Endpoints (API key required)

> **Note:** Admin routes currently reuse some user handlers. For example `GET /v1/subscriptions/:id/details`
> dispatches to the user-oriented handler and therefore expects a user context. Invocation without a
> bearer token will return 401. The request/response contracts below describe the logical intent of
> each endpoint even where additional wiring work may be pending.

### PUT /v1/subscriptions/:id/extend
- **Auth:** `X-API-KEY`
- **Body:** `{ "subscription_id": "uuid", "duration": "72h3m" }` (Go `time.Duration` string).
- **Description:** Extends the current billing period by the supplied duration. The path parameter is ignored; the body `subscription_id` is required.
- **Response:** `{ "message": "subscription extended successfully" }`

### POST /v1/subscriptions/:id/cancel
- **Auth:** `X-API-KEY`
- **Description:** Currently wired to the customer-facing cancel handler; without a bearer token the call returns 401. When invoked with user context it cancels the caller’s subscription.

### GET /v1/subscriptions/:id/details
- **Auth:** `X-API-KEY`
- **Description:** Routed to the user subscription handler; expects a user context. For admin usage, prefer querying the database or adding a dedicated handler that accepts the path ID.

### GET /v1/subscriptions/dashboard-metrics
- **Auth:** `X-API-KEY`
- **Description:** Returns aggregate metrics (totals, revenue, active subscribers) for dashboards.

### GET /v1/subscriptions/daily-metrics
- **Auth:** `X-API-KEY`
- **Query:** `start=YYYY-MM-DD`, `end=YYYY-MM-DD` (required).
- **Description:** Returns per-day subscription and revenue metrics within the window.

### GET /v1/subscriptions/processor-metrics
- **Auth:** `X-API-KEY`
- **Description:** Splits metrics by processor (`ccbill`, `nmi`, `solana`, etc.).

### GET /v1/users/:user_id/entitlements
- **Auth:** `X-API-KEY`
- **Description:** Lists active entitlements for the supplied user ID at the current time.
- **Response:** Array of entitlement records `{ entitlement, start_at, end_at, source_type, source_id }`.

## Webhooks (Processor Specific)

### CCBill
- **Endpoint:** `POST /v1/subscriptions/webhook/ccbill`
- **Security:** Source IP must match configured allow-list (default ranges under `ccbill.webhook_ips`).
- **Payload:** Form-encoded fields documented by CCBill (event type via `eventType` query string).
- **Behaviour:** Deduplicates, validates IP, pushes subscription/payment updates via
  `CCBillWebhookService`.

### NMI
- **Endpoint:** `POST /v1/subscriptions/webhook/nmi`
- **Security:** HMAC signature header `X-Signature` or `X-NMI-Signature`; optional test mode bypass.
- **Payload:** JSON event mirroring NMI webhook schema (recurring subscription events).
- **Behaviour:** Signature verification (unless test mode), lifecycle updates, ClickHouse logging.

---

For additional integration details (e.g., webhook replay, billing analytics schemas) refer to the
source under `internal/services/webhook` and `internal/services/billing_event_service.go`.
