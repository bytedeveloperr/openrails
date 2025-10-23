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
	res, err := r.db.GetDB().NewInsert().Model(price).TableExpr(r.db.QualifiedTable("prices")).Exec(ctx)
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
	if err := r.db.GetDB().NewSelect().Model(price).TableExpr(r.db.QualifiedTable("prices")).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return price, nil
}

func (r *PriceRepo) GetByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	prices := []*models.Price{}
	if err := r.db.GetDB().NewSelect().Model(&prices).TableExpr(r.db.QualifiedTable("prices")).Where("product_id = ?", productID).Where("is_active = ?", true).Scan(ctx); err != nil {
		return nil, err
	}
	return prices, nil
}

func (r *PriceRepo) GetActiveByProductID(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	prices := []*models.Price{}
	if err := r.db.GetDB().NewSelect().Model(&prices).TableExpr(r.db.QualifiedTable("prices")).Where("product_id = ?", productID).Where("is_active = ?", true).Order("amount ASC").Scan(ctx); err != nil {
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

	query := r.db.GetDB().NewSelect().Model(price).TableExpr(r.db.QualifiedTable("prices")).
		Where("nmi_plan_id = ?", nmiPlanID).
		Where("is_active = ?", true)

	if provider == "mobius" {
		query = query.Where("(nmi_provider = ? OR nmi_provider IS NULL OR nmi_provider = '')", provider)
	} else {
		query = query.Where("nmi_provider = ?", provider)
	}

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}
	return price, nil
}

func (r *PriceRepo) GetByCCBillPriceID(ctx context.Context, ccbillPriceID string) (*models.Price, error) {
	price := new(models.Price)
	if err := r.db.GetDB().NewSelect().Model(price).TableExpr(r.db.QualifiedTable("prices")).Relation("Product").Where("ccbill_price_id = ?", ccbillPriceID).Where("price.is_active = ?", true).Scan(ctx); err != nil {
		return nil, err
	}
	return price, nil
}

func (r *PriceRepo) Update(ctx context.Context, price *models.Price) error {
	res, err := r.db.GetDB().NewUpdate().Model(price).TableExpr(r.db.QualifiedTable("prices")).WherePK().Exec(ctx)
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
	res, err := r.db.GetDB().NewDelete().Model((*models.Price)(nil)).TableExpr(r.db.QualifiedTable("prices")).Where("id = ?", id).Exec(ctx)
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
