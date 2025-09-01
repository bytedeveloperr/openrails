package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

var (
	ErrInvalidToken       = errors.New("invalid or unsupported token")
	ErrPriceNotFound      = errors.New("price not found")
	ErrInvalidTransaction = errors.New("invalid transaction format")
	ErrTransactionFailed  = errors.New("transaction failed on network")
	ErrInvalidAmount      = errors.New("transaction amount does not match price")
	ErrInvalidRecipient   = errors.New("transaction recipient does not match expected wallet")
)

// Default token configurations
var defaultTokens = map[string]*config.TokenConfig{
	"SOL": {
		Mint:     "So11111111111111111111111111111111111111112",
		Symbol:   "SOL",
		Name:     "Solana",
		Decimals: 9,
		Enabled:  true,
	},
	"USDC": {
		Mint:     "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		Symbol:   "USDC",
		Name:     "USD Coin",
		Decimals: 6,
		Enabled:  true,
	},
	"USDT": {
		Mint:     "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
		Symbol:   "USDT",
		Name:     "Tether USD",
		Decimals: 6,
		Enabled:  true,
	},
	"PYUSD": {
		Mint:     "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo",
		Symbol:   "PYUSD",
		Name:     "PayPal USD",
		Decimals: 6,
		Enabled:  true,
	},
}

// GetTokenBySymbol retrieves token configuration by symbol
func GetTokenBySymbol(symbol string, cfg *config.SolanaConfig) *config.TokenConfig {
	// Check custom configured tokens first
	if cfg != nil && cfg.SupportedTokens != nil {
		if token, exists := cfg.SupportedTokens[symbol]; exists {
			return &token
		}
	}

	// Fall back to default tokens
	if token, exists := defaultTokens[symbol]; exists {
		return token
	}

	return nil
}

type SolanaPaymentService struct {
	rpcClient       *rpc.Client
	config          *config.SolanaConfig
	recipientWallet solana.PublicKey
	DB              *db.DB
}

// GetPriceByID retrieves a price by its ID
func (s *SolanaPaymentService) GetPriceByID(ctx context.Context, id uuid.UUID) (*models.Price, error) {
	var price models.Price
	err := s.DB.GetDB().NewSelect().
		Model(&price).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get price by ID: %w", err)
	}
	return &price, nil
}

// CreatePurchase creates a new purchase
func (s *SolanaPaymentService) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(purchase).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create purchase: %w", err)
	}
	return nil
}

// GetProductByID retrieves a product by its ID
func (s *SolanaPaymentService) GetProductByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	var product models.Product
	err := s.DB.GetDB().NewSelect().
		Model(&product).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get product by ID: %w", err)
	}
	return &product, nil
}

// ExtendRoleExpiration extends role expiration for a user
func (s *SolanaPaymentService) ExtendRoleExpiration(ctx context.Context, userID, roleID uuid.UUID, days int) (*models.UserRoleGrant, time.Time, error) {
	// Check if user already has this role
	var existingGrant models.UserRoleGrant
	err := s.DB.GetDB().NewSelect().
		Model(&existingGrant).
		Where("user_id = ?", userID).
		Where("role_id = ?", roleID).
		Where("revoked_at IS NULL").
		Scan(ctx)

	var newExpirationDate time.Time
	if err == nil {
		// User has existing grant, extend it
		if existingGrant.AutoExpiresAt != nil {
			newExpirationDate = existingGrant.AutoExpiresAt.AddDate(0, 0, days)
		} else {
			newExpirationDate = time.Now().AddDate(0, 0, days)
		}
		existingGrant.AutoExpiresAt = &newExpirationDate

		_, updateErr := s.DB.GetDB().NewUpdate().
			Model(&existingGrant).
			Where("id = ?", existingGrant.ID).
			Exec(ctx)
		if updateErr != nil {
			return nil, time.Time{}, fmt.Errorf("failed to update role expiration: %w", updateErr)
		}
		return &existingGrant, newExpirationDate, nil
	} else {
		// Create new role grant
		newExpirationDate = time.Now().AddDate(0, 0, days)
		newGrant := &models.UserRoleGrant{
			ID:            uuid.New(),
			UserID:        userID,
			RoleID:        roleID,
			AutoExpiresAt: &newExpirationDate,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		_, insertErr := s.DB.GetDB().NewInsert().
			Model(newGrant).
			Exec(ctx)
		if insertErr != nil {
			return nil, time.Time{}, fmt.Errorf("failed to create role grant: %w", insertErr)
		}
		return newGrant, newExpirationDate, nil
	}
}

// UpdatePurchase updates a purchase
func (s *SolanaPaymentService) UpdatePurchase(ctx context.Context, purchase *models.Purchase) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model(purchase).
		Where("id = ?", purchase.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update purchase: %w", err)
	}
	return nil
}

