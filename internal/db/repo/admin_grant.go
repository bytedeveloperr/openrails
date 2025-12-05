package repo

import (
	"context"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type AdminGrantRepo struct {
	db *db.DB
}

func NewAdminGrantRepo(db *db.DB) *AdminGrantRepo {
	return &AdminGrantRepo{db: db}
}

// Create inserts a new admin grant record
func (r *AdminGrantRepo) Create(ctx context.Context, grant *models.AdminGrant) error {
	_, err := r.db.GetDB().NewInsert().Model(grant).Exec(ctx)
	return err
}

// GetByID retrieves an admin grant by ID
func (r *AdminGrantRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.AdminGrant, error) {
	grant := &models.AdminGrant{}
	err := r.db.GetDB().NewSelect().
		Model(grant).
		Where("ag.id = ?", id).
		Relation("Price").
		Relation("Payment").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return grant, nil
}

// ListByUserID retrieves all admin grants for a user
func (r *AdminGrantRepo) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]models.AdminGrant, int, error) {
	var grants []models.AdminGrant

	count, err := r.db.GetDB().NewSelect().
		Model(&grants).
		Where("ag.user_id = ?", userID).
		Relation("Price").
		Relation("Payment").
		Order("ag.created_at DESC").
		Limit(limit).
		Offset(offset).
		ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}

	return grants, count, nil
}

// ListByGrantedBy retrieves all admin grants made by a specific admin
func (r *AdminGrantRepo) ListByGrantedBy(ctx context.Context, grantedBy string, limit, offset int) ([]models.AdminGrant, int, error) {
	var grants []models.AdminGrant

	count, err := r.db.GetDB().NewSelect().
		Model(&grants).
		Where("ag.granted_by = ?", grantedBy).
		Relation("Price").
		Relation("Payment").
		Order("ag.created_at DESC").
		Limit(limit).
		Offset(offset).
		ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}

	return grants, count, nil
}
