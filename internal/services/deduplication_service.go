package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/open-rails/openrails/internal/db/models"
	log "github.com/sirupsen/logrus"
)

// DeduplicationService provides robust webhook deduplication using the unified IdempotencyService
type DeduplicationService struct {
	idem *IdempotencyService
}

// NewDeduplicationService creates a new webhook deduplication service
func NewDeduplicationService(idem *IdempotencyService) *DeduplicationService {
	return &DeduplicationService{idem: idem}
}

// IsDuplicate checks if a webhook with this eventID has already been processed successfully.
// Returns true if the webhook should be skipped (already processed), false otherwise.
func (s *DeduplicationService) IsDuplicate(ctx context.Context, processor string, eventID string) (bool, error) {
	trimmedEventID := strings.TrimSpace(eventID)
	if trimmedEventID == "" {
		return false, nil // No event ID, can't deduplicate
	}

	op := fmt.Sprintf("webhook.%s.event", processor)
	rec, alreadyExists, err := s.idem.Begin(ctx, op, trimmedEventID)
	if err != nil {
		return false, fmt.Errorf("failed to check idempotency: %w", err)
	}
	if alreadyExists && rec.Status == IdempotencyStatusSuccess {
		return true, nil // Already processed successfully
	}

	// If we got here, we claimed this eventID - mark it as successful immediately
	// since we're just using this for duplicate detection
	if rec != nil && !alreadyExists {
		if err := s.idem.Complete(ctx, op, trimmedEventID, nil); err != nil {
			log.WithContext(ctx).WithError(err).Warn("failed to mark webhook idempotency as complete")
		}
	}

	return false, nil
}

// ProcessWebhook handles webhook deduplication and processing coordination.
func (s *DeduplicationService) ProcessWebhook(ctx context.Context, eventID, eventType string, processor models.Processor, payload interface{}, processingFunc func(ctx context.Context) error) error {
	var payloadBytes []byte
	if payload != nil {
		if data, err := json.Marshal(payload); err == nil {
			payloadBytes = data
		} else {
			log.WithContext(ctx).WithError(err).Warn("failed to marshal webhook payload for idempotency storage")
		}
	}

	trimmedEventID := strings.TrimSpace(eventID)
	op := fmt.Sprintf("webhook.%s.%s", processor, eventType)

	var claimed bool
	if trimmedEventID != "" {
		rec, alreadyExists, err := s.idem.Begin(ctx, op, trimmedEventID)
		if err != nil {
			return fmt.Errorf("failed to begin idempotency: %w", err)
		}
		if alreadyExists && rec.Status == IdempotencyStatusSuccess {
			log.WithContext(ctx).WithFields(log.Fields{
				"eventID":   trimmedEventID,
				"eventType": eventType,
				"processor": processor,
			}).Info("Webhook already processed successfully, skipping")
			return nil
		}
		claimed = !alreadyExists
	}

	processingErr := processingFunc(ctx)
	if processingErr != nil {
		log.WithContext(ctx).WithFields(log.Fields{
			"eventID":   trimmedEventID,
			"eventType": eventType,
			"processor": processor,
			"error":     processingErr.Error(),
		}).Error("Webhook processing failed")

		if claimed && trimmedEventID != "" {
			if err := s.idem.Fail(ctx, op, trimmedEventID, processingErr); err != nil {
				log.WithContext(ctx).WithError(err).Warn("failed to mark webhook idempotency as failed")
			}
		}

		return fmt.Errorf("webhook processing failed: %w", processingErr)
	}

	if claimed && trimmedEventID != "" {
		if err := s.idem.Complete(ctx, op, trimmedEventID, payloadBytes); err != nil {
			log.WithContext(ctx).WithError(err).Warn("failed to mark webhook idempotency as complete")
		}
	}

	log.WithContext(ctx).WithFields(log.Fields{
		"eventID":   trimmedEventID,
		"eventType": eventType,
		"processor": processor,
	}).Info("Webhook processed successfully")

	return nil
}

// GetFailedWebhooks retrieves webhooks that failed and can be retried
// Not supported with TTL-based storage - failed webhooks expire automatically
func (s *DeduplicationService) GetFailedWebhooks(ctx context.Context, processor models.Processor, limit int) ([]any, error) {
	return []any{}, nil
}

// CleanupOldWebhooks is a no-op with TTL-based storage
// Redis/in-memory automatically expires old records
func (s *DeduplicationService) CleanupOldWebhooks(ctx context.Context, retentionDays int) (int64, error) {
	log.WithContext(ctx).WithFields(log.Fields{
		"retentionDays": retentionDays,
	}).Debug("Webhook cleanup is automatic with TTL-based storage")
	return 0, nil
}
