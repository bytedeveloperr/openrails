package repo

import (
	"context"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type PaymentMethodRepo struct {
	db *db.DB
}

func NewPaymentMethodRepo(d *db.DB) *PaymentMethodRepo { return &PaymentMethodRepo{db: d} }

func (r *PaymentMethodRepo) Create(ctx context.Context, m *models.PaymentMethod) error {
	_, err := r.db.GetDB().NewInsert().Model(m).TableExpr(r.db.QualifiedTable("payment_methods")).Exec(ctx)
	return err
}

func (r *PaymentMethodRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.PaymentMethod, error) {
	pm := new(models.PaymentMethod)
	err := r.db.GetDB().NewSelect().Model(pm).TableExpr(r.db.QualifiedTable("payment_methods")).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.GetDB().NewDelete().Model((*models.PaymentMethod)(nil)).TableExpr(r.db.QualifiedTable("payment_methods")).Where("id = ?", id).Exec(ctx)
	return err
}
