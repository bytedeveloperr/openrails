package services

import (
    "context"
    "database/sql"

    "github.com/doujins-org/doujins-billing/internal/db"
)

// UserService provides minimal user lookups for legacy call sites.
// In this project, Zitadel is the source of truth; most flows should
// carry the subject (`user.ID`) directly rather than lookups.
type UserService struct {
    db *db.DB
}

func NewUserService(dbx *db.DB) *UserService { return &UserService{db: dbx} }

// GetGoTrueUserByEmail is not supported in the Zitadel-only setup.
// Return sql.ErrNoRows so callers can gracefully skip when unknown.
func (s *UserService) GetGoTrueUserByEmail(ctx context.Context, email string) (*UserIdentity, error) {
    return nil, sql.ErrNoRows
}

// GetGoTrueUserByID returns a minimal identity with the provided ID.
func (s *UserService) GetGoTrueUserByID(ctx context.Context, id string) (*UserIdentity, error) {
    return &UserIdentity{ID: id}, nil
}

