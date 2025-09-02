package services

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"strings"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type IdempotencyService struct {
	db *db.DB
}

func NewIdempotencyService(db *db.DB) *IdempotencyService { return &IdempotencyService{db: db} }

// Begin attempts to create a pending idempotency record. Returns existing record if it already exists.
func (r *IdempotencyService) Begin(ctx context.Context, operation, key string, userID *uuid.UUID) (*models.IdempotencyRequest, bool, error) {
	// Try to fetch existing first
	existing, err := r.Get(ctx, operation, key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if existing != nil {
		return existing, true, nil
	}

	rec := &models.IdempotencyRequest{
		Operation: operation,
		Key:       key,
		UserID:    userID,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err = r.db.GetDB().NewInsert().Model(rec).Exec(ctx)
	if err != nil {
		// Unique violation race, fetch existing
		if isUniqueConstraintError(err) {
			existing, err = r.Get(ctx, operation, key)
			if err != nil {
				return nil, false, err
			}
			return existing, true, nil
		}
		return nil, false, err
	}
	return rec, false, nil
}

// isUniqueConstraintError checks common PG unique violation; lightweight helper here
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	// crude string check avoids importing driver-specific codes here
	s := err.Error()
	return strings.Contains(s, "unique") || strings.Contains(s, "UNIQUE") || strings.Contains(s, "duplicate key")
}

func (r *IdempotencyService) Complete(ctx context.Context, operation, key string, resultJSON []byte) error {
	_, err := r.db.GetDB().NewUpdate().Model((*models.IdempotencyRequest)(nil)).
		Set("status = ?", "success").
		Set("result_json = ?", resultJSON).
		Set("updated_at = now()").
		Where("operation = ? AND key = ?", operation, key).
		Exec(ctx)
	return err
}

func (r *IdempotencyService) Get(ctx context.Context, operation, key string) (*models.IdempotencyRequest, error) {
	var rec models.IdempotencyRequest
	err := r.db.GetDB().NewSelect().Model(&rec).Where("operation = ? AND key = ?", operation, key).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// DeleteOlderThan deletes idempotency records older than cutoff. If onlySuccess is true, restrict to status='success'.
func (r *IdempotencyService) DeleteOlderThan(ctx context.Context, cutoff time.Time, batchSize int, onlySuccess bool) (int, error) {
	q := r.db.GetDB().NewDelete().Model((*models.IdempotencyRequest)(nil)).Where("created_at < ?", cutoff)
	if onlySuccess {
		q = q.Where("status = ?", "success")
	}
	if batchSize > 0 {
		q = q.Limit(batchSize)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
