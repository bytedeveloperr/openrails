package repo

import (
	"context"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
)

type BillingAnalyticsRepo struct {
	db *db.DB
}

func NewBillingAnalyticsRepo(d *db.DB) *BillingAnalyticsRepo { return &BillingAnalyticsRepo{db: d} }

func (r *BillingAnalyticsRepo) CountActiveUsersWithoutAutoRenew(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusActive).
		Where("cancelled_at IS NOT NULL").
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithAutoRenew(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusActive).
		Where("processor IN (?)", []string{string(models.ProcessorCCBill), string(models.ProcessorNMI)}).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithFailingRebill(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusPastDue).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountDailySignups(ctx context.Context, startOfDay, endOfDay time.Time) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("created_at >= ?", startOfDay).
		Where("created_at < ?", endOfDay).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountDailyExplicitCancellations(ctx context.Context, startOfDay, endOfDay time.Time) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("cancelled_at >= ?", startOfDay).
		Where("cancelled_at < ?", endOfDay).
		Where("cancel_type IN (?)", []string{"user", "admin", "merchant"}).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountDailyFailedRebillCancellations(ctx context.Context, startOfDay, endOfDay time.Time) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("cancelled_at >= ?", startOfDay).
		Where("cancelled_at < ?", endOfDay).
		Where("cancel_type IN (?)", []string{"expired", "failed_payment"}).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithoutAutoRenewByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusActive).
		Where("cancelled_at IS NOT NULL").
		Where("processor = ?", processor).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithAutoRenewByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusActive).
		Where("processor = ?", processor).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithFailingRebillByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr(r.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusPastDue).
		Where("processor = ?", processor).
		Scan(ctx, &count)
	return count, err
}
