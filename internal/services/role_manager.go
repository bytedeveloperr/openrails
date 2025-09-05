package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// RoleAction represents the action taken by a role function
type RoleAction string

const (
	RoleActionInsert    RoleAction = "insert"
	RoleActionMerged    RoleAction = "merged"
	RoleActionExtended  RoleAction = "extended"
	RoleActionClosed    RoleAction = "closed"
	RoleActionIdempotent RoleAction = "idempotent"
	RoleActionNoop      RoleAction = "noop"
	RoleActionUpserted  RoleAction = "upserted"
	RoleActionDeleted   RoleAction = "deleted"
)

// RoleManager handles role operations in the external client database
type RoleManager struct {
	externalDB *bun.DB
	enabled    bool
}

// RoleResult represents the result of a role operation
type RoleResult struct {
    UserRoleID uuid.UUID  `bun:"user_role_id"`
    Action     RoleAction `bun:"action"`
}

// NewRoleManager creates a new role manager
func NewRoleManager(externalDBURL string) (*RoleManager, error) {
	if externalDBURL == "" {
		// Role management is optional - billing service works without it
		return &RoleManager{enabled: false}, nil
	}

	// Create external database connection
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(externalDBURL)))
	externalDB := bun.NewDB(sqldb, pgdialect.New())

	// Test connection
	ctx := context.Background()
	if err := externalDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to external database: %w", err)
	}

	return &RoleManager{
		externalDB: externalDB,
		enabled:    true,
	}, nil
}

// IsEnabled returns whether role management is configured
func (rm *RoleManager) IsEnabled() bool {
	return rm.enabled
}

// OpenOrEnsureUserRole opens or ensures a user role in the client database
func (rm *RoleManager) OpenOrEnsureUserRole(ctx context.Context, params OpenRoleParams) (*RoleResult, error) {
	if !rm.enabled {
		return nil, fmt.Errorf("role management not configured")
	}

	var result RoleResult
	query := `
		SELECT * FROM billing_open_or_ensure_user_role($1, $2, $3, $4, $5, $6, $7)
	`

	err := rm.externalDB.NewRaw(query,
		params.UserID,
		params.RoleSlug,
		params.StartAt,
		params.EndAt,
		params.SourceType,
		params.SourceID,
		params.IdempotencyKey,
	).Scan(ctx, &result)

	if err != nil {
		return nil, fmt.Errorf("failed to open/ensure user role: %w", err)
	}

	return &result, nil
}

// ExtendUserRole extends a user role in the client database
func (rm *RoleManager) ExtendUserRole(ctx context.Context, params ExtendRoleParams) (*RoleResult, error) {
	if !rm.enabled {
		return nil, fmt.Errorf("role management not configured")
	}

	var result RoleResult
	query := `
		SELECT * FROM billing_extend_user_role($1, $2, $3, $4)
	`

	err := rm.externalDB.NewRaw(query,
		params.UserID,
		params.RoleSlug,
		params.NewEndAt,
		params.IdempotencyKey,
	).Scan(ctx, &result)

	if err != nil {
		return nil, fmt.Errorf("failed to extend user role: %w", err)
	}

	return &result, nil
}

// CloseUserRole closes a user role in the client database
func (rm *RoleManager) CloseUserRole(ctx context.Context, params CloseRoleParams) (*RoleResult, error) {
	if !rm.enabled {
		return nil, fmt.Errorf("role management not configured")
	}

	var result RoleResult
	query := `
		SELECT * FROM billing_close_user_role($1, $2, $3, $4, $5)
	`

	err := rm.externalDB.NewRaw(query,
		params.UserID,
		params.RoleSlug,
		params.EffectiveAt,
		params.RevokeReason,
		params.IdempotencyKey,
	).Scan(ctx, &result)

	if err != nil {
		return nil, fmt.Errorf("failed to close user role: %w", err)
	}

	return &result, nil
}

// Close closes the external database connection
func (rm *RoleManager) Close() error {
	if rm.externalDB != nil {
		return rm.externalDB.Close()
	}
	return nil
}

// Parameter structs for role operations

type OpenRoleParams struct {
    UserID         string
    RoleSlug       string
    StartAt        *time.Time // Optional
    EndAt          *time.Time // Optional
    SourceType     string     // 'subscription', 'one_off', etc.
    SourceID       *uuid.UUID // Optional subscription/payment ID
    IdempotencyKey *uuid.UUID // Optional for idempotency
}

type ExtendRoleParams struct {
    UserID         string
    RoleSlug       string
    NewEndAt       *time.Time // New expiration time
    IdempotencyKey *uuid.UUID // Optional for idempotency
}

type CloseRoleParams struct {
    UserID         string
    RoleSlug       string
    EffectiveAt    *time.Time // When to revoke (nil = now)
    RevokeReason   string     // 'admin', 'expired', 'cancelled', etc.
    IdempotencyKey *uuid.UUID // Optional for idempotency
}
