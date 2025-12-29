package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcutil/base58"
	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/solana-go"
	"github.com/doujins-org/solana-go/programs/system"
	"github.com/doujins-org/solana-go/programs/token"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	redis "github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

const (
	// Redis keys
	pendingSolanaPaymentsKey = "pending_solana_payments"
	solanaPayKeyPrefix       = "solana_pay:"

	// TTL for pending payments
	pendingPaymentTTL = 15 * time.Minute
)

// PendingSolanaPayment represents a pending Solana payment stored in Redis
type PendingSolanaPayment struct {
	UserID      string    `json:"user_id"`
	PriceID     string    `json:"price_id"`
	Amount      int64     `json:"amount"`   // cents (fiat equivalent)
	Currency    string    `json:"currency"` // e.g., "usd"
	Token       string    `json:"token"`    // e.g., "USDC"
	TokenMint   string    `json:"token_mint"`
	TokenAmount uint64    `json:"token_amount"` // token base units
	Recipient   string    `json:"recipient"`    // merchant wallet
	CreatedAt   time.Time `json:"created_at"`
}

// SolanaPayResult is returned when creating a new Solana Pay URL
type SolanaPayResult struct {
	URL         string
	Reference   string
	Amount      int64 // cents
	Currency    string
	TokenAmount string // formatted token amount (e.g., "9.99")
	Token       string
	ExpiresAt   time.Time
}

// SolanaPayService handles Solana Pay Transfer Request flow
type SolanaPayService struct {
	db              *db.DB
	redis           *redis.Client
	cfg             *config.Config
	Clock           clockwork.Clock
	priceService    *PriceService
	productService  *ProductService
	checkoutService *CheckoutService
	rpc             *SolanaRPCService
}

// NewSolanaPayService creates a new SolanaPayService
func NewSolanaPayService(
	db *db.DB,
	redis *redis.Client,
	cfg *config.Config,
	priceService *PriceService,
	productService *ProductService,
	checkoutService *CheckoutService,
) *SolanaPayService {
	var rpc *SolanaRPCService
	if cfg != nil && cfg.Solana != nil {
		rpc = NewSolanaRPCService(cfg.Solana.RPCEndpoint, cfg.Solana.Network)
	}
	return &SolanaPayService{
		db:              db,
		redis:           redis,
		cfg:             cfg,
		priceService:    priceService,
		productService:  productService,
		checkoutService: checkoutService,
		rpc:             rpc,
	}
}

func (s *SolanaPayService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now()
}

// SetCheckoutService sets the checkout service (used to break circular dependency during init)
func (s *SolanaPayService) SetCheckoutService(cs *CheckoutService) {
	s.checkoutService = cs
}

