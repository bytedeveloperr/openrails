//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/open-rails/openrails/internal/db/models"
)

func TestAdminEntitlementAppendsAfterLatestEnd(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	userID := uuid.New().String()
	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	suite.SetMockClock(fixedNow)

	// Create a subscription-sourced entitlement window that ends in the future.
	subID := uuid.New()
	start := fixedNow
	subEnd := fixedNow.Add(30 * 24 * time.Hour)
	existing := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: "premium",
		StartAt:     start,
		EndAt:       &subEnd,
		SourceType:  models.EntitlementSourceSubscription,
		SourceID:    &subID,
		CreatedAt:   fixedNow,
		UpdatedAt:   fixedNow,
	}
	_, err := suite.BunDB.NewInsert().Model(existing).Exec(context.Background())
	require.NoError(t, err)

	body, err := json.Marshal(map[string]any{
		"entitlement": "premium",
		"days":        7,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/entitlements", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created models.Entitlement
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	require.Equal(t, models.EntitlementSourceAdmin, created.SourceType)
	require.NotNil(t, created.SourceID)
	require.Equal(t, subEnd, created.StartAt)
	require.NotNil(t, created.EndAt)
	require.Equal(t, subEnd.Add(7*24*time.Hour), *created.EndAt)
}
