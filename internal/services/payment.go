package services

import (
	"context"
	"errors"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
)

type PaymentService struct {
	db *db.DB
}

type GetPaymentsFilters struct {
	UserID    string    `form:"user_id"`
	PriceID   uuid.UUID `form:"price_id"`
	Processor string    `form:"processor"`
	// Optional filters for user-facing billing history
	StartDate *time.Time `form:"start_date"`
	EndDate   *time.Time `form:"end_date"`
	MinAmount *float64   `form:"min_amount"`
	MaxAmount *float64   `form:"max_amount"`
}

func NewPaymentService(db *db.DB) *PaymentService {
	return &PaymentService{db: db}
}

func (r *PaymentService) GetDB() *db.DB {
	return r.db
}

func (r *PaymentService) Create(ctx context.Context, payment *models.Payment) error {
	result, err := r.db.GetDB().NewInsert().Model(payment).Exec(ctx)
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

func (r *PaymentService) GetByID(ctx context.Context, id uuid.UUID) (*models.Payment, error) {
	var payment models.Payment
	err := r.db.GetDB().NewSelect().Model(&payment).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

func (r *PaymentService) GetByUserID(ctx context.Context, userID string) ([]*models.Payment, error) {
	var payments []*models.Payment
	err := r.db.GetDB().NewSelect().Model(&payments).Where("user_id = ?", userID).Order("purchased_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return payments, nil
}

func (r *PaymentService) GetByTransactionID(ctx context.Context, processor models.Processor, transactionID string) (*models.Payment, error) {
	var payment models.Payment
	err := r.db.GetDB().NewSelect().Model(&payment).
		Where("processor = ?", processor).
		Where("transaction_id = ?", transactionID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

func (r *PaymentService) GetByPriceID(ctx context.Context, priceID uuid.UUID) ([]*models.Payment, error) {
	var payments []*models.Payment
	err := r.db.GetDB().NewSelect().Model(&payments).Where("price_id = ?", priceID).Order("purchased_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return payments, nil
}

func (r *PaymentService) GetByProcessor(ctx context.Context, processor models.Processor) ([]*models.Payment, error) {
	var payments []*models.Payment
	err := r.db.GetDB().NewSelect().Model(&payments).Where("processor = ?", processor).Order("purchased_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return payments, nil
}

// Deprecated: role-grant linkage removed; entitlements are linked via source_id
// func (r *PaymentService) GetByGrantID(ctx context.Context, grantID uuid.UUID) ([]*models.Payment, error) { return nil, nil }

// GetAdminActionsByGrantID returns admin/internal adjustments linked to a grant
// Removed: GetAdminActionsByGrantID — admin/grace adjustments are tracked in user_role_grant_extensions

func (r *PaymentService) Update(ctx context.Context, payment *models.Payment) error {
	return errors.New("payments are immutable; updates are not supported")
}

func (r *PaymentService) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.GetDB().NewDelete().Model((*models.Payment)(nil)).Where("id = ?", id).Exec(ctx)
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

// Refund records a refund as a negative payment entry linked by transaction ID
// Note: Processors should handle the actual money movement; this persists the event.
func (r *PaymentService) Refund(ctx context.Context, originalPaymentID uuid.UUID, refundTransactionID string, amount float64) (*models.Payment, error) {
	// Load original payment to mirror key fields
	orig, err := r.GetByID(ctx, originalPaymentID)
	if err != nil {
		return nil, err
	}
	if amount <= 0 {
		return nil, errors.New("refund amount must be > 0")
	}

	refund := &models.Payment{
		ID:             uuid.New(),
		UserID:         orig.UserID,
		PriceID:        orig.PriceID,
		SubscriptionID: orig.SubscriptionID,
		Processor:      orig.Processor,
		TransactionID:  refundTransactionID,
		Amount:         -amount,
		Currency:       orig.Currency,
		PurchasedAt:    time.Now(),
		CreatedAt:      time.Now(),
	}
	if err := r.Create(ctx, refund); err != nil {
		return nil, err
	}
	return refund, nil
}

// GetPaginatedByUserID retrieves paginated payments for a user
func (r *PaymentService) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]*models.Payment, int, error) {
	var payments []*models.Payment
	offset := (page - 1) * pageSize

	count, err := r.db.GetDB().NewSelect().
		Model(&models.Payment{}).
		Where("user_id = ?", userID).
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	err = r.db.GetDB().NewSelect().
		Model(&payments).
		Where("user_id = ?", userID).
		Order("purchased_at DESC").
		Limit(pageSize).
		Offset(offset).
		Scan(ctx)
	if err != nil {
		return nil, 0, err
	}

	return payments, count, nil
}

// GetPayments retrieves payments with filtering and pagination
func (r *PaymentService) GetPayments(ctx context.Context, queryOpts query.QueryOptions[GetPaymentsFilters]) ([]*models.Payment, int64, error) {
	var payments []*models.Payment

	q := r.db.GetDB().NewSelect().Model(&payments).
		// User relation omitted - views.User requires complex JOINs
		Relation("Price").
		Relation("Price.Product")

		// Apply filters
	if queryOpts.Filters.UserID != "" {
		q = q.Where("payments.user_id = ?", queryOpts.Filters.UserID)
	}
	if queryOpts.Filters.PriceID != uuid.Nil {
		q = q.Where("payments.price_id = ?", queryOpts.Filters.PriceID)
	}
	if queryOpts.Filters.Processor != "" {
		q = q.Where("payments.processor = ?", queryOpts.Filters.Processor)
	}
	if queryOpts.Filters.StartDate != nil {
		q = q.Where("payments.purchased_at >= ?", queryOpts.Filters.StartDate)
	}
	if queryOpts.Filters.EndDate != nil {
		q = q.Where("payments.purchased_at <= ?", queryOpts.Filters.EndDate)
	}
	if queryOpts.Filters.MinAmount != nil {
		q = q.Where("payments.amount >= ?", queryOpts.Filters.MinAmount)
	}
	if queryOpts.Filters.MaxAmount != nil {
		q = q.Where("payments.amount <= ?", queryOpts.Filters.MaxAmount)
	}

	// Get total count
	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Apply pagination
	q = q.Limit(queryOpts.GetLimit()).Offset(queryOpts.GetOffset())

	// Apply ordering
	q = q.Order("payments.purchased_at DESC")

	err = q.Scan(ctx)
	if err != nil {
		return nil, 0, err
	}

	return payments, int64(total), nil
}
