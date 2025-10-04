package services

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
)

const (
	FlowTypeDirect    = "direct"
	FlowTypeSolanaPay = "solanapay"

	IntentStatusPending    = "pending"
	IntentStatusProcessing = "processing"
	IntentStatusConfirmed  = "confirmed"
	IntentStatusFailed     = "failed"
	IntentStatusExpired    = "expired"
)

var (
	ErrIntentNotFound     = errors.New("solana payment intent not found")
	ErrIntentInvalidState = errors.New("solana payment intent has invalid state for this operation")
)

// SolanaPaymentIntentService manages persisted payment intents shared by direct and Solana Pay flows.
type SolanaPaymentIntentService struct {
	db           *db.DB
	cfg          *config.Config
	priceService *PriceService
}

func NewSolanaPaymentIntentService(db *db.DB, cfg *config.Config, priceService *PriceService) *SolanaPaymentIntentService {
	return &SolanaPaymentIntentService{db: db, cfg: cfg, priceService: priceService}
}

// CreateDirectIntent creates an intent for a direct wallet transaction using a verified payer wallet.
func (s *SolanaPaymentIntentService) CreateDirectIntent(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, payerWallet string) (*models.SolanaPaymentIntent, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if s.cfg.Solana == nil {
		return nil, fmt.Errorf("solana configuration not available")
	}

	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load price: %w", err)
	}

	tokenCfg, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		return nil, fmt.Errorf("token %s not supported", tokenSymbol)
	}

	merchantWallet := s.cfg.Solana.RecipientWallet
	if merchantWallet == "" {
		return nil, fmt.Errorf("solana recipient wallet not configured")
	}

	tokenAmountUnits, tokenAmountDecimal, err := calculateTokenQuote(ctx, tokenCfg, price.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate token amount: %w", err)
	}

	expiresAt := time.Now().Add(10 * time.Minute)
	intent := &models.SolanaPaymentIntent{
		ID:                     uuid.New(),
		UserID:                 userID,
		PriceID:                price.ID,
		FlowType:               FlowTypeDirect,
		Token:                  tokenSymbol,
		TokenMint:              tokenCfg.Mint,
		Amount:                 tokenAmountDecimal,
		Currency:               tokenSymbol,
		ExpectedAmountLamports: tokenAmountUnits,
		RecipientWallet:        merchantWallet,
		Status:                 IntentStatusPending,
		ExpiresAt:              &expiresAt,
	}

	if payerWallet != "" {
		intent.PayerWallet = &payerWallet
	}

	if err := s.insertIntent(ctx, intent); err != nil {
		return nil, err
	}

	return intent, nil
}

// CreateSolanaPayIntent creates an intent for a Solana Pay transaction and returns memo/reference metadata.
func (s *SolanaPaymentIntentService) CreateSolanaPayIntent(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol string) (*models.SolanaPaymentIntent, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if s.cfg.Solana == nil {
		return nil, fmt.Errorf("solana configuration not available")
	}

	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load price: %w", err)
	}

	tokenCfg, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		return nil, fmt.Errorf("token %s not supported", tokenSymbol)
	}

	merchantWallet := s.cfg.Solana.RecipientWallet

	if merchantWallet == "" {
		return nil, fmt.Errorf("solana recipient wallet not configured")
	}

	tokenAmountUnits, tokenAmountDecimal, err := calculateTokenQuote(ctx, tokenCfg, price.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate token amount: %w", err)
	}

	reference, err := generateReference()
	if err != nil {
		return nil, fmt.Errorf("failed to generate reference: %w", err)
	}

	memo := fmt.Sprintf("purchase:%s:%s", userID, price.ID.String())
	expiresAt := time.Now().Add(15 * time.Minute)

	intent := &models.SolanaPaymentIntent{
		ID:                     uuid.New(),
		UserID:                 userID,
		PriceID:                price.ID,
		FlowType:               FlowTypeSolanaPay,
		Token:                  tokenSymbol,
		TokenMint:              tokenCfg.Mint,
		Amount:                 tokenAmountDecimal,
		Currency:               tokenSymbol,
		ExpectedAmountLamports: tokenAmountUnits,
		RecipientWallet:        merchantWallet,
		Reference:              &reference,
		Memo:                   &memo,
		Status:                 IntentStatusPending,
		ExpiresAt:              &expiresAt,
	}

	if err := s.insertIntent(ctx, intent); err != nil {
		return nil, err
	}

	return intent, nil
}

// GetByID fetches an intent by ID.
func (s *SolanaPaymentIntentService) GetByID(ctx context.Context, intentID uuid.UUID) (*models.SolanaPaymentIntent, error) {
	var intent models.SolanaPaymentIntent
	err := s.db.GetDB().NewSelect().Model(&intent).Where("id = ?", intentID).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIntentNotFound, err)
	}
	return &intent, nil
}

// GetByReference fetches an intent by reference value.
func (s *SolanaPaymentIntentService) GetByReference(ctx context.Context, reference string) (*models.SolanaPaymentIntent, error) {
	var intent models.SolanaPaymentIntent
	err := s.db.GetDB().NewSelect().Model(&intent).Where("reference = ?", reference).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIntentNotFound, err)
	}
	return &intent, nil
}

// MarkProcessing marks an intent as processing to avoid races.
func (s *SolanaPaymentIntentService) MarkProcessing(ctx context.Context, intentID uuid.UUID) error {
	res, err := s.db.GetDB().NewUpdate().Model((*models.SolanaPaymentIntent)(nil)).
		Set("status = ?", IntentStatusProcessing).
		Set("updated_at = ?", time.Now()).
		Where("id = ? AND status = ?", intentID, IntentStatusPending).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark intent processing: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrIntentInvalidState
	}
	return nil
}

// MarkConfirmed marks an intent as confirmed and stores signature metadata.
func (s *SolanaPaymentIntentService) MarkConfirmed(ctx context.Context, intentID uuid.UUID, signature string) error {
	now := time.Now()
	statuses := []string{IntentStatusPending, IntentStatusProcessing}
	res, err := s.db.GetDB().NewUpdate().Model((*models.SolanaPaymentIntent)(nil)).
		Set("status = ?", IntentStatusConfirmed).
		Set("transaction_signature = ?", signature).
		Set("signature = ?", signature).
		Set("confirmed_at = ?", &now).
		Set("updated_at = ?", now).
		Where("id = ? AND status IN (?)", intentID, bun.In(statuses)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to confirm intent: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrIntentInvalidState
	}
	return nil
}

// MarkFailed records an error for the intent.
func (s *SolanaPaymentIntentService) MarkFailed(ctx context.Context, intentID uuid.UUID, message string) error {
	res, err := s.db.GetDB().NewUpdate().Model((*models.SolanaPaymentIntent)(nil)).
		Set("status = ?", IntentStatusFailed).
		Set("error_message = ?", message).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", intentID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark intent failed: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrIntentInvalidState
	}
	return nil
}

func (s *SolanaPaymentIntentService) insertIntent(ctx context.Context, intent *models.SolanaPaymentIntent) error {
	intent.CreatedAt = time.Now()
	intent.UpdatedAt = intent.CreatedAt
	_, err := s.db.GetDB().NewInsert().Model(intent).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create solana payment intent: %w", err)
	}
	log.WithFields(log.Fields{
		"intent_id": intent.ID,
		"flow":      intent.FlowType,
		"user_id":   intent.UserID,
		"price_id":  intent.PriceID,
	}).Info("Created Solana payment intent")
	return nil
}

func generateReference() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base58.Encode(buf), nil
}
