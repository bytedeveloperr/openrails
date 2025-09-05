package services

import (
	"fmt"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/google/uuid"
)

type IdempotencyService struct{}

func NewIdempotencyService(db *db.DB) *IdempotencyService {
    return &IdempotencyService{}
}

// GenerateForChargeSuccess generates key for successful charge
// Format: v5({processor}:{transaction_id})
func (g *IdempotencyService) GenerateForChargeSuccess(processor, transactionID string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // UUID v5 namespace
	data := fmt.Sprintf("%s:%s", processor, transactionID)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForRenewal generates key for subscription renewal
// Format: v5({subscription_id}:{period_end_iso})
func (g *IdempotencyService) GenerateForRenewal(subscriptionID uuid.UUID, periodEndISO string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:%s", subscriptionID.String(), periodEndISO)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForCancelNow generates key for immediate cancellation
// Format: v5({subscription_id}:cancel:{event_id})
func (g *IdempotencyService) GenerateForCancelNow(subscriptionID uuid.UUID, eventID string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b812-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:cancel:%s", subscriptionID.String(), eventID)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForTrialStart generates key for trial start
// Format: v5({subscription_id}:trial:{trial_end_iso})
func (g *IdempotencyService) GenerateForTrialStart(subscriptionID uuid.UUID, trialEndISO string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b813-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:trial:%s", subscriptionID.String(), trialEndISO)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForGracePeriod generates key for grace period
// Format: v5({subscription_id}:grace:{grace_until_iso})
func (g *IdempotencyService) GenerateForGracePeriod(subscriptionID uuid.UUID, graceUntilISO string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b814-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:grace:%s", subscriptionID.String(), graceUntilISO)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForOneOff generates key for one-off/gift purchases
// Format: v5({user_id}:oneoff:{product_id}:{timestamp})
func (g *IdempotencyService) GenerateForOneOff(userID, productID uuid.UUID, timestamp string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b815-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:oneoff:%s:%s", userID.String(), productID.String(), timestamp)
	return uuid.NewSHA1(namespace, []byte(data))
}

// GenerateForRefund generates key for refund operations
// Format: v5({payment_id}:refund:{timestamp})
func (g *IdempotencyService) GenerateForRefund(paymentID uuid.UUID, timestamp string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b816-9dad-11d1-80b4-00c04fd430c8")
	data := fmt.Sprintf("%s:refund:%s", paymentID.String(), timestamp)
	return uuid.NewSHA1(namespace, []byte(data))
}
