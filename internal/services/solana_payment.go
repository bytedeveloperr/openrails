package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	log "github.com/sirupsen/logrus"
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
}

func NewSolanaPaymentService(db *db.DB, cfg *config.Config, price *PriceService, payment *PaymentService, product *ProductService, entitlement *EntitlementService) *SolanaPaymentService {
	return &SolanaPaymentService{
		db:                 db,
		cfg:                cfg,
		priceService:       price,
		paymentSvc:         payment,
		productService:     product,
		entitlementService: entitlement,
	}
}

// Generate creates a pending SolanaTransaction record and returns UI hints for client-side payment.
// This does not (yet) build a binary transaction; instead it returns token amount calculations
// and creates server-side pending state for follow-up confirmation.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Generate(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, userWallet string) (amount float64, currency string, tokenAmount uint64, expiresAt time.Time, pendingID uuid.UUID, err error) {
	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return 0, "", 0, time.Time{}, uuid.Nil, fmt.Errorf("%w: %v", ErrPriceNotFound, err)
	}
	if s.cfg.Solana == nil {
		return 0, "", 0, time.Time{}, uuid.Nil, fmt.Errorf("solana not configured")
	}
	tok, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tok.Enabled {
		return 0, "", 0, time.Time{}, uuid.Nil, ErrInvalidToken
	}

	pow := math.Pow10(tok.Decimals)
	tokenAmt := uint64(math.Round(price.Amount * pow))
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
			return 0, "", 0, time.Time{}, uuid.Nil, fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
		}
	}

	// Create pending transaction record for traceability
	stx := &models.SolanaTransaction{
		ID:          uuid.New(),
		Status:      "pending",
		Amount:      price.Amount,
		Token:       tok.Symbol,
		TokenMint:   tok.Mint,
		FromAddress: userWallet,
		ToAddress:   firstNonEmpty(s.cfg.Solana.RecipientWallet, s.cfg.Solana.DestinationWallet),
		ExpiresAt:   &exp,
	}
	if userID != "" {
		stx.UserID = &userID
	}
	if _, err := s.db.GetDB().NewInsert().Model(stx).Exec(ctx); err != nil {
		return 0, "", 0, time.Time{}, uuid.Nil, fmt.Errorf("failed to create pending solana transaction: %w", err)
	}

	return price.Amount, price.Currency, tokenAmt, exp, stx.ID, nil
}

// Submit records a confirmed payment for the given price and user, and grants associated entitlements.
// This is a pragmatic implementation that skips on-chain signature verification in this codebase.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Submit(ctx context.Context, userID string, priceID uuid.UUID, signature string) (*models.Payment, error) {
	var payment *models.Payment

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

		return nil
	})

	if err != nil {
		return nil, err
	}

	return payment, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
