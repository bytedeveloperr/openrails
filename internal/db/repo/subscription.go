package repo

import (
	"context"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type SubscriptionRepo struct {
	db *db.DB
}

func NewSubscriptionRepo(d *db.DB) *SubscriptionRepo { return &SubscriptionRepo{db: d} }

func (r *SubscriptionRepo) Create(ctx context.Context, s *models.Subscription) error {
	_, err := r.db.GetDB().NewInsert().Model(s).TableExpr(r.db.QualifiedTable("subscriptions")).Exec(ctx)
	return err
}

func (r *SubscriptionRepo) Update(ctx context.Context, s *models.Subscription) error {
	_, err := r.db.GetDB().NewUpdate().Model(s).TableExpr(r.db.QualifiedTable("subscriptions")).WherePK().Exec(ctx)
	return err
}

func (r *SubscriptionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.GetDB().NewDelete().Model((*models.Subscription)(nil)).TableExpr(r.db.QualifiedTable("subscriptions")).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *SubscriptionRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Subscription, error) {
	sub := new(models.Subscription)
	err := r.db.GetDB().NewSelect().Model(sub).TableExpr(r.db.QualifiedTable("subscriptions")).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// GetByUserID returns subscriptions for a user along with a total count (for pagination parity)
func (r *SubscriptionRepo) GetByUserID(ctx context.Context, userID string) ([]*models.Subscription, int64, error) {
	subs := []*models.Subscription{}
	q := r.db.GetDB().NewSelect().Model(&subs).TableExpr(r.db.QualifiedTable("subscriptions")).Where("user_id = ?", userID).Order("created_at DESC")
	count, err := q.ScanAndCount(ctx)
	return subs, int64(count), err
}
