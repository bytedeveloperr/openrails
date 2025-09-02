package services

import (
	"context"
	"errors"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type ProductService struct {
	db *db.DB
}

func NewProductService(db *db.DB) *ProductService {
	return &ProductService{db: db}
}

func (r *ProductService) GetDB() *db.DB {
	return r.db
}

func (r *ProductService) Create(ctx context.Context, product *models.Product) error {
	result, err := r.db.GetDB().NewInsert().Model(product).Exec(ctx)
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

func (r *ProductService) GetByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	var product models.Product
	err := r.db.GetDB().NewSelect().Model(&product).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ProductService) GetByRoleID(ctx context.Context, roleID uuid.UUID) (*models.Product, error) {
	var product models.Product
	err := r.db.GetDB().NewSelect().Model(&product).Where("role_id = ?", roleID).Where("is_active = ?", true).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *ProductService) GetActive(ctx context.Context) ([]*models.Product, error) {
	var products []*models.Product
	err := r.db.GetDB().NewSelect().Model(&products).Where("is_active = ?", true).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return products, nil
}

func (r *ProductService) Update(ctx context.Context, product *models.Product) error {
	result, err := r.db.GetDB().NewUpdate().Model(product).WherePK().Exec(ctx)
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

func (r *ProductService) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.GetDB().NewDelete().Model((*models.Product)(nil)).Where("id = ?", id).Exec(ctx)
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
