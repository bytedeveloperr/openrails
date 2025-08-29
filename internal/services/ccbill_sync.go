package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/supabase-community/gotrue-go/types"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
)

type CCBillSyncService struct {
	UserRepo         *repo.UserRepo
	SubscriptionRepo *repo.SubscriptionRepo
	ProductRepo      *repo.ProductRepo
	PriceRepo        *repo.PriceRepo
	NotificationRepo *repo.NotificationQueueRepo
}

type SyncResult struct {
	TotalRecords         int
	UsersMatched         int
	UsersUnmatched       int
	SubscriptionsCreated int
	SubscriptionsUpdated int
	ErrorsEncountered    int
	ProcessingTime       time.Duration
	// Change tracking for dead-letter logging
	StatusChanges       int                  // Subscriptions where status changed from our DB vs CCBill
	ExpiryDateChanges   int                  // Subscriptions where expiry date changed
	MissedWebhookEvents []MissedWebhookEvent // Detailed changes that indicate missed webhooks
}

type MissedWebhookEvent struct {
	SubscriptionID string              `json:"subscription_id"`
	UserEmail      string              `json:"user_email"`
	ChangeType     string              `json:"change_type"` // "status_change", "expiry_change", "new_subscription"
	OldValue       string              `json:"old_value,omitempty"`
	NewValue       string              `json:"new_value"`
	CCBillRecord   ccbill.CCBillRecord `json:"ccbill_record"`
	DetectedAt     time.Time           `json:"detected_at"`
	Description    string              `json:"description"`
}

type SyncOperation struct {
	Record        ccbill.CCBillRecord
	User          *types.User
	Subscription  *models.Subscription
	OperationType string // "create", "update", "skip"
	Error         error
}

func NewCCBillSyncService(userRepo *repo.UserRepo, subscriptionRepo *repo.SubscriptionRepo, productRepo *repo.ProductRepo, priceRepo *repo.PriceRepo, notificationRepo *repo.NotificationQueueRepo) *CCBillSyncService {
	return &CCBillSyncService{
		UserRepo:         userRepo,
		SubscriptionRepo: subscriptionRepo,
		ProductRepo:      productRepo,
		PriceRepo:        priceRepo,
		NotificationRepo: notificationRepo,
	}
}

func (s *CCBillSyncService) SyncSubscriptions(ctx context.Context, records []ccbill.CCBillRecord, dryRun bool) (*SyncResult, error) {
	startTime := time.Now()

	result := &SyncResult{
		TotalRecords:        len(records),
		MissedWebhookEvents: make([]MissedWebhookEvent, 0),
	}

	log.WithFields(log.Fields{
		"total_records": len(records),
		"dry_run":       dryRun,
	}).Info("Starting CCBill subscription synchronization")

	for _, record := range records {
		operation := s.processRecordWithChangeTracking(ctx, record, dryRun, result)

		if operation.User != nil {
			result.UsersMatched++
		} else {
			result.UsersUnmatched++
		}

		if operation.Error != nil {
			result.ErrorsEncountered++
			log.WithFields(log.Fields{
				"subscription_id": record.SubscriptionID,
				"email":           record.Email,
				"error":           operation.Error,
			}).Error("Failed to process CCBill record")
		} else {
			switch operation.OperationType {
			case "create":
				result.SubscriptionsCreated++
			case "update":
				result.SubscriptionsUpdated++
			}
		}
	}

	result.ProcessingTime = time.Since(startTime)

	// Log summary and missed webhook events to dead-letter queue
	s.logSyncResults(ctx, result, dryRun)

	log.WithFields(log.Fields{
		"total_records":         result.TotalRecords,
		"users_matched":         result.UsersMatched,
		"users_unmatched":       result.UsersUnmatched,
		"subscriptions_created": result.SubscriptionsCreated,
		"subscriptions_updated": result.SubscriptionsUpdated,
		"status_changes":        result.StatusChanges,
		"expiry_changes":        result.ExpiryDateChanges,
		"missed_webhook_events": len(result.MissedWebhookEvents),
		"errors_encountered":    result.ErrorsEncountered,
		"processing_time":       result.ProcessingTime.String(),
		"dry_run":               dryRun,
	}).Info("CCBill subscription synchronization completed")

	return result, nil
}

