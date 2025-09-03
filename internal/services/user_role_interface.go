package services

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// UserRoleInterfaceService provides the proper SQL interface for billing ↔ roles integration
// This service ONLY calls the SECURITY DEFINER functions and never directly manipulates user_roles
type UserRoleInterfaceService struct {
	db *db.DB
}

// UserRoleAction represents the action returned by the SQL functions
type UserRoleAction string

const (
	ActionInsert     UserRoleAction = "insert"
	ActionMerged     UserRoleAction = "merged"
	ActionExtended   UserRoleAction = "extended"
	ActionClosed     UserRoleAction = "closed"
	ActionIdempotent UserRoleAction = "idempotent"
	ActionNoop       UserRoleAction = "noop"
	ActionUpserted   UserRoleAction = "upserted"
	ActionDeleted    UserRoleAction = "deleted"
)

// UserRoleResult represents the result from SQL function calls
type UserRoleResult struct {
	UserRoleID uuid.UUID      `bun:"user_role_id"`
	Action     UserRoleAction `bun:"action"`
}

// IdempotencyKeyGenerator generates proper idempotency keys following the pattern
type IdempotencyKeyGenerator struct{}

// NewIdempotencyKeyGenerator creates a new idempotency key generator
func NewIdempotencyKeyGenerator() *IdempotencyKeyGenerator {
	return &IdempotencyKeyGenerator{}
}

// ForChargeSuccess generates idempotency key for charge success: v5({processor}:{transaction_id})
func (g *IdempotencyKeyGenerator) ForChargeSuccess(processor, transactionID string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // v1 namespace for billing
	return uuid.NewSHA1(namespace, []byte(fmt.Sprintf("%s:%s", processor, transactionID)))
}

// ForRenewal generates idempotency key for renewal: v5({subscription_id}:{period_end_iso})
func (g *IdempotencyKeyGenerator) ForRenewal(subscriptionID uuid.UUID, periodEnd time.Time) uuid.UUID {
	namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	return uuid.NewSHA1(namespace, []byte(fmt.Sprintf("%s:%s", subscriptionID.String(), periodEnd.Format(time.RFC3339))))
}

// ForCancelNow generates idempotency key for cancel now: v5({subscription_id}:cancel:{event_id})
func (g *IdempotencyKeyGenerator) ForCancelNow(subscriptionID uuid.UUID, eventID string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	return uuid.NewSHA1(namespace, []byte(fmt.Sprintf("%s:cancel:%s", subscriptionID.String(), eventID)))
}

// ForTrialStart generates idempotency key for trial start: v5({subscription_id}:trial:{trial_end_iso})
func (g *IdempotencyKeyGenerator) ForTrialStart(subscriptionID uuid.UUID, trialEnd time.Time) uuid.UUID {
	namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	return uuid.NewSHA1(namespace, []byte(fmt.Sprintf("%s:trial:%s", subscriptionID.String(), trialEnd.Format(time.RFC3339))))
}

// NewUserRoleInterfaceService creates a new interface service
func NewUserRoleInterfaceService(database *db.DB) *UserRoleInterfaceService {
	return &UserRoleInterfaceService{
		db: database,
	}
}

// OpenOrEnsureUserRole calls billing_open_or_ensure_user_role SECURITY DEFINER function
func (s *UserRoleInterfaceService) OpenOrEnsureUserRole(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	startAt *time.Time,
	endAt *time.Time,
	sourceType string,
	sourceID *uuid.UUID,
	idempotencyKey uuid.UUID,
) (*UserRoleResult, error) {
	var result UserRoleResult
	
	query := `SELECT * FROM billing_open_or_ensure_user_role($1, $2, $3, $4, $5, $6, $7)`
	
	err := s.db.GetDB().NewRaw(query, 
		userID, 
		roleSlug, 
		startAt, 
		endAt, 
		sourceType, 
		sourceID, 
		idempotencyKey,
	).Scan(ctx, &result)
	
	if err != nil {
		return nil, fmt.Errorf("failed to open/ensure user role: %w", err)
	}
	
	log.WithFields(log.Fields{
		"user_id":         userID,
		"role_slug":       roleSlug,
		"action":          result.Action,
		"user_role_id":    result.UserRoleID,
		"idempotency_key": idempotencyKey,
	}).Info("User role opened/ensured")
	
	return &result, nil
}

// ExtendUserRole calls billing_extend_user_role SECURITY DEFINER function
func (s *UserRoleInterfaceService) ExtendUserRole(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	newEndAt *time.Time,
	idempotencyKey uuid.UUID,
) (*UserRoleResult, error) {
	var result UserRoleResult
	
	query := `SELECT * FROM billing_extend_user_role($1, $2, $3, $4)`
	
	err := s.db.GetDB().NewRaw(query,
		userID,
		roleSlug,
		newEndAt,
		idempotencyKey,
	).Scan(ctx, &result)
	
	if err != nil {
		return nil, fmt.Errorf("failed to extend user role: %w", err)
	}
	
	log.WithFields(log.Fields{
		"user_id":         userID,
		"role_slug":       roleSlug,
		"new_end_at":      newEndAt,
		"action":          result.Action,
		"user_role_id":    result.UserRoleID,
		"idempotency_key": idempotencyKey,
	}).Info("User role extended")
	
	return &result, nil
}

