package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/open-rails/openrails/internal/db"
	"github.com/open-rails/openrails/internal/db/models"
	"github.com/uptrace/bun"
)

var (
	ErrInsufficientCredits = errors.New("insufficient_credits")
)

type CreditsService struct {
	db    *db.DB
	Clock clockwork.Clock
}

func NewCreditsService(database *db.DB) *CreditsService {
	return &CreditsService{db: database, Clock: clockwork.NewRealClock()}
}

func (s *CreditsService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *CreditsService) GetCreditTypeByName(ctx context.Context, name string) (*models.CreditType, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("credits service not initialized")
	}
	ct := new(models.CreditType)
	err := s.db.GetDB().NewSelect().Model(ct).Where("name = ?", name).Limit(1).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return ct, nil
}

func (s *CreditsService) GetBalance(ctx context.Context, userID string, creditType string) (*models.UserCreditBalance, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("credits service not initialized")
	}
	ct, err := s.GetCreditTypeByName(ctx, creditType)
	if err != nil {
		return nil, err
	}
	bal := new(models.UserCreditBalance)
	err = s.db.GetDB().NewSelect().
		Model(bal).
		Where("user_id = ? AND credit_type_id = ?", userID, ct.ID).
		Limit(1).
		Scan(ctx)
	if err == nil {
		return bal, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return &models.UserCreditBalance{
			UserID:       userID,
			CreditTypeID: ct.ID,
			Balance:      0,
			HeldBalance:  0,
		}, nil
	}
	return nil, err
}

func (s *CreditsService) GetTransactions(ctx context.Context, userID string, creditType string, limit, offset int) ([]models.CreditTransaction, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, fmt.Errorf("credits service not initialized")
	}
	ct, err := s.GetCreditTypeByName(ctx, creditType)
	if err != nil {
		return nil, 0, err
	}
	var items []models.CreditTransaction
	q := s.db.GetDB().NewSelect().Model(&items).
		Where("user_id = ? AND credit_type_id = ?", userID, ct.ID)
	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	err = q.OrderExpr("created_at DESC").Limit(limit).Offset(offset).Scan(ctx)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type CreditDepositParams struct {
	UserID      string
	CreditType  string
	Amount      int64
	Source      string
	SourceID    *uuid.UUID
	ExpiresAt   *time.Time
	Description *string
}

func (s *CreditsService) Deposit(ctx context.Context, params CreditDepositParams) (*models.CreditTransaction, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("credits service not initialized")
	}
	if params.Amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	ct, err := s.GetCreditTypeByName(ctx, params.CreditType)
	if err != nil {
		return nil, err
	}
	if !ct.IsActive {
		return nil, ErrCreditTypeInactive
	}

	tx, err := s.db.GetDB().(*bun.DB).BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	trx, err := s.depositTx(ctx, tx, ct, params)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return trx, nil
}

func (s *CreditsService) getCreditTypeByNameTx(ctx context.Context, tx bun.Tx, name string) (*models.CreditType, error) {
	ct := new(models.CreditType)
	if err := tx.NewSelect().Model(ct).Where("name = ?", name).Limit(1).Scan(ctx); err != nil {
		return nil, err
	}
	return ct, nil
}

