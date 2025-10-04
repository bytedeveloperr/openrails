package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/utils/solana"
	"github.com/google/uuid"
)

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletAlreadyExists = errors.New("wallet already exists")
)

// SolanaWalletService provides DB-backed operations for user wallets
type SolanaWalletService struct{ db *db.DB }

func NewSolanaWalletService(db *db.DB) *SolanaWalletService { return &SolanaWalletService{db: db} }
func (s *SolanaWalletService) GetDB() *db.DB                { return s.db }

// Link adds a wallet for a user if not present; returns existing or newly created
func (s *SolanaWalletService) Link(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return nil, fmt.Errorf("address validation failed: %w", err)
	}

	// Try to find existing
	var existing models.SolanaWallet
	err := s.db.GetDB().NewSelect().Model(&existing).
		Where("user_id = ? AND address = ?", userID, address).
		Scan(ctx)
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing wallet: %w", err)
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
		return nil, fmt.Errorf("failed to insert wallet: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows < 1 {
		return nil, fmt.Errorf("wallet insert affected 0 rows")
	}
	return w, nil
}

// List returns wallets for a user
func (s *SolanaWalletService) List(ctx context.Context, userID string) ([]*models.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}

	var wallets []*models.SolanaWallet
	if err := s.db.GetDB().NewSelect().Model(&wallets).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list wallets for user %s: %w", userID, err)
	}
	return wallets, nil
}

// Verify marks a wallet as verified
func (s *SolanaWalletService) Verify(ctx context.Context, userID, address string) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return fmt.Errorf("address validation failed: %w", err)
	}

	now := time.Now()
	res, err := s.db.GetDB().NewUpdate().Model((*models.SolanaWallet)(nil)).
		Set("is_verified = ?", true).
		Set("verified_at = ?", &now).
		Set("updated_at = ?", now).
		Where("user_id = ? AND address = ?", userID, address).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify wallet %s for user %s: %w", address, userID, err)
	}
	if rows, _ := res.RowsAffected(); rows < 1 {
		return fmt.Errorf("%w: wallet %s for user %s", ErrWalletNotFound, address, userID)
	}
	return nil
}

// Delete removes a wallet for a user
func (s *SolanaWalletService) Delete(ctx context.Context, userID, address string) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return fmt.Errorf("address validation failed: %w", err)
	}

	res, err := s.db.GetDB().NewDelete().Model((*models.SolanaWallet)(nil)).
		Where("user_id = ? AND address = ?", userID, address).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete wallet %s for user %s: %w", address, userID, err)
	}
	if rows, _ := res.RowsAffected(); rows < 1 {
		return fmt.Errorf("%w: wallet %s for user %s", ErrWalletNotFound, address, userID)
	}
	return nil
}

// Get returns the latest record for a wallet belonging to a user.
func (s *SolanaWalletService) Get(ctx context.Context, userID, address string) (*models.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	if err := solana.ValidateAddress(address); err != nil {
		return nil, fmt.Errorf("address validation failed: %w", err)
	}

	var wallet models.SolanaWallet
	err := s.db.GetDB().NewSelect().Model(&wallet).
		Where("user_id = ? AND address = ?", userID, address).
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: wallet %s for user %s", ErrWalletNotFound, address, userID)
		}
		return nil, fmt.Errorf("failed to get wallet %s for user %s: %w", address, userID, err)
	}
	return &wallet, nil
}

// GetPrimary returns the most recently verified wallet for a user.
func (s *SolanaWalletService) GetPrimary(ctx context.Context, userID string) (*models.SolanaWallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}

	var wallet models.SolanaWallet
	err := s.db.GetDB().NewSelect().Model(&wallet).
		Where("user_id = ?", userID).
		OrderExpr("is_verified DESC").
		Order("updated_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: no wallet for user %s", ErrWalletNotFound, userID)
		}
		return nil, fmt.Errorf("failed to get primary wallet for user %s: %w", userID, err)
	}
	return &wallet, nil
}