func (s *CCBillSyncService) processRecord(ctx context.Context, record ccbill.CCBillRecord, dryRun bool) SyncOperation {
	operation := SyncOperation{
		Record: record,
	}

	user, err := s.UserRepo.GetGoTrueUserByEmail(ctx, record.Email)
	if err != nil {
		operation.Error = fmt.Errorf("user not found for email %s: %w", record.Email, err)
		return operation
	}
	operation.User = user

	existingSubscription, err := s.SubscriptionRepo.GetByUserID(ctx, user.ID)
	if err != nil {

		operation.OperationType = "create"
		if !dryRun {
			subscription, createErr := s.createSubscription(ctx, record, user.ID)
			if createErr != nil {
				operation.Error = createErr
				return operation
			}
			operation.Subscription = subscription
		}
	} else {

		operation.OperationType = "update"
		operation.Subscription = existingSubscription
		if !dryRun {
			updateErr := s.updateSubscription(ctx, record, existingSubscription)
			if updateErr != nil {
				operation.Error = updateErr
				return operation
			}
		}
	}

	return operation
}

func (s *CCBillSyncService) processRecordWithChangeTracking(ctx context.Context, record ccbill.CCBillRecord, dryRun bool, result *SyncResult) SyncOperation {
	operation := SyncOperation{
		Record: record,
	}

	user, err := s.UserRepo.GetGoTrueUserByEmail(ctx, record.Email)
	if err != nil {
		operation.Error = fmt.Errorf("user not found for email %s: %w", record.Email, err)
		return operation
	}
	operation.User = user

	existingSubscription, err := s.SubscriptionRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		// New subscription - this indicates we missed a webhook event for subscription creation
		operation.OperationType = "create"
		if !dryRun {
			subscription, createErr := s.createSubscription(ctx, record, user.ID)
			if createErr != nil {
				operation.Error = createErr
				return operation
			}
			operation.Subscription = subscription

			// Log missed webhook event for new subscription
			result.MissedWebhookEvents = append(result.MissedWebhookEvents, MissedWebhookEvent{
				SubscriptionID: strconv.FormatInt(record.SubscriptionID, 10),
				UserEmail:      record.Email,
				ChangeType:     "new_subscription",
				NewValue:       record.Status,
				CCBillRecord:   record,
				DetectedAt:     time.Now(),
				Description:    fmt.Sprintf("Found new CCBill subscription %d for user %s - likely missed subscription creation webhook", record.SubscriptionID, record.Email),
			})
		}
	} else {
		// Existing subscription - check for changes that indicate missed webhook events
		operation.OperationType = "update"
		operation.Subscription = existingSubscription

		// Track changes before updating
		s.trackSubscriptionChanges(record, existingSubscription, result)

		if !dryRun {
			updateErr := s.updateSubscription(ctx, record, existingSubscription)
			if updateErr != nil {
				operation.Error = updateErr
				return operation
			}
		}
	}

	return operation
}

func (s *CCBillSyncService) trackSubscriptionChanges(record ccbill.CCBillRecord, existingSubscription *models.Subscription, result *SyncResult) {
	ccbillStatus := s.mapCCBillStatusToInternal(record.Status)
	currentStatus := string(existingSubscription.Status)

	// Check for status changes
	if ccbillStatus != currentStatus {
		result.StatusChanges++
		result.MissedWebhookEvents = append(result.MissedWebhookEvents, MissedWebhookEvent{
			SubscriptionID: strconv.FormatInt(record.SubscriptionID, 10),
			UserEmail:      record.Email,
			ChangeType:     "status_change",
			OldValue:       currentStatus,
			NewValue:       ccbillStatus,
			CCBillRecord:   record,
			DetectedAt:     time.Now(),
			Description:    fmt.Sprintf("Status mismatch: our DB shows '%s' but CCBill shows '%s' - likely missed status webhook", currentStatus, ccbillStatus),
		})
	}

	// Check for expiry date changes
	ccbillExpiryDate, err := s.parseCCBillDate(record.ExpiryDate)
	if err == nil && ccbillExpiryDate != nil {
		var currentExpiryStr string
		if existingSubscription.CurrentPeriodEndsAt != nil {
			currentExpiryStr = existingSubscription.CurrentPeriodEndsAt.Format("2006-01-02")
		}
		ccbillExpiryStr := ccbillExpiryDate.Format("2006-01-02")

		if currentExpiryStr != ccbillExpiryStr {
			result.ExpiryDateChanges++
			result.MissedWebhookEvents = append(result.MissedWebhookEvents, MissedWebhookEvent{
				SubscriptionID: strconv.FormatInt(record.SubscriptionID, 10),
				UserEmail:      record.Email,
				ChangeType:     "expiry_change",
				OldValue:       currentExpiryStr,
				NewValue:       ccbillExpiryStr,
				CCBillRecord:   record,
				DetectedAt:     time.Now(),
				Description:    fmt.Sprintf("Expiry date mismatch: our DB shows '%s' but CCBill shows '%s' - likely missed renewal webhook", currentExpiryStr, ccbillExpiryStr),
			})
		}
	}
}

