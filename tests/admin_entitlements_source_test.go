//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
)

func TestAdminEntitlementGrantCreatesSourceRecord(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	userID := uuid.New().String()

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

	var ent models.Entitlement
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ent))

	require.Equal(t, userID, ent.UserID)
	require.Equal(t, "premium", ent.Entitlement)
	require.Equal(t, models.EntitlementSourceAdmin, ent.SourceType)
	require.NotNil(t, ent.SourceID)

	var grant models.AdminGrant
	require.NoError(t, suite.BunDB.NewSelect().Model(&grant).Where("id = ?", *ent.SourceID).Scan(req.Context()))
	require.Equal(t, userID, grant.UserID)
	require.Equal(t, "admin_entitlement", grant.Reason)
	require.Nil(t, grant.PriceID)
}

func TestRemovedAdminGrantRoutesReturn404(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	userID := uuid.New().String()

	for _, path := range []string{
		"/v1/admin/users/" + userID + "/grants",
		"/v1/admin/grants/" + uuid.New().String(),
		"/v1/admin/users/" + userID + "/mobius",
		"/v1/admin/users/" + userID + "/mobius/metrics",
	} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code, path)
	}
}
