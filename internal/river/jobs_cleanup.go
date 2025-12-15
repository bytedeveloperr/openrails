package riverjobs

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/jonboulle/clockwork"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"
)

const KindCleanupExpiredData = "billing.cleanup_expired_data"

// CleanupConfig defines retention periods for various data types
type CleanupConfig struct {
	// WalletChallengeRetention is how long to keep expired wallet challenges
	// Default: 24 hours (challenges expire after 5 minutes, keep 24h for debugging)
	WalletChallengeRetention time.Duration

	// PaymentIntentRetention is how long to keep expired/failed payment intents
	// Default: 7 days
	PaymentIntentRetention time.Duration

	// SolanaTransactionRetention is how long to keep expired/failed Solana transactions
	// Default: 30 days
	SolanaTransactionRetention time.Duration

	// NotificationSeenRetention is how long to keep seen notifications
	// Default: 90 days (matches model's IsExpiredForCleanup)
	NotificationSeenRetention time.Duration

	// NotificationUnseenRetention is how long to keep unseen notifications
	// Default: 180 days (matches model's IsExpiredForCleanup)
	NotificationUnseenRetention time.Duration

	// WebhookEventRetention is how long to keep processed webhook events
	// Default: 30 days
	WebhookEventRetention time.Duration
}

// DefaultCleanupConfig returns sensible default retention periods
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		WalletChallengeRetention:    24 * time.Hour,
		PaymentIntentRetention:      7 * 24 * time.Hour,
		SolanaTransactionRetention:  30 * 24 * time.Hour,
		NotificationSeenRetention:   90 * 24 * time.Hour,
		NotificationUnseenRetention: 180 * 24 * time.Hour,
		WebhookEventRetention:       30 * 24 * time.Hour,
	}
}

type CleanupExpiredDataArgs struct{}

func (CleanupExpiredDataArgs) Kind() string { return KindCleanupExpiredData }

type CleanupExpiredDataWorker struct {
	river.WorkerDefaults[CleanupExpiredDataArgs]
	DB     *db.DB
	Clock  clockwork.Clock
	Config CleanupConfig
}

func (CleanupExpiredDataWorker) Kind() string { return KindCleanupExpiredData }

// CleanupResult holds the count of deleted records per table
type CleanupResult struct {
	WalletChallenges   int64
	PaymentIntents     int64
	SolanaTransactions int64
	NotificationsSeen  int64
	NotificationsAll   int64
	WebhookEvents      int64
}

func (w CleanupExpiredDataWorker) Work(ctx context.Context, job *river.Job[CleanupExpiredDataArgs]) error {
	if w.DB == nil {
		return fmt.Errorf("db is required")
	}

	clock := w.Clock
	if clock == nil {
		clock = clockwork.NewRealClock()
	}

	config := w.Config
	if config.WalletChallengeRetention == 0 {
		config = DefaultCleanupConfig()
	}

	now := clock.Now()
	result := CleanupResult{}
	var err error

	logger := log.WithContext(ctx).WithField("worker", KindCleanupExpiredData)
	logger.Info("Starting cleanup of expired data")

	// 1. Clean up expired wallet challenges
	result.WalletChallenges, err = w.cleanupWalletChallenges(ctx, now, config.WalletChallengeRetention)
	if err != nil {
		logger.WithError(err).Error("Failed to cleanup wallet challenges")
	}

	// 2. Clean up expired/failed payment intents
	result.PaymentIntents, err = w.cleanupPaymentIntents(ctx, now, config.PaymentIntentRetention)
	if err != nil {
		logger.WithError(err).Error("Failed to cleanup payment intents")
	}

	// 3. Clean up expired/failed Solana transactions
	result.SolanaTransactions, err = w.cleanupSolanaTransactions(ctx, now, config.SolanaTransactionRetention)
	if err != nil {
		logger.WithError(err).Error("Failed to cleanup Solana transactions")
	}

	// 4. Clean up old notifications (seen ones first with shorter retention)
	result.NotificationsSeen, err = w.cleanupSeenNotifications(ctx, now, config.NotificationSeenRetention)
	if err != nil {
		logger.WithError(err).Error("Failed to cleanup seen notifications")
	}

	result.NotificationsAll, err = w.cleanupOldNotifications(ctx, now, config.NotificationUnseenRetention)
	if err != nil {
		logger.WithError(err).Error("Failed to cleanup old notifications")
	}

	// 5. Clean up old processed webhook events
	result.WebhookEvents, err = w.cleanupWebhookEvents(ctx, now, config.WebhookEventRetention)
	if err != nil {
		logger.WithError(err).Error("Failed to cleanup webhook events")
	}

	logger.WithFields(log.Fields{
		"wallet_challenges":    result.WalletChallenges,
		"payment_intents":      result.PaymentIntents,
		"solana_transactions":  result.SolanaTransactions,
		"notifications_seen":   result.NotificationsSeen,
		"notifications_unseen": result.NotificationsAll,
		"webhook_events":       result.WebhookEvents,
	}).Info("Cleanup completed")

	return nil
}

