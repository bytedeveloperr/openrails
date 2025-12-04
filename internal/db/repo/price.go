package repo

import (
	"context"
	"errors"
	"strings"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type PriceRepo struct {
	db *db.DB
}

func NewPriceRepo(d *db.DB) *PriceRepo { return &PriceRepo{db: d} }

func (r *PriceRepo) Create(ctx context.Context, price *models.Price) error {
	res, err := r.db.GetDB().NewInsert().Model(price).Exec(ctx)
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

func (r *PriceRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Price, error) {
	price := new(models.Price)
	if err := r.db.GetDB().NewSelect().Model(price).Where("price.id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return price, nil
}

func (r *PriceRepo) GetByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	prices := []*models.Price{}
	if err := r.db.GetDB().NewSelect().Model(&prices).Where("price.product_id = ?", productID).Where("price.is_active = ?", true).Scan(ctx); err != nil {
		return nil, err
	}
	return prices, nil
}

func (r *PriceRepo) GetActiveByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	prices := []*models.Price{}
	if err := r.db.GetDB().NewSelect().Model(&prices).Where("price.product_id = ?", productID).Where("price.is_active = ?", true).OrderExpr("price.amount ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return prices, nil
}

func (r *PriceRepo) GetAllActive(ctx context.Context) ([]*models.Price, error) {
	prices := []*models.Price{}
	if err := r.db.GetDB().NewSelect().Model(&prices).Relation("Product").Where("price.is_active = ?", true).OrderExpr("price.amount ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return prices, nil
}

func (r *PriceRepo) GetAll(ctx context.Context) ([]*models.Price, error) {
	prices := []*models.Price{}
	if err := r.db.GetDB().NewSelect().Model(&prices).Relation("Product").OrderExpr("price.amount ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return prices, nil
}

func (r *PriceRepo) GetByNMIPlan(ctx context.Context, provider, nmiPlanID string) (*models.Price, error) {
	price := new(models.Price)
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = "mobius"
	}

	// Query using JSONB operators. Look for plan_id in either:
	// 1. processors->'mobius'->>'plan_id' (new format)
	// 2. processors->'nmi'->>'plan_id' (legacy format)
	// The provider parameter determines which processor key to look up directly (e.g., "mobius", "acme")
	query := r.db.GetDB().NewSelect().Model(price).
		Where("(price.processors->?->>'plan_id' = ? OR price.processors->'nmi'->>'plan_id' = ?)", provider, nmiPlanID, nmiPlanID).
		Where("price.is_active = ?", true)

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return price, nil
}

func (r *PriceRepo) GetByCCBillPriceID(ctx context.Context, ccbillPriceID string) (*models.Price, error) {
	price := new(models.Price)
	// Query using JSONB: processors->'ccbill'->>'price_id' = priceID
	if err := r.db.GetDB().NewSelect().Model(price).Relation("Product").
		Where("price.processors->'ccbill'->>'price_id' = ?", ccbillPriceID).
		Where("price.is_active = ?", true).
		Scan(ctx); err != nil {
		return nil, err
	}
	return price, nil
}

func (r *PriceRepo) Update(ctx context.Context, price *models.Price) error {
	res, err := r.db.GetDB().NewUpdate().Model(price).WherePK().Exec(ctx)
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

func (r *PriceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models.Price)(nil)).Where("price.id = ?", id).Exec(ctx)
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
