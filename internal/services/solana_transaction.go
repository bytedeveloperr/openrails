package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/solana-go"
	"github.com/doujins-org/solana-go/programs/system"
	"github.com/doujins-org/solana-go/programs/token"
	"github.com/doujins-org/solana-go/rpc"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// SolanaTransactionService builds real Solana transactions for payments
type SolanaTransactionService struct {
	db           *db.DB
	rpc          *SolanaRPCService
	cfg          *config.Config
	priceService *PriceService
	paymentSvc   *PaymentService
}

// NewSolanaTransactionService creates a new transaction service
func NewSolanaTransactionService(db *db.DB, rpc *SolanaRPCService, cfg *config.Config, price *PriceService, payment *PaymentService) *SolanaTransactionService {
	return &SolanaTransactionService{
		db:           db,
		rpc:          rpc,
		cfg:          cfg,
		priceService: price,
		paymentSvc:   payment,
	}
}

// TransactionRequest represents a payment transaction request
type TransactionRequest struct {
	PriceID    uuid.UUID
	TokenMint  solana.PublicKey
	FromWallet solana.PublicKey
	ToWallet   solana.PublicKey
	Amount     uint64 // Amount in token's smallest unit (lamports for SOL, smallest unit for SPL tokens)
}

// TransactionResponse contains the built transaction and metadata
type TransactionResponse struct {
	Transaction       *solana.Transaction
	TransactionBase64 string
	PendingID         uuid.UUID
	Amount            float64
	TokenAmount       uint64
	TokenSymbol       string
	ExpiresAt         time.Time
	Instructions      string
}

// BuildPaymentTransaction creates a real Solana transaction for payment
func (s *SolanaTransactionService) BuildPaymentTransaction(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, userWallet string) (*TransactionResponse, error) {
	// Get price information
	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	// Validate Solana configuration
	if s.cfg.Solana == nil {
		return nil, fmt.Errorf("solana configuration not found")
	}

	// Get token configuration
	tokenCfg, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		return nil, fmt.Errorf("token %s not supported", tokenSymbol)
	}

	// Parse wallet addresses
	fromWallet, err := solana.PublicKeyFromBase58(userWallet)
	if err != nil {
		return nil, fmt.Errorf("invalid user wallet address: %w", err)
	}

	merchantWallet := s.cfg.Solana.RecipientWallet
	if merchantWallet == "" {
		merchantWallet = s.cfg.Solana.DestinationWallet
	}
	if merchantWallet == "" {
		return nil, fmt.Errorf("merchant wallet not configured")
	}

	toWallet, err := solana.PublicKeyFromBase58(merchantWallet)
	if err != nil {
		return nil, fmt.Errorf("invalid merchant wallet address: %w", err)
	}

	// Calculate token amount in smallest units
	tokenAmount := uint64(math.Round(price.Amount * math.Pow10(tokenCfg.Decimals)))

	// Get latest blockhash
	blockhash, err := s.rpc.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get blockhash: %w", err)
	}

	var transaction *solana.Transaction
	var instructions []solana.Instruction

	if tokenSymbol == "SOL" {
		// Native SOL transfer
		instruction := system.NewTransferInstruction(
			tokenAmount,
			fromWallet,
			toWallet,
		).Build()
		instructions = append(instructions, instruction)
	} else {
		// SPL Token transfer
		var tokenMint solana.PublicKey
		tokenMint, err = solana.PublicKeyFromBase58(tokenCfg.Mint)
		if err != nil {
			return nil, fmt.Errorf("invalid token mint address: %w", err)
		}

		// Find associated token accounts
		var fromTokenAccount solana.PublicKey
		fromTokenAccount, _, err = solana.FindAssociatedTokenAddress(fromWallet, tokenMint)
		if err != nil {
			return nil, fmt.Errorf("failed to find from token account: %w", err)
		}

		var toTokenAccount solana.PublicKey
		toTokenAccount, _, err = solana.FindAssociatedTokenAddress(toWallet, tokenMint)
		if err != nil {
			return nil, fmt.Errorf("failed to find to token account: %w", err)
		}

		// Create transfer instruction
		instruction := token.NewTransferInstruction(
			tokenAmount,
			fromTokenAccount,
			toTokenAccount,
			fromWallet,
			[]solana.PublicKey{},
		).Build()
		instructions = append(instructions, instruction)
	}

	// Build transaction
	transaction, err = solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(fromWallet),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Serialize transaction for frontend
	txBytes, err := transaction.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	// Create pending transaction record
	expiresAt := time.Now().Add(10 * time.Minute)
	stx := &models.SolanaTransaction{
		ID:          uuid.New(),
		Status:      "pending",
		Amount:      price.Amount,
		Token:       tokenSymbol,
		TokenMint:   tokenCfg.Mint,
		FromAddress: userWallet,
		ToAddress:   merchantWallet,
		ExpiresAt:   &expiresAt,
	}
	if userID != "" {
		stx.UserID = &userID
	}

	if _, err := s.db.GetDB().NewInsert().Model(stx).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create pending transaction: %w", err)
	}

	log.WithFields(log.Fields{
		"pending_id":   stx.ID,
		"user_id":      userID,
		"price_id":     priceID,
		"token":        tokenSymbol,
		"amount":       price.Amount,
		"token_amount": tokenAmount,
		"from_wallet":  userWallet,
		"to_wallet":    merchantWallet,
	}).Info("Built Solana payment transaction")

	return &TransactionResponse{
		Transaction:       transaction,
		TransactionBase64: base64.StdEncoding.EncodeToString(txBytes),
		PendingID:         stx.ID,
		Amount:            price.Amount,
		TokenAmount:       tokenAmount,
		TokenSymbol:       tokenSymbol,
		ExpiresAt:         expiresAt,
		Instructions:      fmt.Sprintf("Sign this transaction to pay %.2f %s using %s", price.Amount, price.Currency, tokenSymbol),
	}, nil
}

// SimulateTransaction simulates the transaction to check if it would succeed
func (s *SolanaTransactionService) SimulateTransaction(ctx context.Context, tx *solana.Transaction) error {
	resp, err := s.rpc.SimulateTransaction(ctx, tx)
	if err != nil {
		return fmt.Errorf("transaction simulation failed: %w", err)
	}

	if resp.Value.Err != nil {
		return fmt.Errorf("transaction would fail: %v", resp.Value.Err)
	}

	log.WithFields(log.Fields{
		"units_consumed": resp.Value.UnitsConsumed,
		"logs":           resp.Value.Logs,
	}).Info("Transaction simulation successful")

	return nil
}

// VerifyTransactionSignature verifies a signed transaction on-chain
func (s *SolanaTransactionService) VerifyTransactionSignature(ctx context.Context, signature string) (*rpc.GetTransactionResult, error) {
	sig, err := solana.SignatureFromBase58(signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature format: %w", err)
	}

	// Wait for confirmation
	if err = s.rpc.ConfirmTransaction(ctx, sig, rpc.CommitmentConfirmed); err != nil {
		return nil, fmt.Errorf("transaction confirmation failed: %w", err)
	}

	// Get transaction details
	txResult, err := s.rpc.GetTransaction(ctx, sig)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	if txResult.Meta.Err != nil {
		return nil, fmt.Errorf("transaction failed on-chain: %v", txResult.Meta.Err)
	}

	log.WithFields(log.Fields{
		"signature": signature,
		"slot":      txResult.Slot,
		"fee":       txResult.Meta.Fee,
	}).Info("Transaction verified on-chain")

	return txResult, nil
}
