package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

var (
	ErrInvalidToken  = errors.New("invalid or unsupported token")
	ErrPriceNotFound = errors.New("price not found")
)

type SolanaPaymentService struct {
	db                 *db.DB
	cfg                *config.Config
	priceService       *PriceService
	paymentSvc         *PaymentService
	productService     *ProductService
	entitlementService *EntitlementService
	notificationSvc    *NotificationService
}

type oneOffNotificationData struct {
	UserID      string
	Amount      float64
	Currency    string
	ProductName string
}

func NewSolanaPaymentService(db *db.DB, cfg *config.Config, price *PriceService, payment *PaymentService, product *ProductService, entitlement *EntitlementService, notification *NotificationService) *SolanaPaymentService {
	return &SolanaPaymentService{
		db:                 db,
		cfg:                cfg,
		priceService:       price,
		paymentSvc:         payment,
		productService:     product,
		entitlementService: entitlement,
		notificationSvc:    notification,
	}
}

// SetNotificationService wires the notification service after SolanaPaymentService construction
func (s *SolanaPaymentService) SetNotificationService(notification *NotificationService) {
	s.notificationSvc = notification
}

// Generate returns payment calculation for client-side transaction building.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Generate(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, userWallet string) (amount float64, currency string, tokenAmount uint64, expiresAt time.Time, err error) {
	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return 0, "", 0, time.Time{}, fmt.Errorf("%w: %v", ErrPriceNotFound, err)
	}
	if s.cfg.Solana == nil {
		return 0, "", 0, time.Time{}, fmt.Errorf("solana not configured")
	}
	tok, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tok.Enabled {
		return 0, "", 0, time.Time{}, ErrInvalidToken
	}
	mainnetMint := tok.MainnetMint
	if mainnetMint == "" {
		mainnetMint = tok.Mint
	}
	if mainnetMint == "" {
		return 0, "", 0, time.Time{}, fmt.Errorf("token %s missing mint configuration", tokenSymbol)
	}

	tokenUnits, _, err := calculateTokenQuote(ctx, tok, price.Amount)
	if err != nil {
		return 0, "", 0, time.Time{}, err
	}
	exp := time.Now().Add(10 * time.Minute)

	// Disallow one-off if a subscription entitlement is already active (indefinite)
	if userID != "" {
		exists, _ := s.db.GetDB().NewSelect().
			Model((*models.Entitlement)(nil)).
			Where("user_id = ? AND entitlement = ?", userID, "premium").
			Where("revoked_at IS NULL").
			Where("end_at IS NULL").
			Where("start_at <= ?", time.Now()).
			Exists(ctx)
		if exists {
			return 0, "", 0, time.Time{}, fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
		}
	}

	return price.Amount, price.Currency, tokenUnits, exp, nil
}

// Submit records a confirmed payment for the given price and user, and grants associated entitlements.
// This is a pragmatic implementation that skips on-chain signature verification in this codebase.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Submit(ctx context.Context, userID string, priceID uuid.UUID, signature string, userEmail *string) (*models.Payment, error) {
	var payment *models.Payment
	var notificationData *oneOffNotificationData
	var purchasedProductName string

	err := s.db.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Create transactional services
		txDB := db.NewWithTx(tx)
		priceService := NewPriceService(txDB)
		paymentService := NewPaymentService(txDB)
		productService := NewProductService(txDB)
		entitlementService := NewEntitlementService(txDB)

		// Get price information
		price, err := priceService.GetByID(ctx, priceID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPriceNotFound, err)
		}

		// Disallow one-off if a subscription entitlement is already active (indefinite)
		if userID != "" {
			exists, _ := tx.NewSelect().
				Model((*models.Entitlement)(nil)).
				Where("user_id = ? AND entitlement = ?", userID, "premium").
				Where("revoked_at IS NULL").
				Where("end_at IS NULL").
				Where("start_at <= ?", time.Now()).
				Exists(ctx)
			if exists {
				return fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
			}
		}

		// Create canonical payment record
		now := time.Now()
		payment = &models.Payment{
			ID:            uuid.New(),
			UserID:        userID,
			PriceID:       price.ID,
			Processor:     models.ProcessorSolana,
			TransactionID: signature,
			Amount:        price.Amount,
			Currency:      price.Currency,
			PurchasedAt:   now,
		}
		if err := paymentService.Create(ctx, payment); err != nil {
			return fmt.Errorf("failed to create payment: %w", err)
		}

		// Grant entitlements based on product specification
		if s.entitlementService != nil && s.productService != nil {
			product, err := productService.GetByID(ctx, price.ProductID)
			if err != nil {
				return fmt.Errorf("failed to get product: %w", err)
			}
			purchasedProductName = product.DisplayName

			// Build list of entitlement names from product spec
			entNames := make([]string, 0, 4)
			if len(product.EntitlementsSpec) > 0 {
				for name := range product.EntitlementsSpec {
					entNames = append(entNames, name)
				}
			} else {
				// Default to premium entitlement if no spec provided
				entNames = append(entNames, "premium")
			}

			// Grant entitlements for each entitlement name
			for _, entName := range entNames {
				// Get entitlement configuration from product spec
				spec, exists := product.EntitlementsSpec[entName]
				var days int
				if exists && spec != nil {
					days = *spec
				} else {
					// Default to 30 days for one-off payments if not specified
					days = 30
				}

				// Grant entitlement using AppendEntitlementDays to avoid overlap with existing windows
				_, err := entitlementService.AppendEntitlementDays(ctx, userID, entName, days, models.EntitlementSourceOneOff, &payment.ID)
				if err != nil {
					log.WithFields(log.Fields{
						"payment_id":  payment.ID,
						"user_id":     userID,
						"entitlement": entName,
						"days":        days,
						"error":       err.Error(),
					}).Error("Failed to grant entitlement for Solana payment")
					return fmt.Errorf("failed to grant entitlement %s: %w", entName, err)
				}

				log.WithFields(log.Fields{
					"payment_id":  payment.ID,
					"user_id":     userID,
					"entitlement": entName,
					"days":        days,
				}).Info("Successfully granted entitlement for Solana payment")
			}
		}

		// Mark any pending SolanaTransaction for this user and price as confirmed (best-effort)
		_, _ = tx.NewUpdate().
			TableExpr("solana_transactions").
			Set("status = ?", "confirmed").
			Set("signature = ?", signature).
			Where("user_id = ?", userID).
			Where("amount = ?", price.Amount).
			Exec(ctx)

		notificationData = &oneOffNotificationData{
			UserID:      userID,
			Amount:      price.Amount,
			Currency:    price.Currency,
			ProductName: purchasedProductName,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.enqueueOneOffNotification(ctx, notificationData, userEmail)

	return payment, nil
}

func (s *SolanaPaymentService) enqueueOneOffNotification(ctx context.Context, data *oneOffNotificationData, userEmail *string) {
	if data == nil || s.notificationSvc == nil {
		return
	}

	email := ""
	if userEmail != nil {
		email = *userEmail
	}
	if email == "" {
		log.WithContext(ctx).WithField("user_id", data.UserID).Warn("skipping one-off receipt notification - user email missing")
		return
	}

	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    data.UserID,
		EventType: models.NotificationOneOffPurchaseCompleted,
		Data: map[string]any{
			"amount":       data.Amount,
			"currency":     data.Currency,
			"product_name": data.ProductName,
			"user_email":   email,
		},
	}

	if err := s.notificationSvc.CreateAndDeliver(ctx, notification); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":    data.UserID,
			"event_type": models.NotificationOneOffPurchaseCompleted,
		}).Error("failed to create and deliver one-off purchase notification")
	}
}
