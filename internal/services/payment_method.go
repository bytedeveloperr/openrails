package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

func (r *PaymentMethodService) GetByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).Where("user_id = ?", userID).Order("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func (r *PaymentMethodService) GetActiveByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
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

// GetByVaultID finds a Mobius payment method by vault ID
func (r *PaymentMethodService) GetByVaultID(ctx context.Context, vaultID string) (*models.PaymentMethod, error) {
	var method models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&method).
		Where("processor = ?", models.ProcessorMobius).
		Where("vault_id = ?", vaultID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &method, nil
}

// GetByBillingID is no longer needed since payment methods only support Mobius
// Keeping for backwards compatibility, but always filters for Mobius processor
func (r *PaymentMethodService) GetByBillingID(ctx context.Context, billingID string) (*models.PaymentMethod, error) {
	var method models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&method).
		Where("processor = ?", models.ProcessorMobius).
		Where("billing_id = ?", billingID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &method, nil
}

// GetByInitialTransactionID finds a Mobius payment method by initial transaction ID
func (r *PaymentMethodService) GetByInitialTransactionID(ctx context.Context, initialTransactionID string) (*models.PaymentMethod, error) {
	var method models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&method).
		Where("processor = ?", models.ProcessorMobius).
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
func (r *PaymentMethodService) DeactivateByUserID(ctx context.Context, userID string) error {
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

// GetAllMobius returns all Mobius payment methods
func (r *PaymentMethodService) GetAllMobius(ctx context.Context) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("processor = ?", models.ProcessorMobius).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// GetActiveMobius returns all active Mobius payment methods
func (r *PaymentMethodService) GetActiveMobius(ctx context.Context) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("processor = ?", models.ProcessorMobius).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// GetMobiusByUserID returns Mobius payment methods for a specific user
func (r *PaymentMethodService) GetMobiusByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
		Where("user_id = ?", userID).
		Where("processor = ?", models.ProcessorMobius).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// GetActiveMobiusByUserID returns active Mobius payment methods for a specific user
func (r *PaymentMethodService) GetActiveMobiusByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	var methods []*models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&methods).
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

// GetACUPendingMethods is deprecated since payment methods only support Mobius
// ACU fields were removed from the model, so this returns empty
func (r *PaymentMethodService) GetACUPendingMethods(ctx context.Context) ([]*models.PaymentMethod, error) {
	// ACU fields were removed from the payment method model
	// Return empty slice since we don't track ACU status anymore
	return []*models.PaymentMethod{}, nil
}

// ValidateOwnership verifies that a payment method belongs to the specified user
// Returns error if the payment method doesn't exist or doesn't belong to the user
func (r *PaymentMethodService) ValidateOwnership(ctx context.Context, id uuid.UUID, userID string) error {
	if id == uuid.Nil {
		return errors.New("invalid payment method ID")
	}

	if userID == "" {
		return errors.New("user ID is required")
	}

	count, err := r.db.GetDB().NewSelect().
		Model((*models.PaymentMethod)(nil)).
		Where("id = ?", id).
		Where("user_id = ?", userID).
		Count(ctx)

	if err != nil {
		return fmt.Errorf("failed to validate ownership: %w", err)
	}

	if count == 0 {
		return errors.New("payment method not found or access denied")
	}

	return nil
}

// ValidatePaymentMethodOperation performs general validation for payment method operations
func (r *PaymentMethodService) ValidatePaymentMethodOperation(ctx context.Context, id uuid.UUID, userID string) (*models.PaymentMethod, error) {
	// Validate input parameters
	if id == uuid.Nil {
		return nil, errors.New("invalid payment method ID")
	}

	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	// Get the payment method
	paymentMethod, err := r.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("payment method not found")
		}
		return nil, fmt.Errorf("failed to get payment method: %w", err)
	}

	// Validate ownership
	if err := r.ValidateOwnership(ctx, id, userID); err != nil {
		return nil, err
	}

	return paymentMethod, nil
}
