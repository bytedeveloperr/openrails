package services

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	log "github.com/sirupsen/logrus"
)

// BillingAnalyticsService provides dashboard metrics by querying PostgreSQL
type BillingAnalyticsService struct {
	db               *db.DB
	subscriptionRepo *SubscriptionService
}

// NewBillingAnalyticsService creates a new billing analytics service
func NewBillingAnalyticsService(db *db.DB) *BillingAnalyticsService {
	return &BillingAnalyticsService{
		db:               db,
		subscriptionRepo: NewSubscriptionService(db),
	}
}

// DashboardMetrics contains all the dashboard metrics
type DashboardMetrics struct {
	ActiveUsersWithoutAutoRenew  int64 `json:"active_users_without_auto_renew"`
	ActiveUsersWithAutoRenew     int64 `json:"active_users_with_auto_renew"`
	ActiveUsersWithFailingRebill int64 `json:"active_users_with_failing_rebill"`
}

// DailyMetrics contains daily counts for a specific date
type DailyMetrics struct {
	Date               time.Time `json:"date"`
	NewSignups         int64     `json:"new_signups"`
	ExplicitCancel     int64     `json:"explicit_cancellations"`
	FailedRebillCancel int64     `json:"failed_rebill_cancellations"`
}

// ProcessorMetrics contains metrics broken down by payment processor
type ProcessorMetrics struct {
	Processor                    string `json:"processor"`
	ActiveUsersWithoutAutoRenew  int64  `json:"active_users_without_auto_renew"`
	ActiveUsersWithAutoRenew     int64  `json:"active_users_with_auto_renew"`
	ActiveUsersWithFailingRebill int64  `json:"active_users_with_failing_rebill"`
}

// GetDashboardMetrics returns the 6 key dashboard metrics
func (s *BillingAnalyticsService) GetDashboardMetrics(ctx context.Context) (*DashboardMetrics, error) {
	metrics := &DashboardMetrics{}

	// Metric #1: Active users WITHOUT auto-renewal
	count, err := s.getActiveUsersWithoutAutoRenew(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active users without auto-renew: %w", err)
	}
	metrics.ActiveUsersWithoutAutoRenew = count

	// Metric #2: Active users WITH auto-renewal
	count, err = s.getActiveUsersWithAutoRenew(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active users with auto-renew: %w", err)
	}
	metrics.ActiveUsersWithAutoRenew = count

	// Metric #3: Active users with failing rebills (past_due status)
	count, err = s.getActiveUsersWithFailingRebill(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active users with failing rebill: %w", err)
	}
	metrics.ActiveUsersWithFailingRebill = count

	return metrics, nil
}

// GetDailyMetrics returns daily metrics for a specific date range
func (s *BillingAnalyticsService) GetDailyMetrics(ctx context.Context, startDate, endDate time.Time) ([]DailyMetrics, error) {
	var metrics []DailyMetrics

	// Get all dates in the range
	currentDate := startDate
	for !currentDate.After(endDate) {
		dailyMetric := DailyMetrics{
			Date: currentDate,
		}

		// Metric #4: Daily NEW signups
		count, err := s.getDailySignups(ctx, currentDate)
		if err != nil {
			return nil, fmt.Errorf("failed to get daily signups for %s: %w", currentDate.Format("2006-01-02"), err)
		}
		dailyMetric.NewSignups = count

		// Metric #5: Daily explicit cancellations
		count, err = s.getDailyExplicitCancellations(ctx, currentDate)
		if err != nil {
			return nil, fmt.Errorf("failed to get daily explicit cancellations for %s: %w", currentDate.Format("2006-01-02"), err)
		}
		dailyMetric.ExplicitCancel = count

		// Metric #6: Daily failed rebill cancellations
		count, err = s.getDailyFailedRebillCancellations(ctx, currentDate)
		if err != nil {
			return nil, fmt.Errorf("failed to get daily failed rebill cancellations for %s: %w", currentDate.Format("2006-01-02"), err)
		}
		dailyMetric.FailedRebillCancel = count

		metrics = append(metrics, dailyMetric)
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return metrics, nil
}

// GetMetricsByProcessor returns metrics broken down by payment processor
func (s *BillingAnalyticsService) GetMetricsByProcessor(ctx context.Context) ([]ProcessorMetrics, error) {
	var allMetrics []ProcessorMetrics

	processors := []string{"mobius", "ccbill"}

	for _, processor := range processors {
		metrics := ProcessorMetrics{
			Processor: processor,
		}

		// Get metrics for this processor
		count, err := s.getActiveUsersWithoutAutoRenewByProcessor(ctx, processor)
		if err != nil {
			return nil, fmt.Errorf("failed to get active users without auto-renew for %s: %w", processor, err)
		}
		metrics.ActiveUsersWithoutAutoRenew = count

		count, err = s.getActiveUsersWithAutoRenewByProcessor(ctx, processor)
		if err != nil {
			return nil, fmt.Errorf("failed to get active users with auto-renew for %s: %w", processor, err)
		}
		metrics.ActiveUsersWithAutoRenew = count

		count, err = s.getActiveUsersWithFailingRebillByProcessor(ctx, processor)
		if err != nil {
			return nil, fmt.Errorf("failed to get active users with failing rebill for %s: %w", processor, err)
		}
		metrics.ActiveUsersWithFailingRebill = count

		allMetrics = append(allMetrics, metrics)
	}

	return allMetrics, nil
}

// Metric #1: Active users WITHOUT auto-renewal
func (s *BillingAnalyticsService) getActiveUsersWithoutAutoRenew(ctx context.Context) (int64, error) {
	var count int64
	// No auto-renew: Solana payments are one-time (no subscriptions)
	// For subscriptions: users who cancelled but are still active until expiration
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusActive).
		Where("cancelled_at IS NOT NULL"). // Cancelled but still active until expiration
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).Error("Failed to count active users without auto-renew")
		return 0, err
	}

	return count, nil
}

