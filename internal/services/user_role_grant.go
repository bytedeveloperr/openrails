package services

import (
	"context"
	"errors"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

type UserRoleGrantService struct {
	db *db.DB
}

type GetUserRoleGrantsFilters struct {
    UserID string    `form:"user_id"`
    RoleID uuid.UUID `form:"role_id"`
}

func NewUserRoleGrantService(db *db.DB) *UserRoleGrantService {
	return &UserRoleGrantService{db: db}
}

func (r *UserRoleGrantService) GetDB() *db.DB {
	return r.db
}

func (r *UserRoleGrantService) Create(ctx context.Context, grant *models.UserRoleGrant) error {
	result, err := r.db.GetDB().NewInsert().Model(grant).Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (r *UserRoleGrantService) GetByID(ctx context.Context, id uuid.UUID) (*models.UserRoleGrant, error) {
	var grant models.UserRoleGrant
	err := r.db.GetDB().NewSelect().Model(&grant).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &grant, nil
}

func (r *UserRoleGrantService) GetByUserID(ctx context.Context, userID string) ([]*models.UserRoleGrant, error) {
	var grants []*models.UserRoleGrant
	err := r.db.GetDB().NewSelect().Model(&grants).Where("user_id = ?", userID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return grants, nil
}

func (r *UserRoleGrantService) GetActiveByUserID(ctx context.Context, userID string) ([]*models.UserRoleGrant, error) {
	var grants []*models.UserRoleGrant
	now := time.Now()
	err := r.db.GetDB().NewSelect().Model(&grants).
		Where("user_id = ?", userID).
		Where("(auto_expires_at IS NULL OR auto_expires_at > ?)", now).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return grants, nil
}

func (r *UserRoleGrantService) GetBySubSourceID(ctx context.Context, subSourceID uuid.UUID) ([]*models.UserRoleGrant, error) {
	var grants []*models.UserRoleGrant
	err := r.db.GetDB().NewSelect().Model(&grants).Where("sub_source_id = ?", subSourceID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return grants, nil
}

// Removed: GetByGrantSource — grant_source dropped from schema; use payments-linked audit instead

func (r *UserRoleGrantService) GetExpired(ctx context.Context) ([]*models.UserRoleGrant, error) {
	var grants []*models.UserRoleGrant
	now := time.Now()
	err := r.db.GetDB().NewSelect().Model(&grants).
		Where("auto_expires_at IS NOT NULL").
		Where("auto_expires_at <= ?", now).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return grants, nil
}

func (r *UserRoleGrantService) Update(ctx context.Context, grant *models.UserRoleGrant) error {
	result, err := r.db.GetDB().NewUpdate().Model(grant).WherePK().Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (r *UserRoleGrantService) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.GetDB().NewDelete().Model((*models.UserRoleGrant)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

// RevokeBySubSourceID revokes all role grants associated with a subscription
func (r *UserRoleGrantService) RevokeBySubSourceID(ctx context.Context, subSourceID uuid.UUID) error {
	result, err := r.db.GetDB().NewDelete().Model((*models.UserRoleGrant)(nil)).Where("sub_source_id = ?", subSourceID).Exec(ctx)
	if err != nil {
		return err
	}

	_, err = result.RowsAffected()
	if err != nil {
		return err
	}

	return nil
}

// CleanupExpiredGrants removes expired role grants
func (r *UserRoleGrantService) CleanupExpiredGrants(ctx context.Context) (int64, error) {
	now := time.Now()
	result, err := r.db.GetDB().NewDelete().Model((*models.UserRoleGrant)(nil)).
		Where("auto_expires_at IS NOT NULL").
		Where("auto_expires_at <= ?", now).
		Exec(ctx)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// ===== USER-ROLE RELATIONSHIP FUNCTIONS =====
// These functions manage the relationship between users and roles using UserRoleGrant

// HasRole checks if a user has a specific role
func (r *UserRoleGrantService) HasRole(ctx context.Context, userID string, roleID uuid.UUID) (bool, error) {
	db := r.db.GetDB()
	return db.NewSelect().
		Model((*models.UserRoleGrant)(nil)).
		Where("user_id = ?", userID).
		Where("role_id = ?", roleID).
		Where("(auto_expires_at IS NULL OR auto_expires_at > NOW())").
		Exists(ctx)
}

// HasRoleSlug checks if a user has a role by slug
func (r *UserRoleGrantService) HasRoleSlug(ctx context.Context, userID string, slug string) (bool, error) {
	db := r.db.GetDB()

	result, err := db.NewSelect().
		Model((*models.UserRoleGrant)(nil)).
		Join("JOIN roles ON roles.id = urg.role_id").
		Where("urg.user_id = ?", userID).
		Where("roles.slug = ?", slug).
		Where("(urg.auto_expires_at IS NULL OR urg.auto_expires_at > NOW())").
		Exists(ctx)
	if err != nil {
		return false, err
	}

	return result, nil
}

// GrantRole grants a role to a user using the new UserRoleGrant model
// NOTE: This is a simplified version with admin manual source. For full control over grant source and expiration,
// use the Create method directly.
func (r *UserRoleGrantService) GrantRole(ctx context.Context, userID string, roleID uuid.UUID) error {
	// Check if user already has this role
	exists, err := r.HasRole(ctx, userID, roleID)
	if err != nil {
		return err
	}

	if exists {
		return nil // Already has the role
	}

	// Create new role grant
	userRoleGrant := &models.UserRoleGrant{
		ID:        uuid.New(),
		UserID:    userID,
		RoleID:    roleID,
		GrantedAt: time.Now(),
	}

	return r.Create(ctx, userRoleGrant)
}

// RevokeRole revokes a role from a user (deletes all grants for that role)
func (r *UserRoleGrantService) RevokeRole(ctx context.Context, userID string, roleID uuid.UUID) error {
	db := r.db.GetDB()
	_, err := db.NewDelete().
		Model((*models.UserRoleGrant)(nil)).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Exec(ctx)
	return err
}

// GetRolesForUser fetches all active roles for a user
func (r *UserRoleGrantService) GetRolesForUser(ctx context.Context, userID string) ([]*models.Role, error) {
	var roles []*models.Role
	err := r.db.GetDB().NewSelect().
		Model(&roles).
		Join("JOIN user_role_grants urg ON urg.role_id = r.id").
		Where("urg.user_id = ?", userID).
		Where("(urg.auto_expires_at IS NULL OR urg.auto_expires_at > NOW())").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return roles, nil
}

// GetRolesForUsers fetches all roles for multiple users in a single query
func (r *UserRoleGrantService) GetRolesForUsers(ctx context.Context, userIDs []string) (map[string][]*models.Role, error) {
	var roles []*models.Role
	var userRoleGrants []models.UserRoleGrant

	// First get all user_role_grants for the given users
    err := r.db.GetDB().NewSelect().
        Model(&userRoleGrants).
        Where("user_id IN (?)", bun.In(userIDs)).
		Where("(auto_expires_at IS NULL OR auto_expires_at > NOW())").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

    if len(userRoleGrants) == 0 {
        return make(map[string][]*models.Role), nil
    }

	roleIDs := make([]uuid.UUID, 0, len(userRoleGrants))
	for _, urg := range userRoleGrants {
		roleIDs = append(roleIDs, urg.RoleID)
	}

	err = r.db.GetDB().NewSelect().
		Model(&roles).
		Where("id IN (?)", bun.In(roleIDs)).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	// Create a map of role ID to role for quick lookup
	roleMap := make(map[uuid.UUID]*models.Role)
	for _, role := range roles {
		roleMap[role.ID] = role
	}

	// Create the final map of user ID to roles
    result := make(map[string][]*models.Role)
    for _, urg := range userRoleGrants {
        if role, exists := roleMap[urg.RoleID]; exists {
            result[urg.UserID] = append(result[urg.UserID], role)
        }
    }

	return result, nil
}

// IsUserAdmin checks if a user has admin role
func (r *UserRoleGrantService) IsUserAdmin(ctx context.Context, userID string) (bool, error) {
	result, err := r.HasRoleSlug(ctx, userID, "admin")
	if err != nil {
		log.WithError(err).WithField("userID", userID).Error("Failed to check admin role")
		return false, err
	}

	return result, nil
}

// GetOrCreateUserRoleGrant gets existing grant or creates new grant for user+role
func (r *UserRoleGrantService) GetOrCreateUserRoleGrant(ctx context.Context, userID string, roleID uuid.UUID) (*models.UserRoleGrant, error) {
	// Try to get existing grant
	var grant models.UserRoleGrant
	err := r.db.GetDB().NewSelect().Model(&grant).
		Where("user_id = ?", userID).
		Where("role_id = ?", roleID).
		Scan(ctx)

	if err == nil {
		// Grant exists, return it
		return &grant, nil
	}

	// Grant doesn't exist, create new one with initial expiration
	now := time.Now()
	newGrant := &models.UserRoleGrant{
		ID:            uuid.New(),
		UserID:        userID,
		RoleID:        roleID,
		AutoExpiresAt: nil, // Will be set when first purchase extends it
		GrantedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := r.Create(ctx, newGrant); err != nil {
		return nil, err
	}

	return newGrant, nil
}

// GetActiveRoleGrant gets the active role grant for a specific user and role (if not expired)
func (r *UserRoleGrantService) GetActiveRoleGrant(ctx context.Context, userID string, roleID uuid.UUID) (*models.UserRoleGrant, error) {
	var grant models.UserRoleGrant
	now := time.Now()
	err := r.db.GetDB().NewSelect().Model(&grant).
		Where("user_id = ?", userID).
		Where("role_id = ?", roleID).
		Where("(auto_expires_at IS NULL OR auto_expires_at > ?)", now).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &grant, nil
}

// ExtendRoleExpiration extends the expiration date of an existing role grant
// If the user doesn't have the role yet, it creates a new grant
func (r *UserRoleGrantService) ExtendRoleExpiration(ctx context.Context, userID string, roleID uuid.UUID, extensionDays int) (*models.UserRoleGrant, *time.Time, error) {
	// Get or create the grant
	grant, err := r.GetOrCreateUserRoleGrant(ctx, userID, roleID)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	var newExpirationDate time.Time

	if grant.AutoExpiresAt != nil && grant.AutoExpiresAt.After(now) {
		// User has active membership - extend from current expiration
		newExpirationDate = grant.AutoExpiresAt.AddDate(0, 0, extensionDays)
	} else {
		// User has no active membership or it's expired - extend from now
		newExpirationDate = now.AddDate(0, 0, extensionDays)
	}

	// Update the grant's expiration
	grant.AutoExpiresAt = &newExpirationDate
	grant.UpdatedAt = now

	if err := r.Update(ctx, grant); err != nil {
		return nil, nil, err
	}

	return grant, &newExpirationDate, nil
}

// CreatePermanentGrant creates a permanent role grant (for admin manual grants)
func (r *UserRoleGrantService) CreatePermanentGrant(ctx context.Context, userID string, roleID uuid.UUID) (*models.UserRoleGrant, error) {
	now := time.Now()
	grant := &models.UserRoleGrant{
		ID:            uuid.New(),
		UserID:        userID,
		RoleID:        roleID,
		AutoExpiresAt: nil, // Permanent - no expiration
		GrantedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := r.Create(ctx, grant); err != nil {
		return nil, err
	}

	return grant, nil
}

// GetUserRoleGrants retrieves role grants with filtering and pagination
func (r *UserRoleGrantService) GetUserRoleGrants(ctx context.Context, queryOpts query.QueryOptions[GetUserRoleGrantsFilters]) ([]*models.UserRoleGrant, int64, error) {
	var grants []*models.UserRoleGrant

	q := r.db.GetDB().NewSelect().Model(&grants).
		Relation("User").
		Relation("Role").
		Relation("Sources")

	// Apply filters
    if queryOpts.Filters.UserID != "" {
        q = q.Where("user_role_grants.user_id = ?", queryOpts.Filters.UserID)
    }
	if queryOpts.Filters.RoleID != uuid.Nil {
		q = q.Where("user_role_grants.role_id = ?", queryOpts.Filters.RoleID)
	}
	// No grant_source filter — sources are tracked via payments

	// Get total count
	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Apply pagination
	q = q.Limit(queryOpts.GetLimit()).Offset(queryOpts.GetOffset())

	// Apply ordering
	q = q.Order("user_role_grants.granted_at DESC")

	err = q.Scan(ctx)
	if err != nil {
		return nil, 0, err
	}

	return grants, int64(total), nil
}

// GetRoleSlugsForUser fetches all active role slugs for a user (used for JWT generation)
func (r *UserRoleGrantService) GetRoleSlugsForUser(ctx context.Context, userID string) ([]string, error) {
	var roleSlugs []string
	err := r.db.GetDB().NewSelect().
		TableExpr("user_role_grants AS urg").
		Column("r.slug").
		Join("JOIN roles r ON r.id = urg.role_id").
		Where("urg.user_id = ?", userID).
		Where("urg.auto_expires_at IS NULL OR urg.auto_expires_at > NOW()").
		Scan(ctx, &roleSlugs)

	return roleSlugs, err
}
