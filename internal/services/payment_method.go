package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
func (r *PaymentMethodService) GetByUserIDAndProcessor(ctx context.Context, userID string, processor models.Processor) ([]*models.PaymentMethod, error) {
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
func (r *PaymentMethodService) GetActiveByUserIDAndProcessor(ctx context.Context, userID string, processor models.Processor) ([]*models.PaymentMethod, error) {
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

// CanDelete checks if a payment method can be deleted based on business rules
// Returns (canDelete, reason, error)
func (r *PaymentMethodService) CanDelete(ctx context.Context, id uuid.UUID) (bool, string, error) {
	if id == uuid.Nil {
		return false, "invalid payment method ID", nil
	}

	// Check if payment method exists
	paymentMethod, err := r.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, "payment method not found", nil
		}
		return false, "", fmt.Errorf("failed to get payment method: %w", err)
	}

	// Count active subscriptions using this payment method
	activeSubscriptionCount, err := r.db.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		Where("payment_method_id = ?", id).
		Where("status = ?", models.StatusActive).
		Count(ctx)

	if err != nil {
		return false, "", fmt.Errorf("failed to count active subscriptions: %w", err)
	}

	// Use the model's CanDelete method for business logic
	canDelete, reason := paymentMethod.CanDelete(activeSubscriptionCount)
	return canDelete, reason, nil
}

// GetDisplayName returns a user-friendly display name for a payment method
func (r *PaymentMethodService) GetDisplayName(pm *models.PaymentMethod) string {
	if pm == nil {
		return "Unknown Payment Method"
	}

	return pm.GetDisplayName()
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

// CreateFromSolanaWallet creates a payment method from a connected Solana wallet
func (r *PaymentMethodService) CreateFromSolanaWallet(ctx context.Context, userID string, walletAddress string) (*models.PaymentMethod, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	if walletAddress == "" {
		return nil, errors.New("wallet address is required")
	}

	// Check if payment method already exists for this wallet
	var existingMethod models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&existingMethod).
		Where("user_id = ?", userID).
		Where("processor = ?", models.ProcessorSolana).
		Where("wallet_address = ?", walletAddress).
		Scan(ctx)

	if err == nil {
		// Payment method already exists, return it
		return &existingMethod, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing payment method: %w", err)
	}

	// Create new payment method
	now := time.Now()
	paymentMethod := &models.PaymentMethod{
		ID:                   uuid.New(),
		UserID:               userID,
		Processor:            models.ProcessorSolana,
		VaultID:              walletAddress,       // Use wallet address as vault ID for Solana
		InitialTransactionID: "wallet_connection", // Placeholder for Solana wallets
		IsActive:             true,
		WalletAddress:        &walletAddress,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := r.Create(ctx, paymentMethod); err != nil {
		return nil, fmt.Errorf("failed to create Solana payment method: %w", err)
	}

	return paymentMethod, nil
}

// GetByWalletAddress finds a payment method by wallet address
func (r *PaymentMethodService) GetByWalletAddress(ctx context.Context, userID string, walletAddress string) (*models.PaymentMethod, error) {
	var method models.PaymentMethod
	err := r.db.GetDB().NewSelect().Model(&method).
		Where("user_id = ?", userID).
		Where("processor = ?", models.ProcessorSolana).
		Where("wallet_address = ?", walletAddress).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &method, nil
}

// CreatePaymentMethodsFromConnectedWallets creates payment methods for all verified Solana wallets
func (r *PaymentMethodService) CreatePaymentMethodsFromConnectedWallets(ctx context.Context, userID string, solanaWalletService *SolanaWalletService) ([]*models.PaymentMethod, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	if solanaWalletService == nil {
		return nil, errors.New("solana wallet service is required")
	}

	// Get all verified wallets for the user
	wallets, err := solanaWalletService.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user wallets: %w", err)
	}

	var paymentMethods []*models.PaymentMethod

	// Create payment methods for verified wallets
	for _, wallet := range wallets {
		if wallet.IsVerified {
			paymentMethod, err := r.CreateFromSolanaWallet(ctx, userID, wallet.Address)
			if err != nil {
				// Log error but continue with other wallets
				continue
			}
			paymentMethods = append(paymentMethods, paymentMethod)
		}
	}

	return paymentMethods, nil
}

// CreateFromCCBillWebhook creates a payment method from CCBill webhook data
func (r *PaymentMethodService) CreateFromCCBillWebhook(ctx context.Context, userID, vaultID, transactionID string) (*models.PaymentMethod, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	if vaultID == "" {
		return nil, errors.New("vault ID is required")
	}

	if transactionID == "" {
		return nil, errors.New("transaction ID is required")
	}

	// Check if payment method already exists for this vault ID
	existingMethod, err := r.GetByVaultID(ctx, models.ProcessorCCBill, vaultID)
	if err == nil {
		// Payment method already exists, update it if needed
		if existingMethod.UserID != userID {
			return nil, fmt.Errorf("vault ID %s already associated with different user", vaultID)
		}

		// Update the initial transaction ID if it's different
		if existingMethod.InitialTransactionID != transactionID {
			existingMethod.InitialTransactionID = transactionID
			existingMethod.UpdatedAt = time.Now()
			if err = r.Update(ctx, existingMethod); err != nil {
				return nil, fmt.Errorf("failed to update existing payment method: %w", err)
			}
		}

		return existingMethod, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing payment method: %w", err)
	}

	// Create new payment method
	now := time.Now()
	paymentMethod := &models.PaymentMethod{
		ID:                   uuid.New(),
		UserID:               userID,
		Processor:            models.ProcessorCCBill,
		VaultID:              vaultID,
		InitialTransactionID: transactionID,
		IsActive:             true,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := r.Create(ctx, paymentMethod); err != nil {
		return nil, fmt.Errorf("failed to create CCBill payment method: %w", err)
	}

	return paymentMethod, nil
}

// UpdatePaymentMethodStatus updates the status of a payment method based on webhook events
func (r *PaymentMethodService) UpdatePaymentMethodStatus(ctx context.Context, vaultID string, isActive bool, failureReason *string) error {
	if vaultID == "" {
		return errors.New("vault ID is required")
	}

	paymentMethod, err := r.GetByVaultID(ctx, models.ProcessorCCBill, vaultID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Payment method doesn't exist, nothing to update
			return nil
		}
		return fmt.Errorf("failed to get payment method: %w", err)
	}

	// Update status and failure reason
	paymentMethod.IsActive = isActive
	paymentMethod.FailureReason = failureReason
	paymentMethod.UpdatedAt = time.Now()

	if err := r.Update(ctx, paymentMethod); err != nil {
		return fmt.Errorf("failed to update payment method status: %w", err)
	}

	return nil
}