// cleanupWalletChallenges deletes wallet challenges that have expired beyond the retention period
func (w CleanupExpiredDataWorker) cleanupWalletChallenges(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	cutoff := now.Add(-retention)

	res, err := w.DB.GetDB().NewDelete().
		TableExpr("billing.solana_wallet_challenges").
		Where("expires_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete wallet challenges: %w", err)
	}

	rows, _ := res.RowsAffected()
	return rows, nil
}

// cleanupPaymentIntents deletes payment intents that are expired or failed beyond the retention period
func (w CleanupExpiredDataWorker) cleanupPaymentIntents(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	cutoff := now.Add(-retention)

	// Delete payment intents that are either:
	// - expired (expires_at < cutoff)
	// - failed and older than retention period
	// - pending and older than retention period (abandoned)
	res, err := w.DB.GetDB().NewDelete().
		TableExpr("billing.solana_payment_intents").
		Where("(expires_at IS NOT NULL AND expires_at < ?) OR (status IN ('failed', 'pending') AND created_at < ?)", cutoff, cutoff).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete payment intents: %w", err)
	}

	rows, _ := res.RowsAffected()
	return rows, nil
}

// cleanupSolanaTransactions deletes Solana transactions that are expired or failed beyond the retention period
func (w CleanupExpiredDataWorker) cleanupSolanaTransactions(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	cutoff := now.Add(-retention)

	// Delete transactions that are either:
	// - expired (expires_at < cutoff)
	// - failed and older than retention period
	// - pending and older than retention period (abandoned)
	res, err := w.DB.GetDB().NewDelete().
		TableExpr("billing.solana_transactions").
		Where("(expires_at IS NOT NULL AND expires_at < ?) OR (status IN ('failed', 'pending') AND created_at < ?)", cutoff, cutoff).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete solana transactions: %w", err)
	}

	rows, _ := res.RowsAffected()
	return rows, nil
}

// cleanupSeenNotifications deletes seen notifications older than the retention period
func (w CleanupExpiredDataWorker) cleanupSeenNotifications(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	cutoff := now.Add(-retention)

	res, err := w.DB.GetDB().NewDelete().
		TableExpr("billing.notification_queue").
		Where("seen = ? AND created_at < ?", true, cutoff).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete seen notifications: %w", err)
	}

	rows, _ := res.RowsAffected()
	return rows, nil
}

// cleanupOldNotifications deletes all notifications (including unseen) older than the retention period
func (w CleanupExpiredDataWorker) cleanupOldNotifications(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	cutoff := now.Add(-retention)

	res, err := w.DB.GetDB().NewDelete().
		TableExpr("billing.notification_queue").
		Where("created_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete old notifications: %w", err)
	}

	rows, _ := res.RowsAffected()
	return rows, nil
}

// cleanupWebhookEvents deletes processed webhook events older than the retention period
func (w CleanupExpiredDataWorker) cleanupWebhookEvents(ctx context.Context, now time.Time, retention time.Duration) (int64, error) {
	cutoff := now.Add(-retention)

	// Only delete processed or duplicate webhook events - keep failed ones for debugging
	res, err := w.DB.GetDB().NewDelete().
		TableExpr("billing.webhook_events").
		Where("status IN ('processed', 'duplicate') AND created_at < ?", cutoff).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete webhook events: %w", err)
	}

	rows, _ := res.RowsAffected()
	return rows, nil
}
