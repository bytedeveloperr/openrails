package services

import (
	"context"
	"errors"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type PaymentMethodService struct {
	db *db.DB
}

func NewPaymentMethodService(db *db.DB) *PaymentMethodService {
	return &PaymentMethodService{db: db}
}

func (r *PaymentMethodService) GetDB() *db.DB {
	return r.db
}

func (r *PaymentMethodService) Create(ctx context.Context, method *models.PaymentMethod) error {
	result, err := r.db.GetDB().NewInsert().Model(method).Exec(ctx)
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

func (r *PaymentMethodService) GetByID(ctx context.Context, id uuid.UUID) (*models.PaymentMethod, error) {
	var method models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&method).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &method, nil
}

func (r *PaymentMethodService) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).Where("user_id = ?", userID).Order("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodService) GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("user_id = ?", userID).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// GetByVaultID finds a payment method by processor and vault ID
func (r *PaymentMethodService) GetByVaultID(ctx context.Context, processor models.Processor, vaultID string) (*models.PaymentMethod, error) {
	var method models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&method).
		Where("processor = ?", processor).
		Where("vault_id = ?", vaultID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &method, nil
}

func (r *PaymentMethodService) GetByBillingID(ctx context.Context, processor models.Processor, billingID string) (*models.PaymentMethod, error) {
	var method models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&method).
		Where("processor = ?", processor).
		Where("billing_id = ?", billingID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &method, nil
}

func (r *PaymentMethodService) GetByInitialTransactionID(ctx context.Context, processor models.Processor, initialTransactionID string) (*models.PaymentMethod, error) {
	var method models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&method).
		Where("processor = ?", processor).
		Where("initial_transaction_id = ?", initialTransactionID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &method, nil
}

func (r *PaymentMethodService) Update(ctx context.Context, method *models.PaymentMethod) error {
	result, err := r.db.GetDB().NewUpdate().Model(method).WherePK().Exec(ctx)
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

func (r *PaymentMethodService) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.GetDB().NewDelete().Model((*models.PaymentMethod)(nil)).Where("id = ?", id).Exec(ctx)
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

// DeactivateByUserID deactivates all payment methods for a user
func (r *PaymentMethodService) DeactivateByUserID(ctx context.Context, userID uuid.UUID) error {
	result, err := r.db.GetDB().NewUpdate().
		Model((*models.PaymentMethod)(nil)).
		Set("is_active = ?", false).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = result.RowsAffected()
	if err != nil {
		return err
	}

	return nil
}

// ActivateByID activates a specific payment method
func (r *PaymentMethodService) ActivateByID(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.GetDB().NewUpdate().
		Model((*models.PaymentMethod)(nil)).
		Set("is_active = ?", true).
		Where("id = ?", id).
		Exec(ctx)
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

// GetByProcessor returns all payment methods for a specific processor
func (r *PaymentMethodService) GetByProcessor(ctx context.Context, processor models.Processor) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("processor = ?", processor).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// GetActiveByProcessor returns all active payment methods for a specific processor
func (r *PaymentMethodService) GetActiveByProcessor(ctx context.Context, processor models.Processor) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("processor = ?", processor).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// GetByUserIDAndProcessor returns payment methods for a specific user and processor
func (r *PaymentMethodService) GetByUserIDAndProcessor(ctx context.Context, userID uuid.UUID, processor models.Processor) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("user_id = ?", userID).
		Where("processor = ?", processor).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// GetActiveByUserIDAndProcessor returns active payment methods for a specific user and processor
func (r *PaymentMethodService) GetActiveByUserIDAndProcessor(ctx context.Context, userID uuid.UUID, processor models.Processor) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("user_id = ?", userID).
		Where("processor = ?", processor).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// GetACUPendingMethods returns payment methods that need ACU retry
// Note: ACU fields were removed from the model, so this now returns empty
func (r *PaymentMethodService) GetACUPendingMethods(ctx context.Context, processor models.Processor) ([]*models.PaymentMethod, error) {
	// ACU fields were removed from the payment method model
	// Return empty slice since we don't track ACU status anymore
	return []*models.PaymentMethod{}, nil
}
