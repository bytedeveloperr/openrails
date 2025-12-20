package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/processors"
	"github.com/uptrace/bun"
)

type BillingAnalyticsRepo struct {
	db *db.DB
}

func NewBillingAnalyticsRepo(d *db.DB) *BillingAnalyticsRepo { return &BillingAnalyticsRepo{db: d} }

func (r *BillingAnalyticsRepo) CountActiveUsersWithoutAutoRenew(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusActive).
		Where("sub.cancelled_at IS NOT NULL").
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithAutoRenew(ctx context.Context) (int64, error) {
	// Auto-renew processors: CCBill and all NMI-backed processors (e.g., mobius)
	autoRenewProcessors := append(processors.GetNMIBackedProcessorsList(), string(models.ProcessorCCBill))
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusActive).
		Where("sub.processor IN (?)", bun.In(autoRenewProcessors)).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithFailingRebill(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusPastDue).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountDailySignups(ctx context.Context, startOfDay, endOfDay time.Time) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.created_at >= ?", startOfDay).
		Where("sub.created_at < ?", endOfDay).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountDailyExplicitCancellations(ctx context.Context, startOfDay, endOfDay time.Time) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.cancelled_at >= ?", startOfDay).
		Where("sub.cancelled_at < ?", endOfDay).
		Where("sub.cancel_type IN (?)", []string{"user", "admin", "merchant"}).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountDailyFailedRebillCancellations(ctx context.Context, startOfDay, endOfDay time.Time) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.cancelled_at >= ?", startOfDay).
		Where("sub.cancelled_at < ?", endOfDay).
		Where("sub.cancel_type IN (?)", []string{"expired", "failed_payment"}).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithoutAutoRenewByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusActive).
		Where("sub.cancelled_at IS NOT NULL").
		Where("sub.processor = ?", processor).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithAutoRenewByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusActive).
		Where("sub.processor = ?", processor).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) CountActiveUsersWithFailingRebillByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		ColumnExpr("COUNT(*)").
		Where("sub.status = ?", models.StatusPastDue).
		Where("sub.processor = ?", processor).
		Scan(ctx, &count)
	return count, err
}

func (r *BillingAnalyticsRepo) GetSnapshotsBetween(ctx context.Context, startDate, endDate time.Time) ([]models.DailyMetricsSnapshot, error) {
	var snapshots []models.DailyMetricsSnapshot
	err := r.db.GetDB().NewSelect().
		Model(&snapshots).
		Where("dms.snapshot_date >= ?", startDate.UTC().Truncate(24*time.Hour)).
		Where("dms.snapshot_date <= ?", endDate.UTC().Truncate(24*time.Hour)).
		OrderExpr("dms.snapshot_date ASC").
		Scan(ctx)
	return snapshots, err
}

func (r *BillingAnalyticsRepo) GetSnapshotByDate(ctx context.Context, date time.Time) (*models.DailyMetricsSnapshot, error) {
	snapshot := new(models.DailyMetricsSnapshot)
	err := r.db.GetDB().NewSelect().
		Model(snapshot).
		Where("dms.snapshot_date = ?", date.UTC().Truncate(24*time.Hour)).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *BillingAnalyticsRepo) UpsertSnapshot(ctx context.Context, snapshot *models.DailyMetricsSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	snapshot.UpdatedAt = time.Now().UTC()
	_, err := r.db.GetDB().NewInsert().
		Model(snapshot).
		On("CONFLICT (snapshot_date) DO UPDATE").
		Set("currency = EXCLUDED.currency").
		Set("mrr_cents = EXCLUDED.mrr_cents").
		Set("subscription_revenue_cents = EXCLUDED.subscription_revenue_cents").
		Set("one_time_revenue_cents = EXCLUDED.one_time_revenue_cents").
		Set("refunds_cents = EXCLUDED.refunds_cents").
		Set("chargebacks_cents = EXCLUDED.chargebacks_cents").
		Set("new_subscriptions = EXCLUDED.new_subscriptions").
		Set("scheduled_starts = EXCLUDED.scheduled_starts").
		Set("cancellations_voluntary = EXCLUDED.cancellations_voluntary").
		Set("cancellations_involuntary = EXCLUDED.cancellations_involuntary").
		Set("reactivations = EXCLUDED.reactivations").
		Set("active_count_end = EXCLUDED.active_count_end").
		Set("past_due_count_end = EXCLUDED.past_due_count_end").
		Set("pending_count_end = EXCLUDED.pending_count_end").
		Set("entitlements_granted = EXCLUDED.entitlements_granted").
		Set("processor_breakdowns = EXCLUDED.processor_breakdowns").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}
