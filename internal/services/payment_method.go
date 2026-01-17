package services

import (
	"context"
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

var (
	ErrPaymentMethodNotFound     = errors.New("payment method not found")
	ErrPaymentMethodAccessDenied = errors.New("payment method access denied")
)

func (s *PaymentMethodService) Create(ctx context.Context, method *models.PaymentMethod) error {
	return s.repo.Create(ctx, method)
}

func (s *PaymentMethodService) GetByID(ctx context.Context, id uuid.UUID) (*models.PaymentMethod, error) {
	pm, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrPaymentMethodNotFound) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

func (s *PaymentMethodService) GetByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *PaymentMethodService) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.PaymentMethod, int64, error) {
	if userID == "" {
		return nil, 0, errors.New("user ID is required")
	}
	if limit < 1 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	items, total, err := s.repo.ListByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetByVaultID finds a NMI payment method by vault ID
func (s *PaymentMethodService) GetByVaultID(ctx context.Context, provider, vaultID string) (*models.PaymentMethod, error) {
	pm, err := s.repo.GetByVaultID(ctx, provider, vaultID)
	if err != nil {
		if errors.Is(err, repo.ErrPaymentMethodNotFound) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

// GetByBillingID is no longer needed since payment methods only support NMI
// Keeping for backwards compatibility, but always filters for NMI processor
func (s *PaymentMethodService) GetByBillingID(ctx context.Context, provider, billingID string) (*models.PaymentMethod, error) {
	pm, err := s.repo.GetByBillingID(ctx, provider, billingID)
	if err != nil {
		if errors.Is(err, repo.ErrPaymentMethodNotFound) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

// GetByInitialTransactionID finds a NMI payment method by initial transaction ID
func (s *PaymentMethodService) GetByInitialTransactionID(ctx context.Context, provider, initialTransactionID string) (*models.PaymentMethod, error) {
	pm, err := s.repo.GetByInitialTransactionID(ctx, provider, initialTransactionID)
	if err != nil {
		if errors.Is(err, repo.ErrPaymentMethodNotFound) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return pm, nil
}

func (s *PaymentMethodService) Update(ctx context.Context, method *models.PaymentMethod) error {
	if err := s.repo.Update(ctx, method); err != nil {
		if errors.Is(err, repo.ErrPaymentMethodNotFound) {
			return ErrPaymentMethodNotFound
		}
		return err
	}
	return nil
}

func (s *PaymentMethodService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repo.ErrPaymentMethodNotFound) {
			return ErrPaymentMethodNotFound
		}
		return err
	}
	return nil
}

// GetAllNMI returns all NMI payment methods
func (s *PaymentMethodService) GetAllNMI(ctx context.Context) ([]*models.PaymentMethod, error) {
	return s.repo.GetAllNMI(ctx)
}

// GetNMIByUserID returns NMI payment methods for a specific user
func (s *PaymentMethodService) GetNMIByUserID(ctx context.Context, userID string) ([]*models.PaymentMethod, error) {
	return s.repo.GetNMIByUserID(ctx, userID)
}

// GetACUPendingMethods is deprecated since payment methods only support NMI
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
		return ErrPaymentMethodAccessDenied
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
		if errors.Is(err, ErrPaymentMethodNotFound) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, fmt.Errorf("failed to get payment method: %w", err)
	}

	// Validate ownership
	if err := s.ValidateOwnership(ctx, id, userID); err != nil {
		if errors.Is(err, ErrPaymentMethodAccessDenied) {
			return nil, ErrPaymentMethodAccessDenied
		}
		return nil, err
	}

	return paymentMethod, nil
}
