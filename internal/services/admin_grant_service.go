package services

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

// AdminGrantRequest represents a request to grant a product to a user
type AdminGrantRequest struct {
	UserID       string    // User receiving the grant
	PriceID      uuid.UUID // Price/Product being granted
	GrantedBy    string    // Admin user ID making the grant
	Reason       string    // Reason for grant (comp, contest_winner, etc.)
	DurationDays *int      // Optional override: nil=use spec, 0=indefinite, N=N days

	// Optional payment info (only if money was received)
	Amount        int64  // Amount in cents (0 = free comp)
	Currency      string // Currency code (defaults to price.Currency)
	TransactionID string // External reference (PayPal ID, cash receipt, etc.)
}

// AdminGrantResponse represents the result of an admin grant
type AdminGrantResponse struct {
	AdminGrantID        uuid.UUID  // Created admin grant ID
	PaymentID           *uuid.UUID // Created payment ID (if amount > 0)
	EntitlementsGranted []string   // Names of entitlements granted
}

// AdminGrantService handles admin-initiated product grants
type AdminGrantService struct {
	repo               *repo.AdminGrantRepo
	priceService       *PriceService
	productService     *ProductService
	paymentService     *PaymentService
	entitlementService *EntitlementService
	Clock              clockwork.Clock
}

// NewAdminGrantService creates a new AdminGrantService
func NewAdminGrantService(
	db *db.DB,
	priceService *PriceService,
	productService *ProductService,
	paymentService *PaymentService,
	entitlementService *EntitlementService,
) *AdminGrantService {
	return &AdminGrantService{
		repo:               repo.NewAdminGrantRepo(db),
		priceService:       priceService,
		productService:     productService,
		paymentService:     paymentService,
		entitlementService: entitlementService,
	}
}

// SetClock sets the clock for this service. Used for testing.
func (s *AdminGrantService) SetClock(c clockwork.Clock) {
	s.Clock = c
}

// now returns the current time from the service's clock, or time.Now() if no clock is set.
func (s *AdminGrantService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now()
}

// Grant creates an admin grant for a user, optionally with a payment record
func (s *AdminGrantService) Grant(ctx context.Context, req *AdminGrantRequest) (*AdminGrantResponse, error) {
	// Validate required fields
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	if req.GrantedBy == "" {
		return nil, fmt.Errorf("granted_by is required")
	}
	if req.Reason == "" {
		return nil, fmt.Errorf("reason is required")
	}

	// Get price
	price, err := s.priceService.GetByID(ctx, req.PriceID)
	if err != nil {
		return nil, fmt.Errorf("price not found: %w", err)
	}

	// Get product
	product, err := s.productService.GetByID(ctx, price.ProductID)
	if err != nil {
		return nil, fmt.Errorf("product not found: %w", err)
	}

	now := s.now()
	var paymentID *uuid.UUID

	// Create payment record if amount > 0
	if req.Amount > 0 {
		currency := req.Currency
		if currency == "" {
			currency = price.Currency
		}

		transactionID := req.TransactionID
		if transactionID == "" {
			// Generate a transaction ID for tracking
			transactionID = fmt.Sprintf("admin-grant-%s", uuid.New().String()[:8])
		}

		payment := &models.Payment{
			ID:            uuid.New(),
			UserID:        req.UserID,
			PriceID:       price.ID,
			Processor:     models.ProcessorAdmin,
			TransactionID: transactionID,
			Amount:        req.Amount,
			Currency:      currency,
			PurchasedAt:   now,
			CreatedAt:     now,
		}

		if err := s.paymentService.Create(ctx, payment); err != nil {
			return nil, fmt.Errorf("failed to create payment record: %w", err)
		}

		paymentID = &payment.ID

		log.WithFields(log.Fields{
			"payment_id":     payment.ID,
			"user_id":        req.UserID,
			"amount":         req.Amount,
			"currency":       currency,
			"transaction_id": transactionID,
			"granted_by":     req.GrantedBy,
		}).Info("created payment record for admin grant")
	}

	// Create admin grant record
	adminGrant := &models.AdminGrant{
		ID:           uuid.New(),
		UserID:       req.UserID,
		PriceID:      price.ID,
		GrantedBy:    req.GrantedBy,
		Reason:       req.Reason,
		PaymentID:    paymentID,
		DurationDays: req.DurationDays,
		CreatedAt:    now,
	}

	if err := s.repo.Create(ctx, adminGrant); err != nil {
		return nil, fmt.Errorf("failed to create admin grant record: %w", err)
	}

	// Grant entitlements from product spec
	var grantedEntitlements []string

	if product.EntitlementsSpec != nil {
		for entitlementName, specDurationDays := range product.EntitlementsSpec {
			var endAt *time.Time

			// Determine duration: request override > product spec
			if req.DurationDays != nil {
				if *req.DurationDays > 0 {
					// Explicit duration override
					end := now.Add(time.Duration(*req.DurationDays) * 24 * time.Hour)
					endAt = &end
				}
				// If *req.DurationDays == 0, endAt stays nil (indefinite)
			} else if specDurationDays != nil && *specDurationDays > 0 {
				// Use product spec duration
				end := now.Add(time.Duration(*specDurationDays) * 24 * time.Hour)
				endAt = &end
			}
			// If both are nil/0, endAt stays nil (indefinite)

			_, err := s.entitlementService.GrantWindow(
				ctx,
				req.UserID,
				entitlementName,
				now,
				endAt,
				models.EntitlementSourceAdmin,
				&adminGrant.ID, // Source ID points to AdminGrant
			)
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"user_id":        req.UserID,
					"entitlement":    entitlementName,
					"admin_grant_id": adminGrant.ID,
				}).Error("failed to grant entitlement")
				// Continue granting other entitlements even if one fails
				continue
			}

			grantedEntitlements = append(grantedEntitlements, entitlementName)

			log.WithFields(log.Fields{
				"user_id":        req.UserID,
				"entitlement":    entitlementName,
				"admin_grant_id": adminGrant.ID,
				"end_at":         endAt,
				"granted_by":     req.GrantedBy,
			}).Info("granted entitlement from admin grant")
		}
	}

	log.WithFields(log.Fields{
		"admin_grant_id": adminGrant.ID,
		"user_id":        req.UserID,
		"price_id":       price.ID,
		"product_id":     product.ID,
		"product_name":   product.DisplayName,
		"granted_by":     req.GrantedBy,
		"reason":         req.Reason,
		"payment_id":     paymentID,
		"entitlements":   grantedEntitlements,
	}).Info("admin grant completed")

	return &AdminGrantResponse{
		AdminGrantID:        adminGrant.ID,
		PaymentID:           paymentID,
		EntitlementsGranted: grantedEntitlements,
	}, nil
}

// GetByID retrieves an admin grant by ID
func (s *AdminGrantService) GetByID(ctx context.Context, id uuid.UUID) (*models.AdminGrant, error) {
	return s.repo.GetByID(ctx, id)
}

// ListByUserID retrieves all admin grants for a user
func (s *AdminGrantService) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]models.AdminGrant, int, error) {
	return s.repo.ListByUserID(ctx, userID, limit, offset)
}

// ListByGrantedBy retrieves all admin grants made by a specific admin
func (s *AdminGrantService) ListByGrantedBy(ctx context.Context, grantedBy string, limit, offset int) ([]models.AdminGrant, int, error) {
	return s.repo.ListByGrantedBy(ctx, grantedBy, limit, offset)
}
