package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type PaymentMethodRepo struct {
	db *db.DB
}

func NewPaymentMethodRepo(d *db.DB) *PaymentMethodRepo { return &PaymentMethodRepo{db: d} }

var (
	ErrPaymentMethodNotFound = errors.New("payment method not found")
)

func (r *PaymentMethodRepo) Create(ctx context.Context, m *models.PaymentMethod) error {
	res, err := r.db.GetDB().NewInsert().Model(m).TableExpr(r.db.QualifiedTable("payment_methods")).Exec(ctx)
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

func (r *PaymentMethodRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.PaymentMethod, error) {
	pm := new(models.PaymentMethod)
	err := r.db.GetDB().NewSelect().Model(pm).TableExpr(r.db.QualifiedTable("payment_methods")).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("payment method %s: %w", id, ErrPaymentMethodNotFound)
		}
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewDelete().Model((*models.PaymentMethod)(nil)).TableExpr(r.db.QualifiedTable("payment_methods")).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return ErrPaymentMethodNotFound
	}

	return nil
}

func (r *PaymentMethodRepo) GetByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	methods := []*models.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodRepo) GetActiveByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	methods := []*models.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("user_id = ?", userID).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodRepo) ListByUserID(ctx context.Context, userID string, includeInactive bool, limit, offset int) ([]*models.PaymentMethod, int64, error) {
	countQuery := r.db.GetDB().NewSelect().Model((*models.PaymentMethod)(nil)).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("user_id = ?", userID)
	if !includeInactive {
		countQuery.Where("is_active = ?", true)
	}

	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	methods := []*models.PaymentMethod{}
	dataQuery := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("user_id = ?", userID).
		Order("created_at DESC")
	if !includeInactive {
		dataQuery.Where("is_active = ?", true)
	}
	if limit > 0 {
		dataQuery.Limit(limit)
	}
	if offset > 0 {
		dataQuery.Offset(offset)
	}

	if err := dataQuery.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return methods, int64(total), nil
}

func (r *PaymentMethodRepo) GetByVaultID(ctx context.Context, vaultID string) (*models.PaymentMethod, error) {
	pm := new(models.PaymentMethod)
	err := r.db.GetDB().NewSelect().Model(pm).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("processor = ?", models.ProcessorMobius).
		Where("vault_id = ?", vaultID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodRepo) GetByBillingID(ctx context.Context, billingID string) (*models.PaymentMethod, error) {
	pm := new(models.PaymentMethod)
	err := r.db.GetDB().NewSelect().Model(pm).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("processor = ?", models.ProcessorMobius).
		Where("billing_id = ?", billingID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodRepo) GetByInitialTransactionID(ctx context.Context, initialTransactionID string) (*models.PaymentMethod, error) {
	pm := new(models.PaymentMethod)
	err := r.db.GetDB().NewSelect().Model(pm).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("processor = ?", models.ProcessorMobius).
		Where("initial_transaction_id = ?", initialTransactionID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodRepo) Update(ctx context.Context, method *models.PaymentMethod) error {
	res, err := r.db.GetDB().NewUpdate().Model(method).TableExpr(r.db.QualifiedTable("payment_methods")).WherePK().Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return ErrPaymentMethodNotFound
	}

	return nil
}

func (r *PaymentMethodRepo) DeactivateByUserID(ctx context.Context, userID string) error {
	res, err := r.db.GetDB().NewUpdate().
		Model((*models.PaymentMethod)(nil)).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Set("is_active = ?", false).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = res.RowsAffected()
	if err != nil {
		return err
	}

	return nil
}

func (r *PaymentMethodRepo) ActivateByID(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.GetDB().NewUpdate().
		Model((*models.PaymentMethod)(nil)).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Set("is_active = ?", true).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows < 1 {
		return ErrPaymentMethodNotFound
	}

	return nil
}

func (r *PaymentMethodRepo) GetAllMobius(ctx context.Context) ([]*models.PaymentMethod, error) {
	methods := []*models.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("processor = ?", models.ProcessorMobius).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodRepo) GetActiveMobius(ctx context.Context) ([]*models.PaymentMethod, error) {
	methods := []*models.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("processor = ?", models.ProcessorMobius).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodRepo) GetMobiusByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	methods := []*models.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("user_id = ?", userID).
		Where("processor = ?", models.ProcessorMobius).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodRepo) GetActiveMobiusByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	methods := []*models.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("user_id = ?", userID).
		Where("processor = ?", models.ProcessorMobius).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodRepo) ExistsForUser(ctx context.Context, id uuid.UUID, userID string) (bool, error) {
	count, err := r.db.GetDB().NewSelect().
		Model((*models.PaymentMethod)(nil)).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("id = ?", id).
		Where("user_id = ?", userID).
		Count(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PaymentMethodRepo) WithTx(txdb *db.DB) *PaymentMethodRepo {
	return NewPaymentMethodRepo(txdb)
}

func (r *PaymentMethodRepo) GetByProcessor(ctx context.Context, processor models.Processor) ([]*models.PaymentMethod, error) {
	methods := []*models.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("processor = ?", processor).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodRepo) GetActiveByProcessor(ctx context.Context, processor models.Processor) ([]*models.PaymentMethod, error) {
	methods := []*models.PaymentMethod{}
	err := r.db.GetDB().NewSelect().Model(&methods).
		TableExpr(r.db.QualifiedTable("payment_methods")).
		Where("processor = ?", processor).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodRepo) RequireByID(ctx context.Context, id uuid.UUID) (*models.PaymentMethod, error) {
	pm, err := r.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrPaymentMethodNotFound) {
			return nil, err
		}
		return nil, err
	}
	return pm, nil
}
