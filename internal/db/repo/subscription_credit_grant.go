package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/open-rails/openrails/internal/db"
	"github.com/open-rails/openrails/internal/db/models"
	"github.com/uptrace/bun"
)

type SubscriptionCreditGrantRepo struct {
	db *db.DB
}

func NewSubscriptionCreditGrantRepo(d *db.DB) *SubscriptionCreditGrantRepo {
	return &SubscriptionCreditGrantRepo{db: d}
}

// InsertIfNotExists inserts an idempotency record for (subscription, credit_type, period_end).
// Returns true if inserted (caller should proceed with granting), false if already exists.
func (r *SubscriptionCreditGrantRepo) InsertIfNotExists(ctx context.Context, tx bun.Tx, grant *models.SubscriptionCreditGrant) (bool, error) {
	if grant == nil {
		return false, errors.New("grant is nil")
	}
	if grant.ID == uuid.Nil {
		grant.ID = uuid.New()
	}
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = time.Now().UTC()
	}

	res, err := tx.NewInsert().
		Model(grant).
		On("CONFLICT (subscription_id, credit_type_id, period_end) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
