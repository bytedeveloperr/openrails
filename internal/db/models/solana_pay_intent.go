package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type SolanaPayIntent struct {
	bun.BaseModel `bun:"table:billing.solana_pay_intents,alias:spi"`

	ID        uuid.UUID  `bun:"id,pk,type:uuid" json:"id"`
	UserID    string     `bun:"user_id,notnull" json:"user_id"`
	Recipient string     `bun:"recipient,notnull" json:"recipient"`
	TokenMint *string    `bun:"token_mint,nullzero" json:"token_mint,omitempty"`
	Amount    int64      `bun:"amount,notnull" json:"amount"`
	Reference string     `bun:"reference,notnull" json:"reference"`
	Message   *string    `bun:"message,nullzero" json:"message,omitempty"`
	ExpiresAt *time.Time `bun:"expires_at,nullzero" json:"expires_at,omitempty"`
	CreatedAt time.Time  `bun:"created_at,notnull" json:"created_at"`
}