// CloseUserRole calls billing_close_user_role SECURITY DEFINER function
func (s *UserRoleInterfaceService) CloseUserRole(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	effectiveAt *time.Time,
	revokeReason *string,
	idempotencyKey uuid.UUID,
) (*UserRoleResult, error) {
	var result UserRoleResult
	
	query := `SELECT * FROM billing_close_user_role($1, $2, $3, $4, $5)`
	
	err := s.db.GetDB().NewRaw(query,
		userID,
		roleSlug,
		effectiveAt,
		revokeReason,
		idempotencyKey,
	).Scan(ctx, &result)
	
	if err != nil {
		return nil, fmt.Errorf("failed to close user role: %w", err)
	}
	
	log.WithFields(log.Fields{
		"user_id":         userID,
		"role_slug":       roleSlug,
		"effective_at":    effectiveAt,
		"revoke_reason":   revokeReason,
		"action":          result.Action,
		"user_role_id":    result.UserRoleID,
		"idempotency_key": idempotencyKey,
	}).Info("User role closed")
	
	return &result, nil
}

// IsActionSuccess checks if the action indicates success (not an error)
func (r *UserRoleResult) IsActionSuccess() bool {
	return r.Action == ActionInsert ||
		r.Action == ActionMerged ||
		r.Action == ActionExtended ||
		r.Action == ActionClosed ||
		r.Action == ActionIdempotent ||
		r.Action == ActionNoop ||
		r.Action == ActionUpserted ||
		r.Action == ActionDeleted
}

// Billing Lifecycle Methods - These implement the specific patterns from the manual

// HandleNewSubscriptionOrTrial opens/ensures a role window for a new subscription or trial
func (s *UserRoleInterfaceService) HandleNewSubscriptionOrTrial(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	periodStart, periodEnd time.Time,
	subscriptionID uuid.UUID,
) (*UserRoleResult, error) {
	keyGen := NewIdempotencyKeyGenerator()
	idempotencyKey := keyGen.ForTrialStart(subscriptionID, periodEnd)
	
	return s.OpenOrEnsureUserRole(
		ctx,
		userID,
		roleSlug,
		&periodStart,
		&periodEnd,
		"subscription",
		&subscriptionID,
		idempotencyKey,
	)
}

// HandleRenewalSucceeded extends the role to the next period end
func (s *UserRoleInterfaceService) HandleRenewalSucceeded(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	nextPeriodEnd time.Time,
	subscriptionID uuid.UUID,
) (*UserRoleResult, error) {
	keyGen := NewIdempotencyKeyGenerator()
	idempotencyKey := keyGen.ForRenewal(subscriptionID, nextPeriodEnd)
	
	return s.ExtendUserRole(
		ctx,
		userID,
		roleSlug,
		&nextPeriodEnd,
		idempotencyKey,
	)
}

// HandleCancelAtPeriodEnd closes the role at period end
func (s *UserRoleInterfaceService) HandleCancelAtPeriodEnd(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	periodEnd time.Time,
	subscriptionID uuid.UUID,
	eventID string,
) (*UserRoleResult, error) {
	keyGen := NewIdempotencyKeyGenerator()
	idempotencyKey := keyGen.ForCancelNow(subscriptionID, eventID)
	
	return s.CloseUserRole(
		ctx,
		userID,
		roleSlug,
		&periodEnd,
		nil, // no revoke reason for end-of-period cancellation
		idempotencyKey,
	)
}

// HandleImmediateCancelOrRefund closes the role immediately
func (s *UserRoleInterfaceService) HandleImmediateCancelOrRefund(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	subscriptionID uuid.UUID,
	eventID string,
	revokeReason string,
) (*UserRoleResult, error) {
	keyGen := NewIdempotencyKeyGenerator()
	idempotencyKey := keyGen.ForCancelNow(subscriptionID, eventID)
	now := time.Now()
	
	return s.CloseUserRole(
		ctx,
		userID,
		roleSlug,
		&now,
		&revokeReason,
		idempotencyKey,
	)
}

// HandleGracePeriod opens/ensures a role with grace period until specified time
func (s *UserRoleInterfaceService) HandleGracePeriod(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	graceUntil time.Time,
	subscriptionID uuid.UUID,
) (*UserRoleResult, error) {
	keyGen := NewIdempotencyKeyGenerator()
	idempotencyKey := keyGen.ForRenewal(subscriptionID, graceUntil)
	now := time.Now()
	
	return s.OpenOrEnsureUserRole(
		ctx,
		userID,
		roleSlug,
		&now,
		&graceUntil,
		"subscription",
		&subscriptionID,
		idempotencyKey,
	)
}

// HandleOneOffOrGift opens/ensures a role for one-off purchases or gifts
func (s *UserRoleInterfaceService) HandleOneOffOrGift(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	periodStart, periodEnd time.Time,
	sourceID uuid.UUID,
	processor, transactionID string,
) (*UserRoleResult, error) {
	keyGen := NewIdempotencyKeyGenerator()
	idempotencyKey := keyGen.ForChargeSuccess(processor, transactionID)
	
	return s.OpenOrEnsureUserRole(
		ctx,
		userID,
		roleSlug,
		&periodStart,
		&periodEnd,
		"one_off",
		&sourceID,
		idempotencyKey,
	)
}

// HandleChargeSuccess handles successful payment charges
func (s *UserRoleInterfaceService) HandleChargeSuccess(
	ctx context.Context,
	userID uuid.UUID,
	roleSlug string,
	periodStart, periodEnd time.Time,
	subscriptionID uuid.UUID,
	processor, transactionID string,
) (*UserRoleResult, error) {
	keyGen := NewIdempotencyKeyGenerator()
	idempotencyKey := keyGen.ForChargeSuccess(processor, transactionID)
	
	return s.OpenOrEnsureUserRole(
		ctx,
		userID,
		roleSlug,
		&periodStart,
		&periodEnd,
		"subscription",
		&subscriptionID,
		idempotencyKey,
	)
}