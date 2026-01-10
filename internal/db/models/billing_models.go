package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// SolanaTransaction tracks Solana payment transactions
type SolanaTransaction struct {
	bun.BaseModel `bun:"table:billing.solana_transactions,alias:stx"`

	ID     uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID *string   `bun:"" json:"user_id,omitempty"`

	// Transaction details
	Signature *string `bun:"" json:"signature,omitempty"` // Solana transaction signature
	Status    string  `bun:",notnull" json:"status"`      // pending, confirmed, failed

	// Payment details - amount in smallest unit (lamports for SOL, base units for SPL tokens)
	Amount    int64  `bun:",notnull" json:"amount"`
	Token     string `bun:",notnull" json:"token"`      // SOL, USDC, PYUSD
	TokenMint string `bun:",notnull" json:"token_mint"` // Token mint address

	// Addresses
	FromAddress string `bun:",notnull" json:"from_address"` // Payer address
	ToAddress   string `bun:",notnull" json:"to_address"`   // Recipient address

	// Product reference
	ProductID *uuid.UUID `bun:",type:uuid" json:"product_id,omitempty"`
	PaymentID *uuid.UUID `bun:",type:uuid" json:"payment_id,omitempty"`

	// Blockchain details
	BlockTime      *time.Time `bun:",nullzero" json:"block_time,omitempty"`
	Slot           *int64     `bun:"" json:"slot,omitempty"`
	Confirmations  int        `bun:",default:0" json:"confirmations"`
	TransactionFee *int64     `bun:"" json:"transaction_fee,omitempty"` // Fee in lamports

	// Processing metadata
	ProcessingResult map[string]interface{} `bun:",type:jsonb" json:"processing_result,omitempty"`
	ErrorMessage     *string                `bun:"" json:"error_message,omitempty"`

	// QR code reference (for payments initiated via QR)
	QRCodeID *string `bun:"" json:"qr_code_id,omitempty"`

	// Expiration for pending transactions
	ExpiresAt *time.Time `bun:",nullzero" json:"expires_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

const (
	TableSolanaTransactions = "solana_transactions"
)
