package services

import (
	"context"
	"errors"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type UserRoleGrantExtensionService struct {
	db *db.DB
}

func NewUserRoleGrantExtensionService(db *db.DB) *UserRoleGrantExtensionService {
	return &UserRoleGrantExtensionService{db: db}
}

func (r *UserRoleGrantExtensionService) GetDB() *db.DB { return r.db }

func (r *UserRoleGrantExtensionService) Create(ctx context.Context, e *models.UserRoleGrantExtension) error {
	res, err := r.db.GetDB().NewInsert().Model(e).Exec(ctx)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *UserRoleGrantExtensionService) CreateAdmin(ctx context.Context, grantID uuid.UUID, days int) error {
	e := &models.UserRoleGrantExtension{
		ID:              uuid.New(),
		UserRoleGrantID: grantID,
		Kind:            models.ExtensionKindAdmin,
		ExtensionDays:   days,
		CreatedAt:       time.Now(),
	}
	return r.Create(ctx, e)
}

func (r *UserRoleGrantExtensionService) CreateGrace(ctx context.Context, grantID uuid.UUID, days int, subscriptionID *uuid.UUID) error {
	e := &models.UserRoleGrantExtension{
		ID:              uuid.New(),
		UserRoleGrantID: grantID,
		Kind:            models.ExtensionKindGrace,
		ExtensionDays:   days,
		SubscriptionID:  subscriptionID,
		CreatedAt:       time.Now(),
	}
	return r.Create(ctx, e)
}

// SumGraceSince sums grace extension days for a subscription after a timestamp
func (r *UserRoleGrantExtensionService) SumGraceSince(ctx context.Context, subscriptionID uuid.UUID, since *time.Time) (int, error) {
	db := r.db.GetDB()
	q := db.NewSelect().
		ColumnExpr("COALESCE(SUM(extension_days),0)").
		TableExpr("user_role_grant_extensions").
		Where("subscription_id = ?", subscriptionID).
		Where("kind = ?", models.ExtensionKindGrace)
	if since != nil {
		q = q.Where("created_at > ?", *since)
	}
	var total int
	if err := q.Scan(ctx, &total); err != nil {
		return 0, err
	}
	return total, nil
}

// ExistsGraceToday checks if a grace extension for this subscription exists today
func (r *UserRoleGrantExtensionService) ExistsGraceToday(ctx context.Context, subscriptionID uuid.UUID) (bool, error) {
	db := r.db.GetDB()
	count, err := db.NewSelect().
		TableExpr("user_role_grant_extensions").
		Where("subscription_id = ?", subscriptionID).
		Where("kind = ?", models.ExtensionKindGrace).
		Where("created_at::date = CURRENT_DATE").
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
