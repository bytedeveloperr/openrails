# Doujins Backend → Billing Migration Checklist

Tracking files from doujins-backend commit `ee21eac` and their migration status into doujins-billing.

Scope below covers the first 10 items requested.

## Status Legend
- [x] Verified/Implemented in doujins-billing
- [ ] Needs follow-up

## First 10 Files

1) internal/api/admin/subscriptions/analytics_handler.go
- [x] Migrated as service methods and types
  - into: `internal/services/billing_analytics_service.go` (Dashboard/Daily/Processor metrics)

2) internal/api/admin/subscriptions/cancel_subscription.go
- [x] Migrated as handler/service
  - into: `internal/handlers/cancel_subscription.go`, `internal/services/admin_subscription_service.go`

3) internal/api/admin/subscriptions/get_subscribers.go
- [x] Migrated as service + pagination utilities
  - into: `internal/services/subscription.go` (GetSubscribers), `pkg/query`, and admin wrappers

4) internal/api/admin/subscriptions/requests.go
- [x] Migrated as handler request DTOs
  - into: `internal/handlers/requests.go` (BaseRequest, analytics params)

5) internal/api/admin/subscriptions/responses.go
- [x] Migrated as service types used by handlers
  - into: `internal/services/billing_analytics_service.go`

6) internal/api/admin/user/grant_role_manual.go
- [x] Migrated as admin service methods
  - into: `internal/services/admin_subscription_service.go` (CreateManualRoleGrant, RevokeRoleGrant, GetAllUserRoleGrants)

7) internal/api/payment/generate_payment.go
- [x] Implemented (Solana generate endpoint + DTOs)
  - into: `internal/handlers/solana_generate.go`
  - DTO: `internal/handlers/requests.go` (GeneratePaymentRequest)
  - Response: `internal/handlers/responses.go` (GeneratePaymentResponse)
  - route: `POST /api/v1/solana/generate` in `internal/server/server.go`

8) internal/api/payment/generate_qr.go
- [x] Implemented (Solana Pay QR endpoint + DTOs)
  - into: `internal/handlers/solana_qr.go`
  - DTO: `internal/handlers/requests.go` (GenerateSolanaPayQR)
  - Response: `internal/handlers/responses.go` (SolanaPayQRResponse)
  - route: `POST /api/v1/solana/qr` in `internal/server/server.go`

9) internal/api/payment/models.go
- [x] Ported equivalent response types
  - into: `internal/handlers/responses.go` (PaymentStatusResponse, ErrorResponse, SolanaPayQRResponse)

10) internal/api/payment/requests.go
- [x] Ported equivalent request DTOs (Solana)
  - into: `internal/handlers/requests.go` (GeneratePaymentRequest, GenerateSolanaPayQR)
  - Note: Mobius tokenized subscribe already exists via `services.SubscribeData.PaymentToken` and `handlers.Subscribe`.

## Additional Fixes
- Fixed handler binding bug in `internal/handlers/subscribe.go` (nil deref) to support Mobius tokenized flow reliably.

## Next Candidates (not in the first 10)
- If desired, extend with diffs/mappings for the remaining files from commit `ee21eac`.

