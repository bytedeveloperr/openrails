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

## Items 21–30

21) internal/api/subscription/get_products.go
- [x] Present as handler
  - `internal/handlers/get_products.go`
  - Route: `GET /api/v1/subscriptions/public/products`

22) internal/api/subscription/get_subscription.go
- [x] Present as handler
  - `internal/handlers/get_subscription.go`
  - Route: `GET /api/v1/subscriptions/active`

23) internal/api/subscription/get_subscription_history.go
- [x] Present as handler
  - `internal/handlers/get_subscription_history.go`
  - Route: `GET /api/v1/subscriptions/history`

24) internal/api/subscription/get_user_purchases.go
- [x] Present as handler, now routed
  - `internal/handlers/get_user_purchases.go`
  - Route: `GET /api/v1/subscriptions/purchases`

25) internal/api/subscription/manage.go
- [x] Present as admin handlers
  - `internal/handlers/manage.go` (UpdateStatus, ExtendSubscription)
  - Routes: admin scope under `/api/v1/subscriptions/:id/*`

26) internal/api/subscription/requests.go
- [x] Present as handler request DTOs
  - `internal/handlers/requests.go`

27) internal/api/subscription/responses.go
- [x] Present as handler responses
  - `internal/handlers/responses.go`

28) internal/api/subscription/subscribe.go
- [x] Present as handler
  - `internal/handlers/subscribe.go`
  - Route: `POST /api/v1/subscriptions/processor/:processor`

29) internal/api/subscription/webhook.go
- [x] Present as handler
  - `internal/handlers/webhook.go`
  - Route: `POST /api/v1/subscriptions/webhook/:processor`

30) internal/api/user/get_notifications.go
- [x] Implemented
  - `internal/handlers/notifications.go` (list, unread-count, mark-read)
  - Routes:
    - `GET /api/v1/notifications`
    - `GET /api/v1/notifications/unread-count`
    - `POST /api/v1/notifications/:id/read`

## Items 31–40

31) internal/api/wallet/connect_solana.go
- [x] Implemented (scaffold)
  - `internal/handlers/wallet_solana.go` (ConnectSolanaWallet)
  - Route: `POST /api/v1/wallet/solana/connect`

32) internal/api/wallet/delete_solana.go
- [x] Implemented (scaffold)
  - `internal/handlers/wallet_solana.go` (DeleteSolanaWallet)
  - Route: `DELETE /api/v1/wallet/solana`

33) internal/api/wallet/list_solana.go
- [x] Implemented (scaffold)
  - `internal/handlers/wallet_solana.go` (ListSolanaWallets)
  - Route: `GET /api/v1/wallet/solana`

34) internal/api/wallet/requests.go
- [x] Replaced with simple request struct
  - `internal/handlers/wallet_solana.go` (SolanaWalletRequest)

35) internal/api/wallet/responses.go
- [x] Responses are simple JSON maps in handlers (no dedicated types)

36) internal/api/wallet/verify_solana.go
- [x] Implemented (scaffold)
  - `internal/handlers/wallet_solana.go` (VerifySolanaWallet)
  - Route: `POST /api/v1/wallet/solana/verify`

37) internal/server/routes/admin/billing.go
- [x] Consolidated under single router
  - `internal/server/server.go` admin group

38) internal/server/routes/payment.go
- [x] Consolidated and extended
  - Solana routes under `/api/v1/solana/*`

39) internal/server/routes/subscription.go
- [x] Consolidated
  - Handled by `internal/server/server.go`

40) internal/server/routes/wallet.go
- [x] Consolidated and implemented (scaffold)
  - `internal/server/server.go` wallet group

## Items 41–50

41) internal/services/billing/billing_analytics_service.go
- [x] Present
  - `internal/services/billing_analytics_service.go`

42) internal/services/billing/billing_event_service.go
- [x] Present
  - `internal/services/billing_event_service.go` (ClickHouse logging)

43) internal/services/billing_history/service.go
- [~] Consolidated
  - Functionality covered by `PaymentService` (Postgres) and `BillingEventService` (ClickHouse). No direct one-to-one file; implement specific history queries as needed.

44) internal/services/mobius/classifier.go
- [~] Consolidated
  - Classifier logic integrated into `internal/services/webhook_mobius.go` and `internal/integrations/mobius/mobius.go`.

45) internal/services/mobius/mobius_api_client.go
- [x] Present (renamed and expanded)
  - `internal/integrations/mobius/mobius.go`

46) internal/services/mobius/mobius_service.go
- [x] Present (consolidated into services and handlers)
  - `internal/services/subscription.go`, `internal/services/webhook_mobius.go`, `internal/integrations/mobius/mobius.go`

47) internal/services/payment/solana_service.go
- [x] Implemented
  - `internal/services/solana_payment.go` (Generate + Submit; DB-backed pending/confirmed)

48) internal/services/subscription/admin_subscription_service.go
- [x] Present
  - `internal/services/admin_subscription_service.go`

49) internal/services/subscription/billing_logs.go
- [~] Consolidated/removed
  - Legacy logging replaced by `BillingEventService` (ClickHouse) and Postgres `PaymentService`. Not ported 1:1.

50) internal/services/subscription/ccbill_sync.go
- [x] Present
 - `internal/services/ccbill_sync.go`

## Items 51–60

51) internal/services/subscription/lifecycle_service.go
- [x] Present
  - `internal/services/lifecycle_service.go`

52) internal/services/subscription/manage_subscription.go
- [x] Present
  - `internal/services/manage_subscription.go`

53) internal/services/subscription/public_subscription_service.go
- [x] Present
  - `internal/services/public_subscription_service.go`

54) internal/services/subscription/subscription.go
- [x] Present
  - `internal/services/subscription.go`

55) internal/services/subscription/types.go
- [x] Present
  - `internal/services/types.go`

56) internal/services/subscription/user_subscription_service.go
- [x] Present
  - `internal/services/user_subscription_service.go`

57) internal/services/subscription/webhook_ccbill.go
- [x] Present
  - `internal/services/webhook_ccbill.go`

58) internal/services/subscription/webhook_mobius.go
- [x] Present
  - `internal/services/webhook_mobius.go`

59) internal/services/wallet/solana_service.go
- [~] Replaced/Consolidated
  - Wallet endpoints implemented as scaffolds in handlers: `internal/handlers/wallet_solana.go`
  - Solana payment logic implemented in `internal/services/solana_payment.go`

60) internal/workers/subscription_lifecycle.go
- [~] Consolidated
  - Worker management via River: `internal/state/river_init.go`, `internal/server/server.go#StartWorkers`
  - Lifecycle handled in `internal/services/lifecycle_service.go`