// GeneratePayment creates a new pending Solana payment and returns the Transfer Request URL.
// It first checks purchase eligibility to prevent duplicate purchases.
func (s *SolanaPayService) GeneratePayment(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol string) (*SolanaPayResult, error) {
	// Check purchase eligibility BEFORE generating the payment URL
	if s.checkoutService != nil {
		eligibility, err := s.checkoutService.CheckPurchaseEligibility(ctx, userID, priceID)
		if err != nil {
			return nil, fmt.Errorf("failed to check purchase eligibility: %w", err)
		}

		switch eligibility.Status {
		case EligibilityBlocked:
			return nil, fmt.Errorf("purchase blocked: %s", eligibility.Reason)
		case EligibilityUpgrade, EligibilityDowngrade:
			// Solana doesn't support subscription upgrades/downgrades
			return nil, fmt.Errorf("solana does not support subscription tier changes; please cancel existing subscription first")
		case EligibilityAllowed:
			// Continue with payment generation
		}
	}

	// Validate price
	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("price not found: %w", err)
	}
	if !price.IsActive {
		return nil, fmt.Errorf("price is not active")
	}

	// Validate product
	if s.productService != nil {
		product, err := s.productService.GetByID(ctx, price.ProductID)
		if err != nil {
			return nil, fmt.Errorf("product not found: %w", err)
		}
		if !product.IsActive {
			return nil, fmt.Errorf("product is not active")
		}
	}

	// Validate Solana config
	if s.cfg.Solana == nil {
		return nil, fmt.Errorf("solana not configured")
	}
	tokenCfg, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		return nil, fmt.Errorf("invalid or unsupported token: %s", tokenSymbol)
	}

	// Calculate token amount from fiat price
	tokenUnits, _, err := calculateTokenQuote(ctx, tokenCfg, price.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate token quote: %w", err)
	}

	// Generate reference (32 bytes, base58 encoded per Solana Pay spec)
	reference, err := generateReference()
	if err != nil {
		return nil, fmt.Errorf("failed to generate reference: %w", err)
	}

	// Get merchant recipient wallet
	recipient := s.cfg.Solana.RecipientWallet
	if recipient == "" {
		return nil, fmt.Errorf("merchant wallet not configured")
	}

	// Get token mint
	tokenMint := tokenCfg.Mint
	if s.cfg.Solana.Network == "mainnet" && tokenCfg.MainnetMint != "" {
		tokenMint = tokenCfg.MainnetMint
	}

	now := s.now()
	expiresAt := now.Add(pendingPaymentTTL)

	// Store pending payment in Redis
	pending := &PendingSolanaPayment{
		UserID:      userID,
		PriceID:     priceID.String(),
		Amount:      price.Amount,
		Currency:    price.Currency,
		Token:       tokenSymbol,
		TokenMint:   tokenMint,
		TokenAmount: tokenUnits,
		Recipient:   recipient,
		CreatedAt:   now,
	}

	if err := s.storePendingPayment(ctx, reference, pending); err != nil {
		return nil, fmt.Errorf("failed to store pending payment: %w", err)
	}

	// Build Solana Pay Transfer Request URL
	url := s.buildTransferRequestURL(recipient, tokenUnits, tokenMint, tokenSymbol, reference)

	return &SolanaPayResult{
		URL:         url,
		Reference:   reference,
		Amount:      price.Amount,
		Currency:    price.Currency,
		TokenAmount: formatTokenAmount(tokenUnits, tokenCfg.Decimals),
		Token:       tokenSymbol,
		ExpiresAt:   expiresAt,
	}, nil
}

// buildTransferRequestURL constructs the solana: URL per the Solana Pay spec
func (s *SolanaPayService) buildTransferRequestURL(recipient string, amount uint64, tokenMint, tokenSymbol, reference string) string {
	// Base URL: solana:<recipient>
	url := fmt.Sprintf("solana:%s", recipient)

	// Get token config for decimals
	tokenCfg, _ := s.cfg.Solana.SupportedTokens[tokenSymbol]

	// Format amount with proper decimals
	formattedAmount := formatTokenAmount(amount, tokenCfg.Decimals)

	// Add query params
	params := fmt.Sprintf("?amount=%s", formattedAmount)

	// Add spl-token param if not native SOL
	if tokenMint != "" && tokenSymbol != "SOL" {
		params += fmt.Sprintf("&spl-token=%s", tokenMint)
	}

	// Add reference for payment detection
	params += fmt.Sprintf("&reference=%s", reference)

	// Add label
	params += "&label=Doujins%20Purchase"

	return url + params
}

// storePendingPayment stores a pending payment in Redis
func (s *SolanaPayService) storePendingPayment(ctx context.Context, reference string, pending *PendingSolanaPayment) error {
	if s.redis == nil {
		return fmt.Errorf("redis not configured")
	}

	key := solanaPayKeyPrefix + reference
	data, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("failed to marshal pending payment: %w", err)
	}

	// Store the payment data with TTL
	if err := s.redis.Set(ctx, key, data, pendingPaymentTTL).Err(); err != nil {
		return fmt.Errorf("failed to store pending payment: %w", err)
	}

	// Add to the pending payments set
	if err := s.redis.SAdd(ctx, pendingSolanaPaymentsKey, reference).Err(); err != nil {
		// Try to cleanup the key we just set
		s.redis.Del(ctx, key)
		return fmt.Errorf("failed to add to pending set: %w", err)
	}

	log.WithFields(log.Fields{
		"reference": reference,
		"user_id":   pending.UserID,
		"amount":    pending.Amount,
		"token":     pending.Token,
	}).Info("Stored pending Solana payment")

	return nil
}

