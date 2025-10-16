package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/google/uuid"
)

type PaymentMethodService struct {
	repo *repo.PaymentMethodRepo
}

func NewPaymentMethodService(db *db.DB) *PaymentMethodService {
	return &PaymentMethodService{repo: repo.NewPaymentMethodRepo(db)}
}

func (s *PaymentMethodService) Create(ctx context.Context, method *models.PaymentMethod) error {
	return s.repo.Create(ctx, method)
}

func (s *PaymentMethodService) GetByID(ctx context.Context, id uuid.UUID) (*models.PaymentMethod, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *PaymentMethodService) GetByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *PaymentMethodService) GetActiveByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	return s.repo.GetActiveByUserID(ctx, userID)
}

// GetByVaultID finds a Mobius payment method by vault ID
func (s *PaymentMethodService) GetByVaultID(ctx context.Context, vaultID string) (*models.PaymentMethod, error) {
	return s.repo.GetByVaultID(ctx, vaultID)
}

// GetByBillingID is no longer needed since payment methods only support Mobius
// Keeping for backwards compatibility, but always filters for Mobius processor
func (s *PaymentMethodService) GetByBillingID(ctx context.Context, billingID string) (*models.PaymentMethod, error) {
	return s.repo.GetByBillingID(ctx, billingID)
}

// GetByInitialTransactionID finds a Mobius payment method by initial transaction ID
func (s *PaymentMethodService) GetByInitialTransactionID(ctx context.Context, initialTransactionID string) (*models.PaymentMethod, error) {
	return s.repo.GetByInitialTransactionID(ctx, initialTransactionID)
}

func (s *PaymentMethodService) Update(ctx context.Context, method *models.PaymentMethod) error {
	return s.repo.Update(ctx, method)
}

func (s *PaymentMethodService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// DeactivateByUserID deactivates all payment methods for a user
func (s *PaymentMethodService) DeactivateByUserID(ctx context.Context, userID string) error {
	return s.repo.DeactivateByUserID(ctx, userID)
}

// ActivateByID activates a specific payment method
func (s *PaymentMethodService) ActivateByID(ctx context.Context, id uuid.UUID) error {
	return s.repo.ActivateByID(ctx, id)
}

// GetAllMobius returns all Mobius payment methods
func (s *PaymentMethodService) GetAllMobius(ctx context.Context) ([]*models.PaymentMethod, error) {
	return s.repo.GetAllMobius(ctx)
}

// GetActiveMobius returns all active Mobius payment methods
func (s *PaymentMethodService) GetActiveMobius(ctx context.Context) ([]*models.PaymentMethod, error) {
	return s.repo.GetActiveMobius(ctx)
}

// GetMobiusByUserID returns Mobius payment methods for a specific user
func (s *PaymentMethodService) GetMobiusByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	return s.repo.GetMobiusByUserID(ctx, userID)
}

// GetActiveMobiusByUserID returns active Mobius payment methods for a specific user
func (s *PaymentMethodService) GetActiveMobiusByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	return s.repo.GetActiveMobiusByUserID(ctx, userID)
}

// GetACUPendingMethods is deprecated since payment methods only support Mobius
// ACU fields were removed from the model, so this returns empty
func (s *PaymentMethodService) GetACUPendingMethods(ctx context.Context) ([]*models.PaymentMethod, error) {
	return []*models.PaymentMethod{}, nil
}

// ValidateOwnership verifies that a payment method belongs to the specified user
// Returns error if the payment method doesn't exist or doesn't belong to the user
func (s *PaymentMethodService) ValidateOwnership(ctx context.Context, id uuid.UUID, userID string) error {
	if id == uuid.Nil {
		return errors.New("invalid payment method ID")
	}

	if userID == "" {
		return errors.New("user ID is required")
	}

	exists, err := s.repo.ExistsForUser(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("failed to validate ownership: %w", err)
	}

	if !exists {
		return errors.New("payment method not found or access denied")
	}

	return nil
}

// ValidatePaymentMethodOperation performs general validation for payment method operations
func (s *PaymentMethodService) ValidatePaymentMethodOperation(ctx context.Context, id uuid.UUID, userID string) (*models.PaymentMethod, error) {
	// Validate input parameters
	if id == uuid.Nil {
		return nil, errors.New("invalid payment method ID")
	}

	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	// Get the payment method
	paymentMethod, err := s.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("payment method not found")
		}
		return nil, fmt.Errorf("failed to get payment method: %w", err)
	}

	// Validate ownership
	if err := s.ValidateOwnership(ctx, id, userID); err != nil {
		return nil, err
	}

	return paymentMethod, nil
}
