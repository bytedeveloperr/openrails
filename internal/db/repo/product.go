package repo

import (
	"context"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type ProductRepo struct {
	db *db.DB
}

func NewProductRepo(d *db.DB) *ProductRepo { return &ProductRepo{db: d} }

func (r *ProductRepo) Create(ctx context.Context, p *models.Product) error {
	_, err := r.db.GetDB().NewInsert().Model(p).TableExpr(r.db.QualifiedTable("products")).Exec(ctx)
	return err
}

func (r *ProductRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.GetDB().NewDelete().Model((*models.Product)(nil)).TableExpr(r.db.QualifiedTable("products")).Where("id = ?", id).Exec(ctx)
	return err
}