// GetPendingPayment retrieves a pending payment by reference
func (s *SolanaPayService) GetPendingPayment(ctx context.Context, reference string) (*PendingSolanaPayment, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("redis not configured")
	}

	key := solanaPayKeyPrefix + reference
	data, err := s.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Not found (expired)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get pending payment: %w", err)
	}

	var pending PendingSolanaPayment
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pending payment: %w", err)
	}

	return &pending, nil
}

// GetAllPendingReferences returns all pending payment references
func (s *SolanaPayService) GetAllPendingReferences(ctx context.Context) ([]string, error) {
	if s.redis == nil {
		return nil, nil
	}

	refs, err := s.redis.SMembers(ctx, pendingSolanaPaymentsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending references: %w", err)
	}

	return refs, nil
}

// RemovePendingPayment removes a pending payment from Redis
func (s *SolanaPayService) RemovePendingPayment(ctx context.Context, reference string) error {
	if s.redis == nil {
		return nil
	}

	key := solanaPayKeyPrefix + reference

	// Remove from set
	if err := s.redis.SRem(ctx, pendingSolanaPaymentsKey, reference).Err(); err != nil {
		log.WithError(err).WithField("reference", reference).Warn("Failed to remove from pending set")
	}

	// Delete the key
	if err := s.redis.Del(ctx, key).Err(); err != nil {
		log.WithError(err).WithField("reference", reference).Warn("Failed to delete pending payment key")
	}

	return nil
}

// GetPaymentStatus checks if a payment is pending, confirmed, or expired
func (s *SolanaPayService) GetPaymentStatus(ctx context.Context, reference string) (status string, payment *models.Payment, err error) {
	// First check Postgres for confirmed payment
	payment, err = s.getPaymentByReference(ctx, reference)
	if err == nil && payment != nil {
		return "confirmed", payment, nil
	}

	// Check Redis for pending payment
	pending, err := s.GetPendingPayment(ctx, reference)
	if err != nil {
		return "", nil, fmt.Errorf("failed to check pending payment: %w", err)
	}

	if pending == nil {
		return "expired", nil, nil
	}

	return "pending", nil, nil
}

// getPaymentByReference looks up a payment by its Solana reference
func (s *SolanaPayService) getPaymentByReference(ctx context.Context, reference string) (*models.Payment, error) {
	// We need to find a payment where the TransactionID or some metadata contains this reference
	// For now, we'll check SolanaTransaction table which has the reference
	var solTx models.SolanaTransaction
	err := s.db.GetDB().NewSelect().
		Model(&solTx).
		Where("processing_result->>'reference' = ?", reference).
		Limit(1).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	if solTx.PaymentID == nil {
		return nil, fmt.Errorf("solana transaction has no linked payment")
	}

	var payment models.Payment
	err = s.db.GetDB().NewSelect().
		Model(&payment).
		Where("id = ?", *solTx.PaymentID).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return &payment, nil
}

// generateReference generates a 32-byte random reference, base58 encoded
func generateReference() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base58.Encode(buf), nil
}

// formatTokenAmount formats a token amount with the appropriate decimal places
func formatTokenAmount(amount uint64, decimals int) string {
	if decimals <= 0 {
		return fmt.Sprintf("%d", amount)
	}
	// Convert to string with decimal point
	divisor := uint64(1)
	for i := 0; i < decimals; i++ {
		divisor *= 10
	}
	whole := amount / divisor
	frac := amount % divisor
	if frac == 0 {
		return fmt.Sprintf("%d", whole)
	}
	// Format fractional part with leading zeros
	fracStr := fmt.Sprintf("%0*d", decimals, frac)
	// Trim trailing zeros
	fracStr = strings.TrimRight(fracStr, "0")
	return fmt.Sprintf("%d.%s", whole, fracStr)
}

// GetIntentByID returns a Solana Pay intent by primary key.
func (s *SolanaPayService) GetIntentByID(ctx context.Context, id uuid.UUID) (*models.SolanaPayIntent, error) {
	intent := new(models.SolanaPayIntent)
	err := s.db.GetDB().NewSelect().
		Model(intent).
		Where("id = ?", id).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return intent, nil
}

