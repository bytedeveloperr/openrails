package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	solanaintegration "github.com/doujins-org/doujins-billing/internal/integrations/solana"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
)

// SolanaTransactionService builds real Solana transactions for payments.
type SolanaTransactionService struct {
	db           *db.DB
	rpc          *solanaintegration.RPCClient
	cfg          *config.Config
	priceService *PriceService
	paymentSvc   *PaymentService
	Clock        clockwork.Clock
}

// now returns the current time from the service's clock, or time.Now() if no clock is set.
func (s *SolanaTransactionService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now()
}

// NewSolanaTransactionService creates a new transaction service.
func NewSolanaTransactionService(db *db.DB, rpc *solanaintegration.RPCClient, cfg *config.Config, price *PriceService, payment *PaymentService) *SolanaTransactionService {
	return &SolanaTransactionService{
		db:           db,
		rpc:          rpc,
		cfg:          cfg,
		priceService: price,
		paymentSvc:   payment,
	}
}

// TransactionResponse contains the built transaction and metadata.
type TransactionResponse struct {
	TransactionBase64 string
	Amount            int64 // Amount in cents (smallest currency unit)
	TokenAmount       uint64
	TokenSymbol       string
	ExpiresAt         time.Time
	Instructions      string
}

// BuildPaymentTransaction creates a Solana transaction for payment.
func (s *SolanaTransactionService) BuildPaymentTransaction(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, userWallet string, reference *string) (*TransactionResponse, error) {
	if s.rpc == nil {
		return nil, fmt.Errorf("solana rpc client unavailable")
	}

	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	if s.cfg.Solana == nil {
		return nil, fmt.Errorf("solana configuration not found")
	}

	tokenCfg, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		return nil, fmt.Errorf("token %s not supported", tokenSymbol)
	}

	merchantWallet := s.cfg.Solana.RecipientWallet
	if merchantWallet == "" {
		return nil, fmt.Errorf("merchant wallet not configured")
	}

	tokenAmount, tokenAmountDecimal, err := calculateTokenQuote(ctx, tokenCfg, price.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate token amount: %w", err)
	}
	if tokenAmount == 0 {
		return nil, fmt.Errorf("calculated token amount is zero")
	}

	referenceStr := ""
	if reference != nil {
		referenceStr = strings.TrimSpace(*reference)
	}

	txResp, err := s.rpc.BuildTransferTransaction(ctx, solanaintegration.TransferRequest{
		FromWallet:  userWallet,
		ToWallet:    merchantWallet,
		TokenSymbol: tokenSymbol,
		TokenMint:   tokenCfg.Mint,
		Amount:      tokenAmount,
		Reference:   referenceStr,
	})
	if err != nil {
		return nil, err
	}

	expiresAt := s.now().Add(10 * time.Minute)

	log.WithFields(log.Fields{
		"user_id":      userID,
		"price_id":     priceID,
		"token":        tokenSymbol,
		"amount_cents": price.Amount,
		"token_amount": tokenAmountDecimal,
		"from_wallet":  userWallet,
		"to_wallet":    merchantWallet,
	}).Info("Built Solana payment transaction")

	return &TransactionResponse{
		TransactionBase64: txResp.TransactionBase64,
		Amount:            price.Amount,
		TokenAmount:       tokenAmount,
		TokenSymbol:       tokenSymbol,
		ExpiresAt:         expiresAt,
		Instructions:      fmt.Sprintf("Sign this transaction to pay %.2f %s using %s", float64(price.Amount)/100.0, price.Currency, tokenSymbol),
	}, nil
}

// VerifyTransactionWithContent verifies a transaction against expected recipient, payer, and optional reference.
func (s *SolanaTransactionService) VerifyTransactionWithContent(ctx context.Context, signature string, expectedAmount uint64, expectedRecipient string, expectedTokenMint string, expectedPayer string, expectedReference *string) error {
	if s.rpc == nil {
		return fmt.Errorf("solana rpc client unavailable")
	}

	reference := ""
	if expectedReference != nil {
		reference = strings.TrimSpace(*expectedReference)
	}

	return s.rpc.VerifyTransfer(ctx, solanaintegration.VerifyTransferRequest{
		Signature:         strings.TrimSpace(signature),
		ExpectedAmount:    expectedAmount,
		ExpectedRecipient: strings.TrimSpace(expectedRecipient),
		ExpectedTokenMint: strings.TrimSpace(expectedTokenMint),
		ExpectedPayer:     strings.TrimSpace(expectedPayer),
		ExpectedReference: reference,
	})
}
