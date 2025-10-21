package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type SolanaWalletRepo struct {
	db *db.DB
}

func NewSolanaWalletRepo(d *db.DB) *SolanaWalletRepo { return &SolanaWalletRepo{db: d} }

func (r *SolanaWalletRepo) GetByUserAndAddress(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	wallet := new(models.SolanaWallet)
	err = r.db.GetDB().NewSelect().
		Model(wallet).
		TableExpr(r.db.QualifiedTable("solana_wallets")).
		Where("user_id = ? AND address = ?", uid, address).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

func (r *SolanaWalletRepo) Insert(ctx context.Context, wallet *models.SolanaWallet) error {
	res, err := r.db.GetDB().NewInsert().Model(wallet).TableExpr(r.db.QualifiedTable("solana_wallets")).Exec(ctx)
	if err != nil {
		return err
	}
	if rows, err := res.RowsAffected(); err != nil {
		return err
	} else if rows < 1 {
		return ErrNoRowsAffected
	}
	return nil
}

func (r *SolanaWalletRepo) ListByUser(ctx context.Context, userID string) ([]*models.SolanaWallet, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	wallets := []*models.SolanaWallet{}
	if err := r.db.GetDB().NewSelect().Model(&wallets).
		TableExpr(r.db.QualifiedTable("solana_wallets")).
		Where("user_id = ?", uid).
		Order("created_at DESC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return wallets, nil
}

func (r *SolanaWalletRepo) MarkVerified(ctx context.Context, userID, address string, verifiedAt time.Time) (int64, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return 0, err
	}
	res, err := r.db.GetDB().NewUpdate().
		Model((*models.SolanaWallet)(nil)).
		TableExpr(r.db.QualifiedTable("solana_wallets")).
		Set("is_verified = ?", true).
		Set("verified_at = ?", &verifiedAt).
		Set("updated_at = ?", verifiedAt).
		Where("user_id = ? AND address = ?", uid, address).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *SolanaWalletRepo) Delete(ctx context.Context, userID, address string) (int64, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return 0, err
	}
	res, err := r.db.GetDB().NewDelete().
		Model((*models.SolanaWallet)(nil)).
		TableExpr(r.db.QualifiedTable("solana_wallets")).
		Where("user_id = ? AND address = ?", uid, address).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *SolanaWalletRepo) GetLatest(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	wallet := new(models.SolanaWallet)
	err = r.db.GetDB().NewSelect().
		Model(wallet).
		TableExpr(r.db.QualifiedTable("solana_wallets")).
		Where("user_id = ? AND address = ?", uid, address).
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

func (r *SolanaWalletRepo) GetPrimary(ctx context.Context, userID string) (*models.SolanaWallet, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	wallet := new(models.SolanaWallet)
	err = r.db.GetDB().NewSelect().
		Model(wallet).
		TableExpr(r.db.QualifiedTable("solana_wallets")).
		Where("user_id = ?", uid).
		OrderExpr("is_verified DESC").
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

var ErrNoRowsAffected = fmt.Errorf("no rows affected")