func (s *CCBillSyncService) logSyncResults(ctx context.Context, result *SyncResult, dryRun bool) {
	if len(result.MissedWebhookEvents) == 0 {
		// Sync was redundant - webhooks are working properly
		log.Info("CCBill datalink sync found no discrepancies - webhook system working correctly")
		return
	}

	// Log each missed webhook event to admin logs (not user notifications)
	for _, missedEvent := range result.MissedWebhookEvents {
		log.WithFields(log.Fields{
			"source":          "ccbill_datalink_sync",
			"missed_event":    missedEvent,
			"sync_timestamp":  time.Now(),
			"requires_review": true,
			"alert_type":      "webhook_dead_letter",
		}).Error("Missed webhook event detected - requires admin review")

		log.WithFields(log.Fields{
			"subscription_id": missedEvent.SubscriptionID,
			"user_email":      missedEvent.UserEmail,
			"change_type":     missedEvent.ChangeType,
			"old_value":       missedEvent.OldValue,
			"new_value":       missedEvent.NewValue,
			"description":     missedEvent.Description,
		}).Warn("Detected missed CCBill webhook event during datalink sync")
	}

	// Create summary notification for admin review
	if !dryRun && s.NotificationRepo != nil {
		summaryNotification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    uuid.Nil, // System notification
			EventType: models.NotificationSystemAlert,
			Data: map[string]any{
				"type":                  "ccbill_webhook_issues",
				"missed_events_count":   len(result.MissedWebhookEvents),
				"status_changes":        result.StatusChanges,
				"expiry_changes":        result.ExpiryDateChanges,
				"subscriptions_created": result.SubscriptionsCreated,
				"sync_timestamp":        time.Now(),
				"message":               fmt.Sprintf("CCBill datalink sync detected %d missed webhook events - webhook delivery may have issues", len(result.MissedWebhookEvents)),
			},
		}

		if err := s.NotificationRepo.Create(ctx, summaryNotification); err != nil {
			log.WithError(err).Error("Failed to create summary notification for CCBill webhook issues")
		}
	}
}

func (s *CCBillSyncService) createSubscription(ctx context.Context, record ccbill.CCBillRecord, userID uuid.UUID) (*models.Subscription, error) {
	status := s.mapCCBillStatusToInternal(record.Status)

	expiryDate, err := s.parseCCBillDate(record.ExpiryDate)
	if err != nil {
		log.WithFields(log.Fields{
			"subscription_id": record.SubscriptionID,
			"expiry_date":     record.ExpiryDate,
			"error":           err,
		}).Warn("Failed to parse expiry date, using default")
		expiryDate = nil
	}

	// Get default premium product and price for Wave 18
	products, err := s.ProductRepo.GetActive(ctx)
	if err != nil || len(products) == 0 {
		return nil, fmt.Errorf("failed to find active products: %w", err)
	}

	// Use first active product (should be premium membership)
	product := products[0]
	prices, err := s.PriceRepo.GetActiveByProductID(ctx, product.ID)
	if err != nil || len(prices) == 0 {
		return nil, fmt.Errorf("failed to find active prices for product: %w", err)
	}

	// Use first active price
	price := prices[0]

	subscription := &models.Subscription{
		ID:                  uuid.New(),
		UserID:              userID,
		Status:              models.SubscriptionStatus(status),
		CurrentPeriodEndsAt: expiryDate,
		PriceID:             price.ID,
	}

	err = s.SubscriptionRepo.Create(ctx, subscription)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	log.WithFields(log.Fields{
		"subscription_id": subscription.ID,
		"user_id":         userID,
		"ccbill_id":       record.SubscriptionID,
		"status":          status,
	}).Info("Created new subscription from CCBill data")

	return subscription, nil
}

func (s *CCBillSyncService) updateSubscription(ctx context.Context, record ccbill.CCBillRecord, subscription *models.Subscription) error {
	status := s.mapCCBillStatusToInternal(record.Status)

	expiryDate, err := s.parseCCBillDate(record.ExpiryDate)
	if err != nil {
		log.WithFields(log.Fields{
			"subscription_id": subscription.ID,
			"expiry_date":     record.ExpiryDate,
			"error":           err,
		}).Warn("Failed to parse expiry date, keeping existing")
	} else {
		subscription.CurrentPeriodEndsAt = expiryDate
	}

	oldStatus := subscription.Status
	subscription.Status = models.SubscriptionStatus(status)
	subscription.UpdatedAt = time.Now()

	err = s.SubscriptionRepo.Update(ctx, subscription)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	log.WithFields(log.Fields{
		"subscription_id": subscription.ID,
		"ccbill_id":       record.SubscriptionID,
		"old_status":      oldStatus,
		"new_status":      status,
		"expiry_date":     record.ExpiryDate,
	}).Info("Updated subscription from CCBill data")

	return nil
}

