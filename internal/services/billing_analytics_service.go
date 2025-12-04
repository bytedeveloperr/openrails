package services

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/doujins-org/doujins-billing/internal/processors"
	log "github.com/sirupsen/logrus"
)

// BillingAnalyticsService provides dashboard metrics by querying PostgreSQL
type BillingAnalyticsService struct {
	repo *repo.BillingAnalyticsRepo
}

// NewBillingAnalyticsService creates a new billing analytics service
func NewBillingAnalyticsService(db *db.DB) *BillingAnalyticsService {
	return &BillingAnalyticsService{repo: repo.NewBillingAnalyticsRepo(db)}
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

	count, err := s.repo.CountActiveUsersWithoutAutoRenew(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to count active users without auto-renew")
		return nil, fmt.Errorf("failed to get active users without auto-renew: %w", err)
	}
	metrics.ActiveUsersWithoutAutoRenew = count

	count, err = s.repo.CountActiveUsersWithAutoRenew(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to count active users with auto-renew")
		return nil, fmt.Errorf("failed to get active users with auto-renew: %w", err)
	}
	metrics.ActiveUsersWithAutoRenew = count

	count, err = s.repo.CountActiveUsersWithFailingRebill(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to count active users with failing rebill")
		return nil, fmt.Errorf("failed to get active users with failing rebill: %w", err)
	}
	metrics.ActiveUsersWithFailingRebill = count

	return metrics, nil
}

// GetDailyMetrics returns daily metrics for a specific date range
func (s *BillingAnalyticsService) GetDailyMetrics(ctx context.Context, startDate, endDate time.Time) ([]DailyMetrics, error) {
	var metrics []DailyMetrics

	currentDate := startDate
	for !currentDate.After(endDate) {
		dailyMetric := DailyMetrics{Date: currentDate}
		startOfDay := time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 0, 0, 0, 0, currentDate.Location())
		endOfDay := startOfDay.Add(24 * time.Hour)

		count, err := s.repo.CountDailySignups(ctx, startOfDay, endOfDay)
		if err != nil {
			log.WithError(err).WithField("date", currentDate).Error("Failed to count daily signups")
			return nil, fmt.Errorf("failed to get daily signups for %s: %w", currentDate.Format("2006-01-02"), err)
		}
		dailyMetric.NewSignups = count

		count, err = s.repo.CountDailyExplicitCancellations(ctx, startOfDay, endOfDay)
		if err != nil {
			log.WithError(err).WithField("date", currentDate).Error("Failed to count daily explicit cancellations")
			return nil, fmt.Errorf("failed to get daily explicit cancellations for %s: %w", currentDate.Format("2006-01-02"), err)
		}
		dailyMetric.ExplicitCancel = count

		count, err = s.repo.CountDailyFailedRebillCancellations(ctx, startOfDay, endOfDay)
		if err != nil {
			log.WithError(err).WithField("date", currentDate).Error("Failed to count daily failed rebill cancellations")
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

	// Include all NMI-backed processors and CCBill
	processorList := append(processors.GetNMIBackedProcessorsList(), string(models.ProcessorCCBill))

	for _, proc := range processorList {
		metrics := ProcessorMetrics{Processor: proc}

		count, err := s.repo.CountActiveUsersWithoutAutoRenewByProcessor(ctx, proc)
		if err != nil {
			return nil, fmt.Errorf("failed to get active users without auto-renew for %s: %w", proc, err)
		}
		metrics.ActiveUsersWithoutAutoRenew = count

		count, err = s.repo.CountActiveUsersWithAutoRenewByProcessor(ctx, proc)
		if err != nil {
			return nil, fmt.Errorf("failed to get active users with auto-renew for %s: %w", proc, err)
		}
		metrics.ActiveUsersWithAutoRenew = count

		count, err = s.repo.CountActiveUsersWithFailingRebillByProcessor(ctx, proc)
		if err != nil {
			return nil, fmt.Errorf("failed to get active users with failing rebill for %s: %w", proc, err)
		}
		metrics.ActiveUsersWithFailingRebill = count

		allMetrics = append(allMetrics, metrics)
	}

	return allMetrics, nil
}
