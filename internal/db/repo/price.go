package repo

import (
	"context"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type PriceRepo struct {
	db *db.DB
}

func NewPriceRepo(d *db.DB) *PriceRepo { return &PriceRepo{db: d} }

func (r *PriceRepo) Create(ctx context.Context, p *models.Price) error {
	_, err := r.db.GetDB().NewInsert().Model(p).TableExpr(r.db.QualifiedTable("prices")).Exec(ctx)
	return err
}

func (r *PriceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.GetDB().NewDelete().Model((*models.Price)(nil)).TableExpr(r.db.QualifiedTable("prices")).Where("id = ?", id).Exec(ctx)
	return err
}
