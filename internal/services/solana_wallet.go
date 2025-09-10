package services

import (
	"context"
	"errors"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

// SolanaWalletService provides DB-backed operations for user wallets
type SolanaWalletService struct{ db *db.DB }

func NewSolanaWalletService(db *db.DB) *SolanaWalletService { return &SolanaWalletService{db: db} }
func (s *SolanaWalletService) GetDB() *db.DB                { return s.db }

// Link adds a wallet for a user if not present; returns existing or newly created
func (s *SolanaWalletService) Link(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	// Try to find existing
	var existing models.SolanaWallet
	err := s.db.GetDB().NewSelect().Model(&existing).
		Where("user_id = ? AND address = ?", userID, address).
		Scan(ctx)
	if err == nil {
		return &existing, nil
	}

	// Create new
	now := time.Now()
	w := &models.SolanaWallet{
		ID:         uuid.New(),
		UserID:     userID,
		Address:    address,
		IsVerified: false,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	res, err := s.db.GetDB().NewInsert().Model(w).Exec(ctx)
	if err != nil {
		return nil, err
	}
	if rows, _ := res.RowsAffected(); rows < 1 {
		return nil, errors.New("no rows affected")
	}
	return w, nil
}

// List returns wallets for a user
func (s *SolanaWalletService) List(ctx context.Context, userID string) ([]*models.SolanaWallet, error) {
	var wallets []*models.SolanaWallet
	if err := s.db.GetDB().NewSelect().Model(&wallets).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return wallets, nil
}

// Verify marks a wallet as verified
func (s *SolanaWalletService) Verify(ctx context.Context, userID, address string) error {
	now := time.Now()
	res, err := s.db.GetDB().NewUpdate().Model((*models.SolanaWallet)(nil)).
		Set("is_verified = ?", true).
		Set("verified_at = ?", &now).
		Set("updated_at = ?", now).
		Where("user_id = ? AND address = ?", userID, address).
		Exec(ctx)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}

// Delete removes a wallet for a user
func (s *SolanaWalletService) Delete(ctx context.Context, userID, address string) error {
	res, err := s.db.GetDB().NewDelete().Model((*models.SolanaWallet)(nil)).
		Where("user_id = ? AND address = ?", userID, address).
		Exec(ctx)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows < 1 {
		return errors.New("no rows affected")
	}
	return nil
}
