package repo

import (
	"context"
	"errors"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
)

type NotificationFilters struct {
	UserID    string
	EventType models.NotificationEventType
	Seen      *bool
}

type NotificationQueueRepo struct {
	db *db.DB
}

func NewNotificationQueueRepo(d *db.DB) *NotificationQueueRepo { return &NotificationQueueRepo{db: d} }

func (r *NotificationQueueRepo) Create(ctx context.Context, notification *models.NotificationQueue) error {
	res, err := r.db.GetDB().NewInsert().Model(notification).TableExpr(r.db.QualifiedTable("notification_queue")).Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *NotificationQueueRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.NotificationQueue, error) {
	notification := new(models.NotificationQueue)
	if err := r.db.GetDB().NewSelect().Model(notification).TableExpr(r.db.QualifiedTable("notification_queue")).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return notification, nil
}

func (r *NotificationQueueRepo) GetByUserID(ctx context.Context, userID string) ([]*models.NotificationQueue, error) {
	notifications := []*models.NotificationQueue{}
	if err := r.db.GetDB().NewSelect().Model(&notifications).TableExpr(r.db.QualifiedTable("notification_queue")).Where("user_id = ?", userID).Order("created_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *NotificationQueueRepo) GetUnseenByUserID(ctx context.Context, userID string) ([]*models.NotificationQueue, error) {
	notifications := []*models.NotificationQueue{}
	if err := r.db.GetDB().NewSelect().Model(&notifications).TableExpr(r.db.QualifiedTable("notification_queue")).Where("user_id = ?", userID).Where("seen = ?", false).Order("created_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *NotificationQueueRepo) GetByEventType(ctx context.Context, eventType models.NotificationEventType) ([]*models.NotificationQueue, error) {
	notifications := []*models.NotificationQueue{}
	if err := r.db.GetDB().NewSelect().Model(&notifications).TableExpr(r.db.QualifiedTable("notification_queue")).Where("event_type = ?", eventType).Order("created_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *NotificationQueueRepo) CountByUserAndEventSince(ctx context.Context, userID string, eventType models.NotificationEventType, since time.Time) (int, error) {
	var count int
	err := r.db.GetDB().NewSelect().TableExpr(r.db.QualifiedTable("notification_queue")).ColumnExpr("COUNT(*)").Where("user_id = ?", userID).Where("event_type = ?", eventType).Where("created_at >= ?", since).Scan(ctx, &count)
	return count, err
}

func (r *NotificationQueueRepo) GetUsersWithPendingDigest(ctx context.Context, since time.Time) ([]string, error) {
	userIDs := []string{}
	err := r.db.GetDB().NewSelect().TableExpr(r.db.QualifiedTable("notification_queue")).ColumnExpr("DISTINCT user_id").Where("event_type = ?", models.NotificationTranslationCompletedPendingDigest).Where("created_at >= ?", since).Scan(ctx, &userIDs)
	if err != nil {
		return nil, err
	}
	return userIDs, nil
}

func (r *NotificationQueueRepo) GetPendingDigestForUser(ctx context.Context, userID string, since time.Time, limit int) ([]*models.NotificationQueue, error) {
	items := []*models.NotificationQueue{}
	q := r.db.GetDB().NewSelect().Model(&items).TableExpr(r.db.QualifiedTable("notification_queue")).Where("user_id = ?", userID).Where("event_type = ?", models.NotificationTranslationCompletedPendingDigest).Where("created_at >= ?", since).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *NotificationQueueRepo) MarkAsSeen(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewUpdate().Model((*models.NotificationQueue)(nil)).TableExpr(r.db.QualifiedTable("notification_queue")).Set("seen = ?", true).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *NotificationQueueRepo) Update(ctx context.Context, notification *models.NotificationQueue) error {
	res, err := r.db.GetDB().NewUpdate().Model(notification).TableExpr(r.db.QualifiedTable("notification_queue")).WherePK().Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *NotificationQueueRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models.NotificationQueue)(nil)).TableExpr(r.db.QualifiedTable("notification_queue")).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *NotificationQueueRepo) GetNotifications(ctx context.Context, opts query.QueryOptions[NotificationFilters]) ([]*models.NotificationQueue, int64, error) {
	notifications := []*models.NotificationQueue{}
	q := r.db.GetDB().NewSelect().Model(&notifications).TableExpr(r.db.QualifiedTable("notification_queue"))

	if opts.Filters.UserID != "" {
		q = q.Where("notification_queue.user_id = ?", opts.Filters.UserID)
	}
	if opts.Filters.EventType != "" {
		q = q.Where("notification_queue.event_type = ?", opts.Filters.EventType)
	}
	if opts.Filters.Seen != nil {
		q = q.Where("notification_queue.seen = ?", *opts.Filters.Seen)
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	q = q.Limit(opts.GetLimit()).Offset(opts.GetOffset()).Order("notification_queue.created_at DESC")

	if err := q.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return notifications, int64(total), nil
}
