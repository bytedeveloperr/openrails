package services

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type EntitlementService struct{ db *db.DB }

func NewEntitlementService(db *db.DB) *EntitlementService { return &EntitlementService{db: db} }
func (s *EntitlementService) GetDB() *db.DB               { return s.db }

// IsEntitled returns true if the user currently has an active entitlement
func (s *EntitlementService) IsEntitled(ctx context.Context, userID, entitlement string, at time.Time) (bool, error) {
	q := s.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		Where("user_id = ?", userID).
		Where("entitlement = ?", entitlement).
		Where("start_at <= ?", at).
		Where("(end_at IS NULL OR end_at > ?)", at).
		Where("revoked_at IS NULL")
	exists, err := q.Exists(ctx)
	return exists, err
}

// GetByUser returns all entitlements for a user (optionally filter by entitlement)
// AppendEntitlementDays appends a finite window of N days right after the latest window's end,
// unless the user currently has an indefinite window (end_at IS NULL). In that case, it is a no-op.
func (s *EntitlementService) AppendEntitlementDays(ctx context.Context, userID, entitlement string, days int, sourceType models.EntitlementSourceType, sourceID *uuid.UUID) (*models.Entitlement, error) {
	if days <= 0 {
		return nil, fmt.Errorf("days must be > 0")
	}
	now := time.Now()
	// If an active indefinite window exists, do nothing.
	{
		exists, err := s.db.GetDB().NewSelect().Model((*models.Entitlement)(nil)).
			Where("user_id = ? AND entitlement = ?", userID, entitlement).
			Where("revoked_at IS NULL AND end_at IS NULL").
			Where("start_at <= ?", now).
			Exists(ctx)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("cannot append entitlement while subscription entitlement is active")
		}
	}
	// Get most recent non-revoked window to compute adjacency
	var last models.Entitlement
	_ = s.db.GetDB().NewSelect().Model(&last).
		Where("user_id = ? AND entitlement = ?", userID, entitlement).
		Where("revoked_at IS NULL").
		Order("start_at DESC").
		Limit(1).
		Scan(ctx)
	start := now
	if last.ID != uuid.Nil && last.EndAt != nil {
		start = *last.EndAt
	}
	end := start.Add(time.Duration(days) * 24 * time.Hour)
	return s.GrantWindow(ctx, userID, entitlement, start, &end, sourceType, sourceID)
}

// ListActiveEntitlements returns a de-duplicated list of active entitlement names for a user at a point in time.
func (s *EntitlementService) ListActiveEntitlements(ctx context.Context, userID string, at time.Time) ([]string, error) {
	var out []string
	q := s.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		ColumnExpr("DISTINCT ent.entitlement").
		Where("user_id = ?", userID).
		Where("start_at <= ?", at).
		Where("(end_at IS NULL OR end_at > ?)", at).
		Where("revoked_at IS NULL")
	if err := q.Scan(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GrantWindow creates a new entitlement window for a user
func (s *EntitlementService) GrantWindow(ctx context.Context, userID, entitlement string, startAt time.Time, endAt *time.Time, sourceType models.EntitlementSourceType, sourceID *uuid.UUID) (*models.Entitlement, error) {
	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: entitlement,
		StartAt:     startAt,
		EndAt:       endAt,
		SourceType:  sourceType,
		SourceID:    sourceID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if _, err := s.db.GetDB().NewInsert().Model(ent).Exec(ctx); err != nil {
		return nil, err
	}
	return ent, nil
}

// EndActiveBySubscription ends active entitlements for a subscription at a given time
func (s *EntitlementService) EndActiveBySubscription(ctx context.Context, subscriptionID uuid.UUID, endAt time.Time, reason *models.EntitlementRevokeReason) error {
	now := time.Now()
	_, err := s.db.GetDB().NewUpdate().
		Model((*models.Entitlement)(nil)).
		Set("end_at = ?", endAt).
		Set("revoked_at = ?", now).
		Set("revoke_reason = ?", reason).
		Set("updated_at = ?", now).
		Where("source_type = ?", models.EntitlementSourceSubscription).
		Where("source_id = ?", subscriptionID).
		Where("end_at IS NULL").
		Exec(ctx)
	return err
}

// EndActiveByPayment ends active entitlements for a one-off payment at a given time
func (s *EntitlementService) EndActiveByPayment(ctx context.Context, paymentID uuid.UUID, endAt time.Time, reason *models.EntitlementRevokeReason) error {
	now := time.Now()
	_, err := s.db.GetDB().NewUpdate().
		Model((*models.Entitlement)(nil)).
		Set("end_at = ?", endAt).
		Set("revoked_at = ?", now).
		Set("revoke_reason = ?", reason).
		Set("updated_at = ?", now).
		Where("source_type = ?", models.EntitlementSourceOneOff).
		Where("source_id = ?", paymentID).
		Where("end_at IS NULL").
		Exec(ctx)
	return err
}
