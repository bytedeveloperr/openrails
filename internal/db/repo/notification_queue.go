package repo

import (
	"context"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"
)

type NotificationQueueRepo struct {
	db *db.DB
}

func NewNotificationQueueRepo(d *db.DB) *NotificationQueueRepo { return &NotificationQueueRepo{db: d} }

type GetNotificationsFilters struct {
	UserID    string                       `json:"user_id"`
	EventType models.NotificationEventType `json:"event_type"`
}

// GetNotifications returns a paginated list of notifications matching filters
func (r *NotificationQueueRepo) GetNotifications(ctx context.Context, opts query.QueryOptions[GetNotificationsFilters]) ([]*models.NotificationQueue, int64, error) {
	items := []*models.NotificationQueue{}
	q := r.db.GetDB().NewSelect().Model(&items).TableExpr(r.db.QualifiedTable("notification_queue")).Order("created_at DESC")
	if opts.Filters.UserID != "" {
		q = q.Where("user_id = ?", opts.Filters.UserID)
	}
	if opts.Filters.EventType != "" {
		q = q.Where("event_type = ?", opts.Filters.EventType)
	}
	if !opts.All {
		if opts.Page <= 0 {
			opts.Page = 1
		}
		if opts.PageSize <= 0 {
			opts.PageSize = 20
		}
		q = q.Offset((opts.Page - 1) * opts.PageSize).Limit(opts.PageSize)
	}
	count, err := q.ScanAndCount(ctx)
	return items, int64(count), err
}

func (r *NotificationQueueRepo) Delete(ctx context.Context, id models.UUID) error {
	_, err := r.db.GetDB().NewDelete().Model((*models.NotificationQueue)(nil)).TableExpr(r.db.QualifiedTable("notification_queue")).Where("id = ?", id).Exec(ctx)
	return err
}
