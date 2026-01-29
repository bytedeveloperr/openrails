//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/open-rails/openrails/internal/db/models"
	"github.com/open-rails/openrails/internal/db/repo"
)

func TestEntitlementSoftDeleteExcludedFromIsEntitled(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	userID := uuid.New().String()
	entName := "soft_delete_test_entitlement"
	now := time.Now().UTC()

	// Insert an entitlement window that is currently active.
	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: entName,
		StartAt:     now.Add(-1 * time.Hour),
		EndAt:       nil, // active indefinitely
		SourceType:  models.EntitlementSourceAdmin,
		SourceID:    ptrUUID(uuid.New()),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := suite.BunDB.NewInsert().Model(ent).Exec(ctx)
	require.NoError(t, err)

	require.NotNil(t, suite.App)
	require.NotNil(t, suite.App.Runtime)
	require.NotNil(t, suite.App.Runtime.DB)
	r := repo.NewEntitlementRepo(suite.App.Runtime.DB)

	ok, err := r.IsEntitled(ctx, userID, entName, now)
	require.NoError(t, err)
	require.True(t, ok, "entitlement should be active before soft delete")

	// Soft-delete the row. The Entitlement model uses bun's soft_delete tag, so this should set deleted_at.
	_, err = suite.BunDB.NewDelete().Model(ent).WherePK().Exec(ctx)
	require.NoError(t, err)

	ok, err = r.IsEntitled(ctx, userID, entName, now)
	require.NoError(t, err)
	require.False(t, ok, "soft-deleted entitlement should not be considered active")
}

func ptrUUID(v uuid.UUID) *uuid.UUID { return &v }
