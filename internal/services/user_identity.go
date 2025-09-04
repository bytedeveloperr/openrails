package services

import (
    "context"
    "fmt"

    "github.com/doujins-org/doujins-billing/internal/db"
    "github.com/google/uuid"
    "github.com/supabase-community/gotrue-go/types"
)

// UserIdentity represents the minimal user information needed by billing
type UserIdentity struct {
    ID       uuid.UUID
    Email    *string
    Username string
    Roles    []string
}

// UserService provides minimal identity lookups against the auth.users table
type UserService struct {
    db *db.DB
}

func NewUserService(db *db.DB) *UserService { return &UserService{db: db} }

// GetEmailByUserID returns the user's email from auth.users, if present
func (s *UserService) GetEmailByUserID(ctx context.Context, id uuid.UUID) (string, error) {
    var email *string
    err := s.db.GetDB().
        NewSelect().
        TableExpr("auth.users").
        Column("email").
        Where("id = ?", id).
        Where("deleted_at IS NULL").
        Scan(ctx, &email)
    if err != nil {
        return "", fmt.Errorf("failed to fetch user email: %w", err)
    }
    if email == nil || *email == "" {
        return "", fmt.Errorf("user has no email")
    }
    return *email, nil
}

// GetGoTrueUserByID returns minimal user info from auth.users for admin enrichment
func (s *UserService) GetGoTrueUserByID(ctx context.Context, id uuid.UUID) (*types.User, error) {
    var user types.User
    err := s.db.GetDB().NewSelect().
        TableExpr("auth.users").
        Column("id", "email", "created_at", "updated_at", "email_confirmed_at", "last_sign_in_at", "aud", "role", "phone", "banned_until").
        Where("id = ?", id).
        Where("deleted_at IS NULL").
        Scan(ctx, &user)
    if err != nil {
        return nil, fmt.Errorf("failed to get user by id: %w", err)
    }
    return &user, nil
}

// GetGoTrueUserByEmail returns minimal user info from auth.users by email
func (s *UserService) GetGoTrueUserByEmail(ctx context.Context, email string) (*types.User, error) {
    var user types.User
    err := s.db.GetDB().NewSelect().
        TableExpr("auth.users").
        Column("id", "email", "created_at", "updated_at", "email_confirmed_at", "last_sign_in_at", "aud", "role", "phone", "banned_until").
        Where("email = ?", email).
        Where("deleted_at IS NULL").
        Scan(ctx, &user)
    if err != nil {
        return nil, fmt.Errorf("failed to get user by email: %w", err)
    }
    return &user, nil
}
