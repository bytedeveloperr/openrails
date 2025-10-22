package repo

import (
	"context"
	"errors"
	"strings"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type SubscriptionFilters struct {
	UserID    string
	Status    string
	PriceID   uuid.UUID
	Processor string
}

type SubscriptionRepo struct {
	db *db.DB
}

func NewSubscriptionRepo(d *db.DB) *SubscriptionRepo { return &SubscriptionRepo{db: d} }

func (r *SubscriptionRepo) Create(ctx context.Context, s *models.Subscription) error {
	res, err := r.db.GetDB().NewInsert().Model(s).Exec(ctx)
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

func (r *SubscriptionRepo) Update(ctx context.Context, s *models.Subscription) error {
	res, err := r.db.GetDB().NewUpdate().Model(s).WherePK().Exec(ctx)
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

func (r *SubscriptionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models.Subscription)(nil)).Where("id = ?", id).Exec(ctx)
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

func (r *SubscriptionRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Subscription, error) {
	sub := new(models.Subscription)
	err := r.selectWithDetails(sub).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetLatestByUserID(ctx context.Context, userID string) (*models.Subscription, error) {
	sub := new(models.Subscription)
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	err = r.selectWithDetails(sub).
		Where("user_id = ?", uid).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetByUserIDAndPriceID(ctx context.Context, userID string, priceID uuid.UUID) (*models.Subscription, error) {
	sub := new(models.Subscription)
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	err = r.selectWithDetails(sub).
		Where("user_id = ?", uid).
		Where("price_id = ?", priceID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetActiveSubscription(ctx context.Context, userID string) (*models.Subscription, error) {
	sub := new(models.Subscription)
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	err = r.selectWithDetails(sub).
		Where("user_id = ?", uid).
		Where("status = ?", models.StatusActive).
		Where("(current_period_ends_at IS NULL OR current_period_ends_at > NOW())").
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetByProcessorSubscriptionID(ctx context.Context, processor, provider, processorSubscriptionID string) (*models.Subscription, error) {
	sub := new(models.Subscription)
	query := r.selectWithDetails(sub).
		Where("processor = ?", processor).
		Where("processor_subscription_id = ?", processorSubscriptionID)

	if strings.EqualFold(processor, string(models.ProcessorNMI)) {
		provider = strings.TrimSpace(strings.ToLower(provider))
		if provider == "" {
			provider = "mobius"
		}

		if provider == "mobius" {
			query = query.Where("(processor_provider = ? OR processor_provider IS NULL OR processor_provider = '')", provider)
		} else {
			query = query.Where("processor_provider = ?", provider)
		}
	}

	err := query.Scan(ctx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *SubscriptionRepo) GetActiveSubscriptionsByUserID(ctx context.Context, userID string) ([]models.Subscription, error) {
	subs := []models.Subscription{}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	query := r.selectWithDetails(&subs).
		Where("user_id = ?", uid).
		Where("status = ?", models.StatusActive).
		Order("created_at DESC")

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *SubscriptionRepo) GetSubscriptionsByProcessorAndUserID(ctx context.Context, userID string, processor models.Processor) ([]models.Subscription, error) {
	subs := []models.Subscription{}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	query := r.selectWithDetails(&subs).
		Where("user_id = ?", uid).
		Where("processor = ?", processor).
		Order("created_at DESC")

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *SubscriptionRepo) GetActiveSubscriptionsByProcessor(ctx context.Context, processor string) ([]*models.Subscription, error) {
	subs := []*models.Subscription{}
	query := r.selectWithDetails(&subs).
		Where("processor = ?", processor).
		Where("status = ?", models.StatusActive)

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *SubscriptionRepo) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]models.Subscription, int, error) {
	offset := (page - 1) * pageSize
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, 0, err
	}
	countQuery := r.db.GetDB().NewSelect().Model((*models.Subscription)(nil)).Where("user_id = ?", uid)
	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	subs := []models.Subscription{}
	dataQuery := r.selectWithDetails(&subs).
		Where("user_id = ?", uid).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset)

	if err := dataQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return subs, total, nil
}

func (r *SubscriptionRepo) GetSubscriptionsWithDetailsForUser(ctx context.Context, userID string, page, pageSize int) ([]models.Subscription, int, error) {
	offset := (page - 1) * pageSize
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, 0, err
	}
	countQuery := r.db.GetDB().NewSelect().Model((*models.Subscription)(nil)).Where("user_id = ?", uid)
	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	subs := []models.Subscription{}
	dataQuery := r.selectWithDetails(&subs).
		Where("user_id = ?", uid).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset)

	if err := dataQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return subs, total, nil
}

func (r *SubscriptionRepo) GetSubscribers(ctx context.Context, params query.QueryOptions[SubscriptionFilters]) ([]*models.Subscription, int64, error) {
	base := r.db.GetDB().NewSelect().Model((*models.Subscription)(nil))
	base = applySubscriptionFilters(base, params.Filters)

	total, err := base.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	subs := []*models.Subscription{}
	dataQuery := r.db.GetDB().NewSelect().Model(&subs)
	dataQuery = applySubscriptionFilters(dataQuery, params.Filters)

	if params.Limit > 0 {
		dataQuery = dataQuery.Limit(params.Limit)
	}
	if params.Offset > 0 {
		dataQuery = dataQuery.Offset(params.Offset)
	}

	dataQuery = dataQuery.Order("created_at DESC")

	if err := dataQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return subs, int64(total), nil
}

func applySubscriptionFilters(q *bun.SelectQuery, filters SubscriptionFilters) *bun.SelectQuery {
	if filters.UserID != "" {
		if uid, err := uuid.Parse(filters.UserID); err == nil {
			q = q.Where("user_id = ?", uid)
		}
	}
	if filters.Status != "" {
		q = q.Where("status = ?", filters.Status)
	}
	if filters.PriceID != uuid.Nil {
		q = q.Where("price_id = ?", filters.PriceID)
	}
	if filters.Processor != "" {
		q = q.Where("processor = ?", filters.Processor)
	}
	return q
}

func (r *SubscriptionRepo) selectWithDetails(model any) *bun.SelectQuery {
	return r.db.GetDB().NewSelect().Model(model).
		Relation("Price").
		Relation("PaymentMethod")
}
