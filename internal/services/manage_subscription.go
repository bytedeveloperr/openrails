package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type ManageSubscriptionService struct {
	SubscriptionService      *SubscriptionService
	UserRoleGrantService     *UserRoleGrantService
	UserRoleInterfaceService *UserRoleInterfaceService
	NotificationQueueService *NotificationQueueService
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

func NewManageSubscriptionService(subscriptionService *SubscriptionService, userRoleGrantService *UserRoleGrantService, userRoleInterfaceService *UserRoleInterfaceService, notificationQueueService *NotificationQueueService) *ManageSubscriptionService {
	return &ManageSubscriptionService{
		SubscriptionService:      subscriptionService,
		UserRoleGrantService:     userRoleGrantService,
		UserRoleInterfaceService: userRoleInterfaceService,
		NotificationQueueService: notificationQueueService,
	}
}

func (s *ManageSubscriptionService) UpdateStatus(ctx context.Context, params *UpdateSubscriptionStatusParams) error {
	subscription, err := s.SubscriptionService.GetByID(ctx, uuid.MustParse(params.SubscriptionID))
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

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return err
	}

	// Handle role grants based on status change
	switch params.Status {
	case models.StatusCancelled, models.StatusPastDue:
		// Revoke role grants for inactive subscriptions using proper interface
		if s.UserRoleInterfaceService != nil {
			result, err := s.UserRoleInterfaceService.HandleImmediateCancelOrRefund(
				ctx,
				subscription.UserID,
				"premium",
				subscription.ID,
				fmt.Sprintf("status_change_%s_%d", params.Status, time.Now().Unix()),
				string(params.Status),
			)
			if err != nil {
				log.WithFields(log.Fields{
					"subscription_id": subscription.ID,
					"user_id":         subscription.UserID,
					"status":          params.Status,
					"error":           err.Error(),
				}).Error("Failed to revoke role grants during subscription status update")
			} else {
				log.WithFields(log.Fields{
					"subscription_id": subscription.ID,
					"user_id":         subscription.UserID,
					"status":          params.Status,
					"action":          result.Action,
					"user_role_id":    result.UserRoleID,
				}).Info("Successfully revoked premium role during subscription status update")
			}
		}

		// Add notification for membership ended
		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: models.NotificationPremiumEnded,
		}
		if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
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
	subscription, err := s.SubscriptionService.GetByID(ctx, uuid.MustParse(params.SubscriptionID))
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

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return err
	}

	// Manual subscription extensions don't generate user notifications

	// Note: Subscription events will be logged to ClickHouse event system in Wave 19
	// This replaces the deprecated SubscriptionEvent table approach

	return nil
}