// Metric #2: Active users WITH auto-renewal
func (s *BillingAnalyticsService) getActiveUsersWithAutoRenew(ctx context.Context) (int64, error) {
	var count int64
	// Auto-renew: CCBill and Mobius subscriptions auto-renew by default
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusActive).
		Where("processor IN (?)", []string{string(models.ProcessorCCBill), string(models.ProcessorMobius)}).
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).Error("Failed to count active users with auto-renew")
		return 0, err
	}

	return count, nil
}

// Metric #3: Active users with failing rebills (past_due status)
func (s *BillingAnalyticsService) getActiveUsersWithFailingRebill(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusPastDue).
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).Error("Failed to count active users with failing rebill")
		return 0, err
	}

	return count, nil
}

// Metric #4: Daily NEW signups
func (s *BillingAnalyticsService) getDailySignups(ctx context.Context, date time.Time) (int64, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var count int64
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("created_at >= ?", startOfDay).
		Where("created_at < ?", endOfDay).
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).WithField("date", date).Error("Failed to count daily signups")
		return 0, err
	}

	return count, nil
}

// Metric #5: Daily explicit cancellations
func (s *BillingAnalyticsService) getDailyExplicitCancellations(ctx context.Context, date time.Time) (int64, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var count int64
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("cancelled_at >= ?", startOfDay).
		Where("cancelled_at < ?", endOfDay).
		Where("cancel_type IN (?)", []string{"user", "admin", "merchant"}).
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).WithField("date", date).Error("Failed to count daily explicit cancellations")
		return 0, err
	}

	return count, nil
}

// Metric #6: Daily failed rebill cancellations
func (s *BillingAnalyticsService) getDailyFailedRebillCancellations(ctx context.Context, date time.Time) (int64, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var count int64
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("cancelled_at >= ?", startOfDay).
		Where("cancelled_at < ?", endOfDay).
		Where("cancel_type IN (?)", []string{"expired", "failed_payment"}).
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).WithField("date", date).Error("Failed to count daily failed rebill cancellations")
		return 0, err
	}

	return count, nil
}

// Processor-specific metrics

func (s *BillingAnalyticsService) getActiveUsersWithoutAutoRenewByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	// No auto-renew by processor: cancelled but still active subscriptions
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusActive).
		Where("processor = ?", processor).
		Where("cancelled_at IS NOT NULL"). // Cancelled but still active until expiration
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).WithField("processor", processor).Error("Failed to count active users without auto-renew by processor")
		return 0, err
	}

	return count, nil
}

func (s *BillingAnalyticsService) getActiveUsersWithAutoRenewByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	// Auto-renew by processor: CCBill and Mobius auto-renew by default
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusActive).
		Where("processor = ?", processor).
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).WithField("processor", processor).Error("Failed to count active users with auto-renew by processor")
		return 0, err
	}

	return count, nil
}

func (s *BillingAnalyticsService) getActiveUsersWithFailingRebillByProcessor(ctx context.Context, processor string) (int64, error) {
	var count int64
	err := s.db.GetDB().NewSelect().ColumnExpr("COUNT(*)").
		TableExpr(s.db.QualifiedTable("subscriptions")).
		Where("status = ?", models.StatusPastDue).
		Where("processor = ?", processor).
		Scan(ctx, &count)

	if err != nil {
		log.WithError(err).WithField("processor", processor).Error("Failed to count active users with failing rebill by processor")
		return 0, err
	}

	return count, nil
}
