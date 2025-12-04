package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
)

type PaymentService struct {
	repo *repo.PaymentRepo
}

type GetPaymentsFilters = repo.PaymentFilters

func NewPaymentService(db *db.DB) *PaymentService {
	return &PaymentService{repo: repo.NewPaymentRepo(db)}
}

func (s *PaymentService) Create(ctx context.Context, payment *models.Payment) error {
	return s.repo.Create(ctx, payment)
}

func (s *PaymentService) GetByID(ctx context.Context, id uuid.UUID) (*models.Payment, error) {
	return s.repo.GetByID(ctx, id)
}

// GetByIDWithDetails returns a payment with all related entities and any refund entries
func (s *PaymentService) GetByIDWithDetails(ctx context.Context, id uuid.UUID) (*models.Payment, []*models.Payment, error) {
	return s.repo.GetByIDWithDetails(ctx, id)
}

func (s *PaymentService) GetByUserID(ctx context.Context, userID string) ([]*models.Payment, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *PaymentService) GetByTransactionID(ctx context.Context, processor models.Processor, transactionID string) (*models.Payment, error) {
	return s.repo.GetByTransactionID(ctx, processor, transactionID)
}

func (s *PaymentService) GetByPriceID(ctx context.Context, priceID uuid.UUID) ([]*models.Payment, error) {
	return s.repo.GetByPriceID(ctx, priceID)
}

func (s *PaymentService) GetByProcessor(ctx context.Context, processor models.Processor) ([]*models.Payment, error) {
	return s.repo.GetByProcessor(ctx, processor)
}

func (s *PaymentService) Update(ctx context.Context, payment *models.Payment) error {
	return errors.New("payments are immutable; updates are not supported")
}

func (s *PaymentService) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.New("payments cannot be deleted")
}

// Refund records a refund as a negative payment entry linked by transaction ID
// Note: Processors should handle the actual money movement; this persists the event.
// amount is in cents (smallest currency unit)
func (s *PaymentService) Refund(ctx context.Context, originalPaymentID uuid.UUID, refundTransactionID string, amount int64) (*models.Payment, error) {
	orig, err := s.GetByID(ctx, originalPaymentID)
	if err != nil {
		return nil, err
	}
	if amount <= 0 {
		return nil, errors.New("refund amount must be > 0")
	}
	if strings.TrimSpace(refundTransactionID) == "" {
		return nil, errors.New("refund transaction id is required")
	}

	refundedTotal, err := s.repo.GetRefundTotalByPaymentID(ctx, originalPaymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate refunded total: %w", err)
	}
	if amount > orig.Amount {
		return nil, errors.New("refund amount cannot exceed original payment amount")
	}
	if refundedTotal > 0 {
		if amount+refundedTotal > orig.Amount {
			return nil, fmt.Errorf("refund total would exceed original payment (refunded %d of %d)", refundedTotal, orig.Amount)
		}
	}

	refund := &models.Payment{
		ID:             uuid.New(),
		UserID:         orig.UserID,
		PriceID:        orig.PriceID,
		SubscriptionID: orig.SubscriptionID,
		RefundedPaymentID: func() *uuid.UUID {
			id := orig.ID
			return &id
		}(),
		Processor:     orig.Processor,
		TransactionID: refundTransactionID,
		Amount:        -amount,
		Currency:      orig.Currency,
		PurchasedAt:   time.Now(),
		CreatedAt:     time.Now(),
	}
	if err := s.Create(ctx, refund); err != nil {
		return nil, err
	}
	return refund, nil
}

func (s *PaymentService) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]*models.Payment, int, error) {
	return s.repo.GetPaginatedByUserID(ctx, userID, page, pageSize)
}

func (s *PaymentService) GetPayments(ctx context.Context, queryOpts query.QueryOptions[GetPaymentsFilters]) ([]*models.Payment, int64, error) {
	repoOpts := query.QueryOptions[repo.PaymentFilters]{
		Filters:  queryOpts.Filters,
		Limit:    queryOpts.Limit,
		Offset:   queryOpts.Offset,
		Page:     queryOpts.Page,
		PageSize: queryOpts.PageSize,
		All:      queryOpts.All,
	}

	return s.repo.GetPayments(ctx, repoOpts)
}
