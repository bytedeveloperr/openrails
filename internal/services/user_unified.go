package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/supabase-community/gotrue-go/types"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
)

// UserService manages the complete user aggregate (auth.users + profiles + related data)
// This follows DDD principles where one Servicesitory handles one aggregate root
type UserService struct {
	db *db.DB
}

// GetUsersFilters provides filtering options for user queries
type GetUsersFilters struct {
	IsBanned *bool  `form:"is_banned"`
	Role     string `form:"role"`
}

type CreateUserRequest struct {
	Username string  // Required
	Email    *string // Optional
	Password string  // Required
	Bio      string  // Optional, defaults to empty
}

func NewUserService(db *db.DB) *UserService {
	return &UserService{db: db}
}

func (r *UserService) GetDB() *db.DB {
	return r.db
}

// GetByID returns a complete user (auth.users + profiles) by user ID
func (r *UserService) GetByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	var user models.User

	err := r.db.GetDB().NewSelect().
		Model(&user).
		ColumnExpr("u.id, u.email, u.email_confirmed_at, u.last_sign_in_at, u.created_at, u.updated_at").
		ColumnExpr("p.id as profile_id, p.username, p.bio").
		Join("LEFT JOIN profiles p ON p.user_id = u.id").
		Where("u.id = ?", userID).
		Where("u.deleted_at IS NULL").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	return &user, nil
}

// GetByUsername returns a user by username (searches profiles table)
func (r *UserService) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User

	err := r.db.GetDB().NewSelect().
		Model(&user).
		ColumnExpr("u.id, u.email, u.email_confirmed_at, u.last_sign_in_at, u.created_at, u.updated_at").
		ColumnExpr("p.id as profile_id, p.username, p.bio").
		Join("JOIN profiles p ON p.user_id = u.id").
		Where("p.username = ?", username).
		Where("u.deleted_at IS NULL").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return &user, nil
}

// GetByUserID returns a profile by user ID
func (r *UserService) GetByUserID(ctx context.Context, userID uuid.UUID) (*models.Profile, error) {
	var profile models.Profile
	err := r.db.GetDB().NewSelect().
		Model(&profile).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get profile by user ID: %w", err)
	}

	return &profile, nil
}

// GetByEmail returns a user by email (searches auth.users table)
func (r *UserService) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User

	err := r.db.GetDB().NewSelect().
		Model(&user).
		ColumnExpr("u.id, u.email, u.email_confirmed_at, u.last_sign_in_at, u.created_at, u.updated_at").
		ColumnExpr("p.id as profile_id, p.username, p.bio").
		Join("LEFT JOIN profiles p ON p.user_id = u.id").
		Where("u.email = ?", email).
		Where("u.deleted_at IS NULL").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &user, nil
}

// GetGoTrueUserByID returns a GoTrue user by ID (for compatibility)
func (r *UserService) GetGoTrueUserByID(ctx context.Context, id uuid.UUID) (*types.User, error) {
	var user types.User
	err := r.db.GetDB().NewSelect().
		TableExpr("auth.users").
		Column("id", "email", "created_at", "updated_at", "email_confirmed_at", "last_sign_in_at", "aud", "role", "phone", "banned_until").
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Scan(ctx, &user)
	if err != nil {
		return nil, fmt.Errorf("failed to get GoTrue user by ID: %w", err)
	}
	return &user, nil
}

// GetGoTrueUserByEmail returns a GoTrue user by email (for compatibility)
func (r *UserService) GetGoTrueUserByEmail(ctx context.Context, email string) (*types.User, error) {
	var user types.User
	err := r.db.GetDB().
		NewSelect().
		TableExpr("auth.users").
		Column("id", "email", "created_at", "updated_at", "email_confirmed_at", "last_sign_in_at", "aud", "role", "phone", "banned_until").
		Where("email = ?", email).
		Where("deleted_at IS NULL").
		Scan(ctx, &user)

	if err != nil {
		return nil, fmt.Errorf("failed to get GoTrue user by email: %w", err)
	}

	return &user, nil
}