func (s *SolanaPayService) tokenConfigForMint(mint string) (config.TokenConfig, bool) {
	if s.cfg == nil || s.cfg.Solana == nil {
		return config.TokenConfig{}, false
	}
	for _, cfg := range s.cfg.Solana.SupportedTokens {
		if !cfg.Enabled {
			continue
		}
		if strings.EqualFold(cfg.Mint, mint) || (cfg.MainnetMint != "" && strings.EqualFold(cfg.MainnetMint, mint)) {
			return cfg, true
		}
	}
	return config.TokenConfig{}, false
}

// BuildTransactionRequest builds an unsigned Solana Pay transaction for a given intent and user account.
func (s *SolanaPayService) BuildTransactionRequest(ctx context.Context, intentID uuid.UUID, accountBase58 string) (string, *models.SolanaPayIntent, error) {
	if s.rpc == nil {
		return "", nil, fmt.Errorf("solana rpc not configured")
	}
	intent, err := s.GetIntentByID(ctx, intentID)
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	if intent.ExpiresAt != nil && intent.ExpiresAt.Before(now) {
		return "", nil, fmt.Errorf("intent expired")
	}
	if intent.Amount <= 0 {
		return "", nil, fmt.Errorf("invalid amount")
	}

	account, err := solana.PublicKeyFromBase58(strings.TrimSpace(accountBase58))
	if err != nil {
		return "", nil, fmt.Errorf("invalid account: %w", err)
	}
	recipient, err := solana.PublicKeyFromBase58(strings.TrimSpace(intent.Recipient))
	if err != nil {
		return "", nil, fmt.Errorf("invalid recipient: %w", err)
	}
	ref, err := solana.PublicKeyFromBase58(strings.TrimSpace(intent.Reference))
	if err != nil {
		return "", nil, fmt.Errorf("invalid reference: %w", err)
	}

	blockhash, err := s.rpc.GetLatestBlockhash(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get blockhash: %w", err)
	}

	var instructions []solana.Instruction
	amount := uint64(intent.Amount)

	if intent.TokenMint != nil && strings.TrimSpace(*intent.TokenMint) != "" {
		mintStr := strings.TrimSpace(*intent.TokenMint)
		mint, err := solana.PublicKeyFromBase58(mintStr)
		if err != nil {
			return "", nil, fmt.Errorf("invalid token mint: %w", err)
		}
		if _, ok := s.tokenConfigForMint(mintStr); !ok {
			return "", nil, fmt.Errorf("unsupported token mint")
		}

		fromATA, _, err := solana.FindAssociatedTokenAddress(account, mint)
		if err != nil {
			return "", nil, fmt.Errorf("failed to find source token account: %w", err)
		}
		toATA, _, err := solana.FindAssociatedTokenAddress(recipient, mint)
		if err != nil {
			return "", nil, fmt.Errorf("failed to find destination token account: %w", err)
		}

		ix := token.NewTransferInstruction(
			amount,
			fromATA,
			toATA,
			account,
			[]solana.PublicKey{},
		).Build()
		accounts := ix.Accounts()
		accounts = append(accounts, &solana.AccountMeta{PublicKey: ref, IsSigner: false, IsWritable: false})
		data, err := ix.Data()
		if err != nil {
			return "", nil, fmt.Errorf("failed to build token transfer data: %w", err)
		}
		ixWithRef := solana.NewInstruction(ix.ProgramID(), accounts, data)
		instructions = append(instructions, ixWithRef)
	} else {
		ix := system.NewTransferInstruction(
			amount,
			account,
			recipient,
		).Build()
		accounts := ix.Accounts()
		accounts = append(accounts, &solana.AccountMeta{PublicKey: ref, IsSigner: false, IsWritable: false})
		data, err := ix.Data()
		if err != nil {
			return "", nil, fmt.Errorf("failed to build sol transfer data: %w", err)
		}
		ixWithRef := solana.NewInstruction(ix.ProgramID(), accounts, data)
		instructions = append(instructions, ixWithRef)
	}

	tx, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(account),
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	// Unsigned; wallet signs and broadcasts.
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	return base64.StdEncoding.EncodeToString(txBytes), intent, nil
}
