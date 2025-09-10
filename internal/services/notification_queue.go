package services

import (
	"context"
	"errors"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
)

type NotificationQueueService struct {
	db *db.DB
}

type GetNotificationsFilters struct {
	UserID    string `form:"user_id"`
	EventType string `form:"event_type"`
	Seen      *bool  `form:"seen"`
}

func NewNotificationQueueService(db *db.DB) *NotificationQueueService {
	return &NotificationQueueService{db: db}
}

func (r *NotificationQueueService) GetDB() *db.DB {
	return r.db
}

func (r *NotificationQueueService) Create(ctx context.Context, notification *models.NotificationQueue) error {
	result, err := r.db.GetDB().NewInsert().Model(notification).Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (r *NotificationQueueService) GetByID(ctx context.Context, id uuid.UUID) (*models.NotificationQueue, error) {
	var notification models.NotificationQueue
	err := r.db.GetDB().NewSelect().Model(&notification).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &notification, nil
}

func (r *NotificationQueueService) GetByUserID(ctx context.Context, userID string) ([]*models.NotificationQueue, error) {
	var notifications []*models.NotificationQueue
	err := r.db.GetDB().NewSelect().Model(&notifications).Where("user_id = ?", userID).Order("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *NotificationQueueService) GetUnseenByUserID(ctx context.Context, userID string) ([]*models.NotificationQueue, error) {
	var notifications []*models.NotificationQueue
	err := r.db.GetDB().NewSelect().Model(&notifications).
		Where("user_id = ?", userID).
		Where("seen = ?", false).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *NotificationQueueService) GetByEventType(ctx context.Context, eventType models.NotificationEventType) ([]*models.NotificationQueue, error) {
	var notifications []*models.NotificationQueue
	err := r.db.GetDB().NewSelect().Model(&notifications).Where("event_type = ?", eventType).Order("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return notifications, nil
}

// CountByUserAndEventSince counts notifications for a user and event type since a timestamp
func (r *NotificationQueueService) CountByUserAndEventSince(ctx context.Context, userID string, eventType models.NotificationEventType, since time.Time) (int, error) {
	var count int
	err := r.db.GetDB().NewSelect().
		Table("notification_queue").
		ColumnExpr("COUNT(*)").
		Where("user_id = ?", userID).
		Where("event_type = ?", eventType).
		Where("created_at >= ?", since).
		Scan(ctx, &count)
	return count, err
}

// GetUsersWithPendingDigest returns distinct user IDs with pending digest items since a timestamp
func (r *NotificationQueueService) GetUsersWithPendingDigest(ctx context.Context, since time.Time) ([]string, error) {
	var userIDs []string
	err := r.db.GetDB().NewSelect().
		TableExpr("notification_queue").
		ColumnExpr("DISTINCT user_id").
		Where("event_type = ?", models.NotificationTranslationCompletedPendingDigest).
		Where("created_at >= ?", since).
		Scan(ctx, &userIDs)
	if err != nil {
		return nil, err
	}
	return userIDs, nil
}

// GetPendingDigestForUser returns pending digest items for a user since a timestamp, limited
func (r *NotificationQueueService) GetPendingDigestForUser(ctx context.Context, userID string, since time.Time, limit int) ([]*models.NotificationQueue, error) {
	var items []*models.NotificationQueue
	q := r.db.GetDB().NewSelect().
		Model(&items).
		Where("user_id = ?", userID).
		Where("event_type = ?", models.NotificationTranslationCompletedPendingDigest).
		Where("created_at >= ?", since).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *NotificationQueueService) MarkAsSeen(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.GetDB().NewUpdate().
		Model((*models.NotificationQueue)(nil)).
		Set("seen = ?", true).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (r *NotificationQueueService) Update(ctx context.Context, notification *models.NotificationQueue) error {
	result, err := r.db.GetDB().NewUpdate().Model(notification).WherePK().Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (r *NotificationQueueService) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.GetDB().NewDelete().Model((*models.NotificationQueue)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

// GetNotifications retrieves notifications with filtering and pagination
func (r *NotificationQueueService) GetNotifications(ctx context.Context, queryOpts query.QueryOptions[GetNotificationsFilters]) ([]*models.NotificationQueue, int64, error) {
	var notifications []*models.NotificationQueue

	q := r.db.GetDB().NewSelect().Model(&notifications)
	// Note: User relationship is not preloaded - fetch separately if needed using UserService.GetFullUser

	// Apply filters
	if queryOpts.Filters.UserID != "" {
		q = q.Where("notification_queue.user_id = ?", queryOpts.Filters.UserID)
	}
	if queryOpts.Filters.EventType != "" {
		q = q.Where("notification_queue.event_type = ?", queryOpts.Filters.EventType)
	}
	if queryOpts.Filters.Seen != nil {
		q = q.Where("notification_queue.seen = ?", *queryOpts.Filters.Seen)
	}

	// Get total count
	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Apply pagination
	q = q.Limit(queryOpts.GetLimit()).Offset(queryOpts.GetOffset())

	// Apply ordering
	q = q.Order("notification_queue.created_at DESC")

	err = q.Scan(ctx)
	if err != nil {
		return nil, 0, err
	}

	return notifications, int64(total), nil
}