func (s *CCBillSyncService) mapCCBillStatusToInternal(ccbillStatus string) string {
	switch ccbillStatus {
	case "ACTIVE":
		return string(models.StatusActive)
	case "EXPIRED":
		// Wave 18: Expired subscriptions are considered cancelled (will never rebill again)
		return string(models.StatusCancelled)
	case "CANCELLED":
		return string(models.StatusCancelled)
	default:
		log.WithField("ccbill_status", ccbillStatus).Warn("Unknown CCBill status, defaulting to pending")
		return string(models.StatusPending)
	}
}

func (s *CCBillSyncService) parseCCBillDate(dateStr string) (*time.Time, error) {
	if dateStr == "" {
		return nil, nil
	}

	formats := []string{
		"2006-01-02",
		"01/02/2006",
		"2006-01-02 15:04:05",
		"01/02/2006 15:04:05",
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, dateStr); err == nil {
			return &parsed, nil
		}
	}

	return nil, fmt.Errorf("unable to parse date: %s", dateStr)
}

// OutputRecordsCSV outputs CCBill records in CSV format to stdout by default
func (s *CCBillSyncService) OutputRecordsCSV(records []ccbill.CCBillRecord, writer io.Writer) error {
	if writer == nil {
		writer = os.Stdout
	}

	csvWriter := csv.NewWriter(writer)
	defer csvWriter.Flush()

	// Write CSV header
	header := []string{
		"subscription_id",
		"email",
		"username",
		"status",
		"transaction_type",
		"client_acc_num",
		"date",
		"rebill_date",
		"expiry_date",
	}
	if err := csvWriter.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write records
	for _, record := range records {
		row := []string{
			strconv.FormatInt(record.SubscriptionID, 10),
			record.Email,
			record.Username,
			record.Status,
			record.TransactionType,
			record.ClientAccNum,
			record.Date,
			record.RebillDate,
			record.ExpiryDate,
		}
		if err := csvWriter.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

// OutputSyncResultCSV outputs sync results in CSV format
func (s *CCBillSyncService) OutputSyncResultCSV(result *SyncResult, writer io.Writer) error {
	if writer == nil {
		writer = os.Stdout
	}

	csvWriter := csv.NewWriter(writer)
	defer csvWriter.Flush()

	// Write CSV header for sync results
	header := []string{
		"metric",
		"value",
	}
	if err := csvWriter.Write(header); err != nil {
		return fmt.Errorf("failed to write sync result CSV header: %w", err)
	}

	// Write sync statistics as rows
	rows := [][]string{
		{"total_records", strconv.Itoa(result.TotalRecords)},
		{"users_matched", strconv.Itoa(result.UsersMatched)},
		{"users_unmatched", strconv.Itoa(result.UsersUnmatched)},
		{"subscriptions_created", strconv.Itoa(result.SubscriptionsCreated)},
		{"subscriptions_updated", strconv.Itoa(result.SubscriptionsUpdated)},
		{"errors_encountered", strconv.Itoa(result.ErrorsEncountered)},
		{"processing_time_ms", strconv.FormatInt(result.ProcessingTime.Milliseconds(), 10)},
		{"completed_at", time.Now().UTC().Format(time.RFC3339)},
	}

	for _, row := range rows {
		if err := csvWriter.Write(row); err != nil {
			return fmt.Errorf("failed to write sync result CSV row: %w", err)
		}
	}

	return nil
}

// SyncSubscriptionsWithCSVOutput performs sync and outputs results as CSV by default
func (s *CCBillSyncService) SyncSubscriptionsWithCSVOutput(ctx context.Context, records []ccbill.CCBillRecord, dryRun bool, writer io.Writer) (*SyncResult, error) {
	// First output the records as CSV
	log.Info("Outputting CCBill records as CSV...")
	if err := s.OutputRecordsCSV(records, writer); err != nil {
		return nil, fmt.Errorf("failed to output records as CSV: %w", err)
	}

	// Then perform the sync
	result, err := s.SyncSubscriptions(ctx, records, dryRun)
	if err != nil {
		return nil, err
	}

	// Output sync results to stderr so CSV data stays clean on stdout
	log.WithFields(log.Fields{
		"total_records":         result.TotalRecords,
		"users_matched":         result.UsersMatched,
		"users_unmatched":       result.UsersUnmatched,
		"subscriptions_created": result.SubscriptionsCreated,
		"subscriptions_updated": result.SubscriptionsUpdated,
		"errors_encountered":    result.ErrorsEncountered,
		"processing_time":       result.ProcessingTime.String(),
		"dry_run":               dryRun,
	}).Info("CCBill sync completed - results sent to stderr, CSV data on stdout")

	return result, nil
}
