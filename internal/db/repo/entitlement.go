package repo

import (
	"context"
	"errors"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type EntitlementRepo struct {
	db *db.DB
}

func NewEntitlementRepo(d *db.DB) *EntitlementRepo { return &EntitlementRepo{db: d} }

func (r *EntitlementRepo) IsEntitled(ctx context.Context, userID, entitlement string, at time.Time) (bool, error) {
	q := r.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		TableExpr(r.db.QualifiedTable("entitlements")).
		Where("user_id = ?", userID).
		Where("entitlement = ?", entitlement).
		Where("start_at <= ?", at).
		Where("(end_at IS NULL OR end_at > ?)", at).
		Where("revoked_at IS NULL")
	return q.Exists(ctx)
}

func (r *EntitlementRepo) HasActiveIndefinite(ctx context.Context, userID, entitlement string, at time.Time) (bool, error) {
	q := r.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		TableExpr(r.db.QualifiedTable("entitlements")).
		Where("user_id = ?", userID).
		Where("entitlement = ?", entitlement).
		Where("revoked_at IS NULL AND end_at IS NULL").
		Where("start_at <= ?", at)
	return q.Exists(ctx)
}

func (r *EntitlementRepo) GetLatestActive(ctx context.Context, userID, entitlement string) (*models.Entitlement, error) {
	var ent models.Entitlement
	err := r.db.GetDB().NewSelect().
		Model(&ent).
		TableExpr(r.db.QualifiedTable("entitlements")).
		Where("user_id = ? AND entitlement = ?", userID, entitlement).
		Where("revoked_at IS NULL").
		Order("start_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &ent, nil
}

func (r *EntitlementRepo) GetLatestFiniteActive(ctx context.Context, userID, entitlement string, at time.Time) (*models.Entitlement, error) {
	var ent models.Entitlement
	err := r.db.GetDB().NewSelect().
		Model(&ent).
		TableExpr(r.db.QualifiedTable("entitlements")).
		Where("user_id = ? AND entitlement = ?", userID, entitlement).
		Where("revoked_at IS NULL").
		Where("end_at IS NOT NULL").
		Where("start_at <= ?", at).
		Where("end_at > ?", at).
		Order("end_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &ent, nil
}

func (r *EntitlementRepo) Insert(ctx context.Context, entitlement *models.Entitlement) error {
	res, err := r.db.GetDB().NewInsert().Model(entitlement).TableExpr(r.db.QualifiedTable("entitlements")).Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *EntitlementRepo) ListActiveEntitlements(ctx context.Context, userID string, at time.Time) ([]string, error) {
	var out []string
	if err := r.db.GetDB().NewSelect().
		TableExpr(r.db.QualifiedTable("entitlements")+" AS ent").
		ColumnExpr("DISTINCT ent.entitlement").
		Where("user_id = ?", userID).
		Where("start_at <= ?", at).
		Where("(end_at IS NULL OR end_at > ?)", at).
		Where("revoked_at IS NULL").
		Scan(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *EntitlementRepo) ListActiveRecords(ctx context.Context, userID string, at time.Time) ([]models.Entitlement, error) {
	ents := []models.Entitlement{}
	if err := r.db.GetDB().NewSelect().
		Model(&ents).
		TableExpr(r.db.QualifiedTable("entitlements")).
		Where("user_id = ?", userID).
		Where("revoked_at IS NULL").
		Where("start_at <= ?", at).
		Where("(end_at IS NULL OR end_at > ?)", at).
		Order("start_at ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return ents, nil
}

func (r *EntitlementRepo) EndActiveBySubscription(ctx context.Context, subscriptionID uuid.UUID, endAt time.Time, reason *models.EntitlementRevokeReason) error {
	now := time.Now()
	_, err := r.db.GetDB().NewUpdate().
		Model((*models.Entitlement)(nil)).
		TableExpr(r.db.QualifiedTable("entitlements")).
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

func (r *EntitlementRepo) EndActiveByPayment(ctx context.Context, paymentID uuid.UUID, endAt time.Time, reason *models.EntitlementRevokeReason) error {
	now := time.Now()
	_, err := r.db.GetDB().NewUpdate().
		Model((*models.Entitlement)(nil)).
		TableExpr(r.db.QualifiedTable("entitlements")).
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

func (r *EntitlementRepo) ExistsBySource(ctx context.Context, sourceType models.EntitlementSourceType, sourceID uuid.UUID, entitlement string) (bool, error) {
	return r.db.GetDB().NewSelect().
		Model((*models.Entitlement)(nil)).
		TableExpr(r.db.QualifiedTable("entitlements")).
		Where("source_type = ?", sourceType).
		Where("source_id = ?", sourceID).
		Where("entitlement = ?", entitlement).
		Where("revoked_at IS NULL").
		Exists(ctx)
}

func (r *EntitlementRepo) ListByUser(ctx context.Context, userID string) ([]models.Entitlement, error) {
	ents := []models.Entitlement{}
	if err := r.db.GetDB().NewSelect().
		Model(&ents).
		TableExpr(r.db.QualifiedTable("entitlements")).
		Where("user_id = ?", userID).
		Order("start_at DESC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return ents, nil
}