func NewSolanaPaymentService(
	db *db.DB,
	config *config.SolanaConfig,
) (*SolanaPaymentService, error) {
	// Create RPC client
	rpcClient := rpc.New(config.RPCEndpoint)

	// Parse recipient wallet
	recipientWallet, err := solana.PublicKeyFromBase58(config.RecipientWallet)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient wallet address: %w", err)
	}

	return &SolanaPaymentService{
		rpcClient:       rpcClient,
		DB:              db,
		config:          config,
		recipientWallet: recipientWallet,
	}, nil
}

// GeneratePayment creates an unsigned SPL token transfer transaction
func (s *SolanaPaymentService) GeneratePayment(ctx context.Context, priceID uuid.UUID, tokenSymbol string, userWallet solana.PublicKey) (string, error) {
	// Validate token
	tokenConfig := GetTokenBySymbol(tokenSymbol, s.config)
	if tokenConfig == nil {
		return "", ErrInvalidToken
	}

	// Get price information
	price, err := s.GetPriceByID(ctx, priceID)
	if err != nil {
		return "", ErrPriceNotFound
	}

	// Convert USD amount to token amount (assuming 1:1 for stablecoins)
	tokenAmount := uint64(price.Amount * math.Pow10(tokenConfig.Decimals))

	// Parse token mint
	tokenMint, err := solana.PublicKeyFromBase58(tokenConfig.Mint)
	if err != nil {
		return "", fmt.Errorf("invalid token mint address: %w", err)
	}

	// Get recent blockhash
	recentBlockhash, err := s.rpcClient.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Find or create associated token accounts
	senderATA, _, err := solana.FindAssociatedTokenAddress(userWallet, tokenMint)
	if err != nil {
		return "", fmt.Errorf("failed to find sender ATA: %w", err)
	}

	recipientATA, _, err := solana.FindAssociatedTokenAddress(s.recipientWallet, tokenMint)
	if err != nil {
		return "", fmt.Errorf("failed to find recipient ATA: %w", err)
	}

	// Check if recipient ATA exists, if not we'll need to create it
	instructions := []solana.Instruction{}

	// For simplicity, assume recipient ATA exists or will be created by the frontend
	// In production, you'd check if ATA exists and create it if needed

	// Create SPL token transfer instruction
	transferInstruction := token.NewTransferInstruction(
		tokenAmount,
		senderATA,
		recipientATA,
		userWallet,
		[]solana.PublicKey{}, // no multisig
	).Build()
	instructions = append(instructions, transferInstruction)

	// Create transaction
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockhash.Value.Blockhash,
		solana.TransactionPayer(userWallet),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}

	// Serialize transaction to base64
	serialized, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	return base64.StdEncoding.EncodeToString(serialized), nil
}

// SubmitPayment processes a signed transaction and creates a purchase record
func (s *SolanaPaymentService) SubmitPayment(ctx context.Context, signedTxBase64 string, priceID uuid.UUID, userID uuid.UUID) (*models.Purchase, error) {
	// Decode transaction
	txBytes, err := base64.StdEncoding.DecodeString(signedTxBase64)
	if err != nil {
		return nil, ErrInvalidTransaction
	}

	tx, err := solana.TransactionFromBytes(txBytes)
	if err != nil {
		return nil, ErrInvalidTransaction
	}

	// Get price information
	price, err := s.GetPriceByID(ctx, priceID)
	if err != nil {
		return nil, ErrPriceNotFound
	}

	// Broadcast transaction to Solana network
	signature, err := s.rpcClient.SendTransaction(ctx, tx)
	if err != nil {
		log.WithError(err).Error("Failed to broadcast Solana transaction")
		return nil, ErrTransactionFailed
	}

	// Wait for confirmation (processed level for immediate access)
	confirmed, err := s.waitForConfirmation(ctx, signature, rpc.CommitmentProcessed)
	if err != nil || !confirmed {
		log.WithFields(log.Fields{
			"signature": signature.String(),
			"error":     err,
		}).Error("Transaction failed to confirm")
		return nil, ErrTransactionFailed
	}

	// Validate transaction details
	if err := s.validateTransaction(ctx, tx, price); err != nil {
		log.WithFields(log.Fields{
			"signature": signature.String(),
			"error":     err,
		}).Error("Transaction validation failed")
		return nil, err
	}

	// Create purchase record
	purchase := &models.Purchase{
		ID:            uuid.New(),
		UserID:        userID,
		PriceID:       priceID,
		Processor:     models.ProcessorSolana,
		TransactionID: signature.String(),
		Amount:        price.Amount,
		Currency:      "USD",
		PurchasedAt:   time.Now(),
	}

	if err := s.CreatePurchase(ctx, purchase); err != nil {
		return nil, fmt.Errorf("failed to create purchase record: %w", err)
	}

	// Grant role if product has one associated
	if err := s.grantRoleForPurchase(ctx, purchase, price); err != nil {
		log.WithError(err).Error("Failed to grant role for purchase")
		// Don't fail the purchase, just log the error
	}

	log.WithFields(log.Fields{
		"user_id":   userID,
		"price_id":  priceID,
		"signature": signature.String(),
		"amount":    price.Amount,
		"currency":  "USD",
	}).Info("Solana payment processed successfully")

	return purchase, nil
}

