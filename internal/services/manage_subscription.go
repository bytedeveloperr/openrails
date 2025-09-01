package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type ManageSubscriptionService struct {
	DB *db.DB
}

// GetSubscriptionByID retrieves a subscription by its ID
func (s *ManageSubscriptionService) GetSubscriptionByID(ctx context.Context, id uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription by ID: %w", err)
	}
	return &subscription, nil
}

// UpdateSubscription updates a subscription
func (s *ManageSubscriptionService) UpdateSubscription(ctx context.Context, subscription *models.Subscription) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model(subscription).
		Where("id = ?", subscription.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}
	return nil
}

// RevokeUserRolesBySubSourceID revokes user roles by subscription source ID
func (s *ManageSubscriptionService) RevokeUserRolesBySubSourceID(ctx context.Context, subID uuid.UUID) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model((*models.UserRoleGrant)(nil)).
		Set("revoked_at = ?", time.Now()).
		Where("sub_source_id = ?", subID).
		Where("revoked_at IS NULL").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke user roles: %w", err)
	}
	return nil
}

// CreateNotification creates a new notification
func (s *ManageSubscriptionService) CreateNotification(ctx context.Context, notification *models.NotificationQueue) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(notification).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}
	return nil
}

type UpdateSubscriptionStatusParams struct {
	SubscriptionID string
	Status         models.SubscriptionStatus
	FailureReason  string            // Legacy field - not used in Wave 18
	FailureCode    string            // Legacy field - not used in Wave 18
	CancelFeedback string            // Maps to subscription.CancelFeedback
	CancelType     models.CancelType // Maps to subscription.CancelType
}

type ExtendSubscriptionParams struct {
	SubscriptionID string
	Duration       time.Duration
}

func NewManageSubscriptionService(db *db.DB) *ManageSubscriptionService {
	return &ManageSubscriptionService{
		DB: db,
	}
}

func (s *ManageSubscriptionService) UpdateStatus(ctx context.Context, params *UpdateSubscriptionStatusParams) error {
	subscription, err := s.GetSubscriptionByID(ctx, uuid.MustParse(params.SubscriptionID))
	if err != nil {
		return err
	}

	// oldValue := subscription.Status  // Unused in Wave 18
	subscription.Status = params.Status
	subscription.UpdatedAt = time.Now()

	switch params.Status {
	case models.StatusActive:
		if subscription.StartedAt.IsZero() {
			subscription.StartedAt = time.Now()
		}
	case models.StatusPastDue:
		// Note: FailureReason and FailureCode events should be logged separately
		// StatusPastDue means payment failed but we're still trying to rebill
	case models.StatusCancelled:
		if params.CancelFeedback != "" {
			subscription.CancelFeedback = &params.CancelFeedback
		}
		if params.CancelType != "" {
			subscription.CancelType = &params.CancelType
		}
		cancelledAt := time.Now()
		subscription.CancelledAt = &cancelledAt
		// For cancelled subscriptions, also set EndedAt
		subscription.EndedAt = &cancelledAt
	}

	if err := s.UpdateSubscription(ctx, subscription); err != nil {
		return err
	}

	// Handle role grants based on status change
	switch params.Status {
	case models.StatusCancelled, models.StatusPastDue:
		// Revoke role grants for inactive subscriptions
		if err := s.RevokeUserRolesBySubSourceID(ctx, subscription.ID); err != nil {
			log.WithFields(log.Fields{
				"subscription_id": subscription.ID,
				"user_id":         subscription.UserID,
				"error":           err.Error(),
			}).Error("Failed to revoke role grants during subscription status update")
		}

		// Add notification for membership ended
		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: models.NotificationPremiumEnded,
		}
		if err := s.CreateNotification(ctx, notification); err != nil {
			log.WithFields(log.Fields{
				"subscription_id":   subscription.ID,
				"user_id":           subscription.UserID,
				"notification_type": notification.EventType,
				"error":             err.Error(),
			}).Error("Failed to create notification during subscription status update")
		}
	}

	// Note: Subscription events will be logged to ClickHouse event system in Wave 19
	// This replaces the deprecated SubscriptionEvent table approach

	return nil
}

func (s *ManageSubscriptionService) ExtendSubscription(ctx context.Context, params *ExtendSubscriptionParams) error {
	subscription, err := s.GetSubscriptionByID(ctx, uuid.MustParse(params.SubscriptionID))
	if err != nil {
		return err
	}

	if subscription.Status != models.StatusActive {
		return errors.New("subscription is not active")
	}

	// oldEndTime := subscription.CurrentPeriodEndsAt  // Unused in Wave 18
	// extendedAt := time.Now()
	// subscription.ManuallyExtendedAt = &extendedAt  // Field removed in Wave 18

	if subscription.CurrentPeriodEndsAt != nil {
		newEndTime := subscription.CurrentPeriodEndsAt.Add(params.Duration)
		subscription.CurrentPeriodEndsAt = &newEndTime
	} else {
		newEndTime := time.Now().Add(params.Duration)
		subscription.CurrentPeriodEndsAt = &newEndTime
		startTime := time.Now()
		subscription.CurrentPeriodStartsAt = &startTime
	}

	if err := s.UpdateSubscription(ctx, subscription); err != nil {
		return err
	}

	// Manual subscription extensions don't generate user notifications

	// Note: Subscription events will be logged to ClickHouse event system in Wave 19
	// This replaces the deprecated SubscriptionEvent table approach

	return nil
}
