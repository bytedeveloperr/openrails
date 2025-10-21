package repo

import (
	"context"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/google/uuid"
)

type SolanaTransactionRepo struct {
	db *db.DB
}

func NewSolanaTransactionRepo(d *db.DB) *SolanaTransactionRepo { return &SolanaTransactionRepo{db: d} }

func (r *SolanaTransactionRepo) MarkConfirmedByUserAndAmount(ctx context.Context, userID string, amount float64, signature string) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return err
	}

	_, err = r.db.GetDB().NewUpdate().
		TableExpr(r.db.QualifiedTable("solana_transactions")).
		Set("status = ?", "confirmed").
		Set("signature = ?", signature).
		Where("user_id = ?", uid).
		Where("amount = ?", amount).
		Exec(ctx)

	return err
}
