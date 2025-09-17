package services

import (
	"context"
	"errors"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type PriceService struct {
	db *db.DB
}

func NewPriceService(db *db.DB) *PriceService {
	return &PriceService{db: db}
}

func (r *PriceService) GetDB() *db.DB {
	return r.db
}

func (r *PriceService) Create(ctx context.Context, price *models.Price) error {
	result, err := r.db.GetDB().NewInsert().Model(price).Exec(ctx)
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

func (r *PriceService) GetByID(ctx context.Context, id uuid.UUID) (*models.Price, error) {
	var price models.Price
	if err := r.db.GetDB().NewSelect().Model(&price).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return &price, nil
}

func (r *PriceService) GetByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	var prices []*models.Price
	err := r.db.GetDB().NewSelect().Model(&prices).Where("product_id = ?", productID).Where("is_active = ?", true).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return prices, nil
}

func (r *PriceService) GetByMobiusPlanID(ctx context.Context, mobiusPlanID string) (*models.Price, error) {
	var price models.Price
	if err := r.db.GetDB().
		NewSelect().
		Model(&price).
		Where("mobius_plan_id = ?", mobiusPlanID).
		Where("is_active = ?", true).
		Scan(ctx); err != nil {
		return nil, err
	}

	return &price, nil
}

func (r *PriceService) GetByCCBillPriceID(ctx context.Context, ccbillPriceID string) (*models.Price, error) {
	var price models.Price
	err := r.db.GetDB().NewSelect().Model(&price).Relation("Product").Where("ccbill_price_id = ?", ccbillPriceID).Where("price.is_active = ?", true).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &price, nil
}

func (r *PriceService) GetActiveByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	var prices []*models.Price
	err := r.db.GetDB().NewSelect().Model(&prices).Where("product_id = ?", productID).Where("is_active = ?", true).Order("amount ASC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return prices, nil
}

func (r *PriceService) Update(ctx context.Context, price *models.Price) error {
	result, err := r.db.GetDB().NewUpdate().Model(price).WherePK().Exec(ctx)
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

func (r *PriceService) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.GetDB().NewDelete().Model((*models.Price)(nil)).Where("id = ?", id).Exec(ctx)
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
