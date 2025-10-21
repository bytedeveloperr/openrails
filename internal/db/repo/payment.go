package repo

import (
	"context"
	"errors"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
)

type PaymentFilters struct {
	UserID    string
	PriceID   uuid.UUID
	Processor string
	StartDate *time.Time
	EndDate   *time.Time
	MinAmount *float64
	MaxAmount *float64
}

type PaymentRepo struct {
	db *db.DB
}

func NewPaymentRepo(d *db.DB) *PaymentRepo { return &PaymentRepo{db: d} }

func (r *PaymentRepo) Create(ctx context.Context, payment *models.Payment) error {
	res, err := r.db.GetDB().NewInsert().Model(payment).TableExpr(r.db.QualifiedTable("payments")).Exec(ctx)
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

func (r *PaymentRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Payment, error) {
	payment := new(models.Payment)
	if err := r.db.GetDB().NewSelect().Model(payment).TableExpr(r.db.QualifiedTable("payments")).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *PaymentRepo) GetByUserID(ctx context.Context, userID string) ([]*models.Payment, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	payments := []*models.Payment{}
	if err := r.db.GetDB().NewSelect().Model(&payments).TableExpr(r.db.QualifiedTable("payments")).Where("user_id = ?", uid).Order("purchased_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return payments, nil
}

func (r *PaymentRepo) GetByTransactionID(ctx context.Context, processor models.Processor, transactionID string) (*models.Payment, error) {
	payment := new(models.Payment)
	if err := r.db.GetDB().NewSelect().Model(payment).TableExpr(r.db.QualifiedTable("payments")).Where("processor = ?", processor).Where("transaction_id = ?", transactionID).Scan(ctx); err != nil {
		return nil, err
	}
	return payment, nil
}

func (r *PaymentRepo) GetByPriceID(ctx context.Context, priceID uuid.UUID) ([]*models.Payment, error) {
	payments := []*models.Payment{}
	if err := r.db.GetDB().NewSelect().Model(&payments).TableExpr(r.db.QualifiedTable("payments")).Where("price_id = ?", priceID).Order("purchased_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return payments, nil
}

func (r *PaymentRepo) GetByProcessor(ctx context.Context, processor models.Processor) ([]*models.Payment, error) {
	payments := []*models.Payment{}
	if err := r.db.GetDB().NewSelect().Model(&payments).TableExpr(r.db.QualifiedTable("payments")).Where("processor = ?", processor).Order("purchased_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	return payments, nil
}

func (r *PaymentRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models.Payment)(nil)).TableExpr(r.db.QualifiedTable("payments")).Where("id = ?", id).Exec(ctx)
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

func (r *PaymentRepo) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]*models.Payment, int, error) {
	payments := []*models.Payment{}
	offset := (page - 1) * pageSize

	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, 0, err
	}

	count, err := r.db.GetDB().NewSelect().Model((*models.Payment)(nil)).TableExpr(r.db.QualifiedTable("payments")).Where("user_id = ?", uid).Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if err := r.db.GetDB().NewSelect().Model(&payments).TableExpr(r.db.QualifiedTable("payments")).Where("user_id = ?", uid).Order("purchased_at DESC").Limit(pageSize).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}

	return payments, count, nil
}

func (r *PaymentRepo) GetPayments(ctx context.Context, opts query.QueryOptions[PaymentFilters]) ([]*models.Payment, int64, error) {
	payments := []*models.Payment{}
	q := r.db.GetDB().NewSelect().Model(&payments).
		TableExpr(r.db.QualifiedTable("payments"))

	q = q.Relation("Price").Relation("Price.Product")

	if opts.Filters.UserID != "" {
		if uid, err := uuid.Parse(opts.Filters.UserID); err == nil {
			q = q.Where("payments.user_id = ?", uid)
		} else {
			return nil, 0, err
		}
	}
	if opts.Filters.PriceID != uuid.Nil {
		q = q.Where("payments.price_id = ?", opts.Filters.PriceID)
	}
	if opts.Filters.Processor != "" {
		q = q.Where("payments.processor = ?", opts.Filters.Processor)
	}
	if opts.Filters.StartDate != nil {
		q = q.Where("payments.purchased_at >= ?", opts.Filters.StartDate)
	}
	if opts.Filters.EndDate != nil {
		q = q.Where("payments.purchased_at <= ?", opts.Filters.EndDate)
	}
	if opts.Filters.MinAmount != nil {
		q = q.Where("payments.amount >= ?", opts.Filters.MinAmount)
	}
	if opts.Filters.MaxAmount != nil {
		q = q.Where("payments.amount <= ?", opts.Filters.MaxAmount)
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	q = q.Limit(opts.GetLimit()).Offset(opts.GetOffset()).Order("payments.purchased_at DESC")

	if err := q.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return payments, int64(total), nil
}