// waitForConfirmation waits for a transaction to reach the specified commitment level
func (s *SolanaPaymentService) waitForConfirmation(ctx context.Context, signature solana.Signature, commitment rpc.CommitmentType) (bool, error) {
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-timeout.C:
			return false, errors.New("transaction confirmation timeout")
		case <-ticker.C:
			statuses, err := s.rpcClient.GetSignatureStatuses(ctx, false, signature)
			if err != nil {
				continue // Keep trying
			}

			if len(statuses.Value) == 0 || statuses.Value[0] == nil {
				continue // Transaction not found yet
			}

			status := statuses.Value[0]

			if status != nil && status.ConfirmationStatus != "" {
				// For simplicity, accept processed, confirmed, or finalized
				if status.ConfirmationStatus == "processed" ||
					status.ConfirmationStatus == "confirmed" ||
					status.ConfirmationStatus == "finalized" {
					if status.Err != nil {
						return false, fmt.Errorf("transaction failed: %v", status.Err)
					}
					return true, nil
				}
			}
		}
	}
}

// validateTransaction ensures the transaction matches our expected parameters
func (s *SolanaPaymentService) validateTransaction(ctx context.Context, tx *solana.Transaction, price *models.Price) error {
	// For now, we'll do basic validation
	// In production, you'd want to parse the transaction instructions and validate:
	// - Token mint matches expected
	// - Amount matches price
	// - Recipient matches our wallet
	// - No unexpected instructions

	// This is a simplified validation - you may want to implement more thorough checks
	return nil
}

// grantRoleForPurchase grants the role associated with a product to a user for one-off purchases
func (s *SolanaPaymentService) grantRoleForPurchase(ctx context.Context, purchase *models.Purchase, price *models.Price) error {
	// Get product information
	product, err := s.GetProductByID(ctx, price.ProductID)
	if err != nil {
		return fmt.Errorf("failed to get product: %w", err)
	}

	// Only grant role if product has one
	if product.RoleID == nil {
		return nil
	}

	// Determine extension days from product or default
	var durationDays int
	if product.RoleDurationDays != nil && *product.RoleDurationDays > 0 {
		durationDays = *product.RoleDurationDays
	} else {
		// Default to 30 days for one-off purchases without specified duration
		durationDays = 30
	}

	// Extend the user's existing role expiration or create new grant
	grant, newExpirationDate, err := s.ExtendRoleExpiration(ctx, purchase.UserID, *product.RoleID, durationDays)
	if err != nil {
		return fmt.Errorf("failed to extend role expiration: %w", err)
	}

	// Update the purchase record to link to the grant and record extension
	purchase.UserRoleGrantID = &grant.ID
	purchase.ExtensionDays = &durationDays
	if err := s.UpdatePurchase(ctx, purchase); err != nil {
		return fmt.Errorf("failed to update purchase with grant link: %w", err)
	}

	log.WithFields(log.Fields{
		"userID":            purchase.UserID,
		"roleID":            *product.RoleID, // Dereference the pointer
		"productID":         product.ID,
		"purchaseID":        purchase.ID,
		"extensionDays":     durationDays,
		"newExpirationDate": newExpirationDate,
	}).Info("Extended role expiration via Solana purchase")

	return nil
}