func (s *CreditsService) depositTx(ctx context.Context, tx bun.Tx, ct *models.CreditType, params CreditDepositParams) (*models.CreditTransaction, error) {
	now := s.now()
	bal, err := s.lockBalance(ctx, tx, params.UserID, ct.ID)
	if err != nil {
		return nil, err
	}

	// Idempotency: if caller provides SourceID, treat (user_id, credit_type_id, source, source_id)
	// as an idempotency key for deposits. This is safe because we lock the user_credit_balances row
	// for UPDATE, serializing deposits per user+credit_type.
	if params.SourceID != nil {
		existing := new(models.CreditTransaction)
		err := tx.NewSelect().
			Model(existing).
			Where("user_id = ? AND credit_type_id = ?", params.UserID, ct.ID).
			Where("transaction_type = 'deposit'").
			Where("source = ? AND source_id = ?", params.Source, *params.SourceID).
			Limit(1).
			Scan(ctx)
		if err == nil {
			return existing, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	newBal := bal.Balance + params.Amount

	if _, err := tx.NewUpdate().Model((*models.UserCreditBalance)(nil)).
		Set("balance = ?", newBal).
		Set("updated_at = ?", now).
		Where("user_id = ? AND credit_type_id = ?", params.UserID, ct.ID).
		Exec(ctx); err != nil {
		return nil, err
	}

	trx := &models.CreditTransaction{
		ID:              uuid.New(),
		UserID:          params.UserID,
		CreditTypeID:    ct.ID,
		Amount:          params.Amount,
		BalanceAfter:    newBal,
		TransactionType: "deposit",
		Source:          params.Source,
		SourceID:        params.SourceID,
		ExpiresAt:       params.ExpiresAt,
		Description:     params.Description,
		CreatedAt:       now,
	}
	if _, err := tx.NewInsert().Model(trx).Exec(ctx); err != nil {
		return nil, err
	}
	if params.ExpiresAt != nil {
		batch := &models.CreditExpiryBatch{
			ID:                  uuid.New(),
			UserID:              params.UserID,
			CreditTypeID:        ct.ID,
			OriginalAmount:      params.Amount,
			RemainingAmount:     params.Amount,
			ExpiresAt:           *params.ExpiresAt,
			SourceTransactionID: &trx.ID,
			CreatedAt:           now,
		}
		if _, err := tx.NewInsert().Model(batch).Exec(ctx); err != nil {
			return nil, err
		}
	}

	return trx, nil
}

type CreditWithdrawParams struct {
	UserID     string
	CreditType string
	Amount     int64
	Source     string
	SourceID   *uuid.UUID
}

func (s *CreditsService) Withdraw(ctx context.Context, params CreditWithdrawParams) (*models.CreditTransaction, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("credits service not initialized")
	}
	if params.Amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	ct, err := s.GetCreditTypeByName(ctx, params.CreditType)
	if err != nil {
		return nil, err
	}
	if !ct.IsActive {
		return nil, ErrCreditTypeInactive
	}

	tx, err := s.db.GetDB().(*bun.DB).BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	trx, err := s.withdrawTx(ctx, tx, ct.ID, params)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return trx, nil
}

func (s *CreditsService) Hold(ctx context.Context, userID string, creditType string, amount int64, source, sourceID string, expiresAt time.Time) (*models.CreditHold, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("credits service not initialized")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	ct, err := s.GetCreditTypeByName(ctx, creditType)
	if err != nil {
		return nil, err
	}
	if !ct.IsActive {
		return nil, ErrCreditTypeInactive
	}

	tx, err := s.db.GetDB().(*bun.DB).BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	now := s.now()
	bal, err := s.lockBalance(ctx, tx, userID, ct.ID)
	if err != nil {
		return nil, err
	}
	available := bal.Balance - bal.HeldBalance
	if available < amount {
		return nil, ErrInsufficientCredits
	}

	if _, err := tx.NewUpdate().Model((*models.UserCreditBalance)(nil)).
		Set("held_balance = ?", bal.HeldBalance+amount).
		Set("updated_at = ?", now).
		Where("user_id = ? AND credit_type_id = ?", userID, ct.ID).
		Exec(ctx); err != nil {
		return nil, err
	}

	hold := &models.CreditHold{
		ID:           uuid.New(),
		UserID:       userID,
		CreditTypeID: ct.ID,
		Amount:       amount,
		Source:       source,
		SourceID:     sourceID,
		Status:       "active",
		ExpiresAt:    expiresAt.UTC(),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := tx.NewInsert().Model(hold).Exec(ctx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return hold, nil
}

func (s *CreditsService) CaptureHold(ctx context.Context, holdID uuid.UUID, actualAmount int64) (*models.CreditTransaction, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("credits service not initialized")
	}
	tx, err := s.db.GetDB().(*bun.DB).BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	hold := new(models.CreditHold)
	if err := tx.NewSelect().Model(hold).Where("id = ?", holdID).For("UPDATE").Scan(ctx); err != nil {
		return nil, err
	}
	if hold.Status != "active" {
		return nil, fmt.Errorf("hold is not active")
	}
	if actualAmount <= 0 || actualAmount > hold.Amount {
		return nil, fmt.Errorf("invalid capture amount")
	}

	now := s.now()
	bal, err := s.lockBalance(ctx, tx, hold.UserID, hold.CreditTypeID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.NewUpdate().Model((*models.UserCreditBalance)(nil)).
		Set("held_balance = ?", bal.HeldBalance-hold.Amount).
		Set("updated_at = ?", now).
		Where("user_id = ? AND credit_type_id = ?", hold.UserID, hold.CreditTypeID).
		Exec(ctx); err != nil {
		return nil, err
	}

	hold.Status = "captured"
	hold.Captured = &actualAmount
	hold.UpdatedAt = now
	if _, err := tx.NewUpdate().Model(hold).WherePK().Exec(ctx); err != nil {
		return nil, err
	}

	trx, err := s.withdrawTx(ctx, tx, hold.CreditTypeID, CreditWithdrawParams{
		UserID:     hold.UserID,
		CreditType: "",
		Amount:     actualAmount,
		Source:     "hold",
		SourceID:   &hold.ID,
	})
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return trx, nil
}

func (s *CreditsService) ReleaseHold(ctx context.Context, holdID uuid.UUID) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("credits service not initialized")
	}
	tx, err := s.db.GetDB().(*bun.DB).BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	hold := new(models.CreditHold)
	if err := tx.NewSelect().Model(hold).Where("id = ?", holdID).For("UPDATE").Scan(ctx); err != nil {
		return err
	}
	if hold.Status != "active" {
		return fmt.Errorf("hold is not active")
	}

	now := s.now()
	bal, err := s.lockBalance(ctx, tx, hold.UserID, hold.CreditTypeID)
	if err != nil {
		return err
	}
	if _, err := tx.NewUpdate().Model((*models.UserCreditBalance)(nil)).
		Set("held_balance = ?", bal.HeldBalance-hold.Amount).
		Set("updated_at = ?", now).
		Where("user_id = ? AND credit_type_id = ?", hold.UserID, hold.CreditTypeID).
		Exec(ctx); err != nil {
		return err
	}

	hold.Status = "released"
	hold.UpdatedAt = now
	if _, err := tx.NewUpdate().Model(hold).WherePK().Exec(ctx); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *CreditsService) lockBalance(ctx context.Context, tx bun.Tx, userID string, creditTypeID uuid.UUID) (*models.UserCreditBalance, error) {
	bal := new(models.UserCreditBalance)
	err := tx.NewSelect().Model(bal).
		Where("user_id = ? AND credit_type_id = ?", userID, creditTypeID).
		For("UPDATE").
		Scan(ctx)
	if err == nil {
		return bal, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := s.now()
	bal = &models.UserCreditBalance{
		ID:           uuid.New(),
		UserID:       userID,
		CreditTypeID: creditTypeID,
		Balance:      0,
		HeldBalance:  0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := tx.NewInsert().Model(bal).Exec(ctx); err != nil {
		return nil, err
	}
	return bal, nil
}

func (s *CreditsService) withdrawTx(ctx context.Context, tx bun.Tx, creditTypeID uuid.UUID, params CreditWithdrawParams) (*models.CreditTransaction, error) {
	now := s.now()
	bal, err := s.lockBalance(ctx, tx, params.UserID, creditTypeID)
	if err != nil {
		return nil, err
	}
	available := bal.Balance - bal.HeldBalance
	if available < params.Amount {
		return nil, ErrInsufficientCredits
	}

	remaining := params.Amount
	var batches []models.CreditExpiryBatch
	if err := tx.NewSelect().Model(&batches).
		Where("user_id = ? AND credit_type_id = ? AND remaining_amount > 0 AND expires_at > ?", params.UserID, creditTypeID, now).
		OrderExpr("expires_at ASC").
		For("UPDATE").
		Scan(ctx); err != nil {
		return nil, err
	}
	for i := range batches {
		if remaining == 0 {
			break
		}
		use := batches[i].RemainingAmount
		if use > remaining {
			use = remaining
		}
		if use <= 0 {
			continue
		}
		batches[i].RemainingAmount -= use
		remaining -= use
		if _, err := tx.NewUpdate().Model(&batches[i]).
			Column("remaining_amount").
			WherePK().
			Exec(ctx); err != nil {
			return nil, err
		}
	}
	bal.Balance -= params.Amount

	if _, err := tx.NewUpdate().Model((*models.UserCreditBalance)(nil)).
		Set("balance = ?", bal.Balance).
		Set("updated_at = ?", now).
		Where("user_id = ? AND credit_type_id = ?", params.UserID, creditTypeID).
		Exec(ctx); err != nil {
		return nil, err
	}

	trx := &models.CreditTransaction{
		ID:              uuid.New(),
		UserID:          params.UserID,
		CreditTypeID:    creditTypeID,
		Amount:          -params.Amount,
		BalanceAfter:    bal.Balance,
		TransactionType: "withdrawal",
		Source:          params.Source,
		SourceID:        params.SourceID,
		CreatedAt:       now,
	}
	if _, err := tx.NewInsert().Model(trx).Exec(ctx); err != nil {
		return nil, err
	}
	return trx, nil
}
