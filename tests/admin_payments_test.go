//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/api"
)

// TestAdminPaymentsRequireAuth verifies that payment endpoints require admin JWT
func TestAdminPaymentsRequireAuth(t *testing.T) {
	suite, _ := setupAdminTestSuite(t)

	t.Run("GET payments returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("GET payment by ID returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments/"+uuid.New().String(), nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 403 with non-admin JWT", func(t *testing.T) {
		userID := uuid.New().String()
		userToken := CreateUserToken(t, userID)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 Forbidden for non-admin user")
	})
}

// TestAdminListPayments tests GET /v1/admin/payments
func TestAdminListPayments(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	// Seed test data
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID
	userID := uuid.New().String()

	// Create test subscription and payments
	sub := suite.CreateTestSubscription(userID, priceID, models.StatusActive)
	payment1 := suite.CreateTestPaymentWithOptions(PaymentOptions{
		UserID:         userID,
		PriceID:        priceID,
		SubscriptionID: &sub.ID,
		Processor:      models.ProcessorMobius,
		Amount:         999,
		PurchasedAt:    time.Now().Add(-24 * time.Hour),
	})
	_ = suite.CreateTestPaymentWithOptions(PaymentOptions{
		UserID:         userID,
		PriceID:        priceID,
		SubscriptionID: &sub.ID,
		Processor:      models.ProcessorMobius,
		Amount:         999,
		PurchasedAt:    time.Now(),
	})

	t.Run("returns paginated payments list with Stripe-like format", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments?limit=10", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Should have data array
		data, ok := response["data"].([]interface{})
		require.True(t, ok, "Should have data array")
		require.GreaterOrEqual(t, len(data), 2, "Should have at least 2 payments")

		// Verify Stripe-like payment format
		payment := data[0].(map[string]interface{})
		assert.True(t, strings.HasPrefix(payment["id"].(string), "pay_"), "ID should have pay_ prefix")
		assert.Equal(t, "payment", payment["object"], "Object should be 'payment'")
		assert.NotNil(t, payment["amount"], "Should have amount")
		assert.NotNil(t, payment["currency"], "Should have currency")
		assert.True(t, strings.HasPrefix(payment["user"].(string), "usr_"), "User should have usr_ prefix")
		assert.NotNil(t, payment["processor"], "Should have processor")
		assert.NotNil(t, payment["transaction_id"], "Should have transaction_id")
		assert.NotNil(t, payment["created"], "Should have created (unix timestamp)")
		assert.NotNil(t, payment["refunded"], "Should have refunded boolean")
		assert.NotNil(t, payment["amount_refunded"], "Should have amount_refunded")

		// Should have pagination fields
		assert.NotNil(t, response["total"], "Should have total")
		assert.NotNil(t, response["limit"], "Should have limit")
		assert.NotNil(t, response["offset"], "Should have offset")
	})

	t.Run("filters by user_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/payments?user_id=%s", userID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		require.Len(t, data, 2, "Should return exactly 2 payments for this user")

		// All payments should belong to the user (user field has usr_ prefix)
		expectedUser := api.FormatUserID(userID)
		for _, p := range data {
			payment := p.(map[string]interface{})
			assert.Equal(t, expectedUser, payment["user"], "Payment should belong to filtered user")
		}
	})

	t.Run("filters by processor", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments?processor=mobius", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		for _, p := range data {
			payment := p.(map[string]interface{})
			assert.Equal(t, "mobius", payment["processor"], "Payment should use mobius processor")
		}
	})

	t.Run("filters by amount range", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments?min_amount=500&max_amount=1500", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		for _, p := range data {
			payment := p.(map[string]interface{})
			amount := int64(payment["amount"].(float64))
			assert.GreaterOrEqual(t, amount, int64(500), "Amount should be >= min_amount")
			assert.LessOrEqual(t, amount, int64(1500), "Amount should be <= max_amount")
		}
	})

	t.Run("filters refunds only", func(t *testing.T) {
		// Create a refund
		_ = suite.CreateTestPaymentWithOptions(PaymentOptions{
			UserID:            userID,
			PriceID:           priceID,
			RefundedPaymentID: &payment1.ID,
			Processor:         models.ProcessorMobius,
			Amount:            -999, // Negative amount for refund
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments?refunds_only=true", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		require.GreaterOrEqual(t, len(data), 1, "Should have at least 1 refund")

		for _, p := range data {
			payment := p.(map[string]interface{})
			// Refunds have negative amounts
			amount := int64(payment["amount"].(float64))
			assert.Less(t, amount, int64(0), "Refund should have negative amount")
		}
	})

	t.Run("sorts by created descending", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/payments?user_id=%s&sort_by=created_at&sort_order=desc", userID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		require.GreaterOrEqual(t, len(data), 2)

		// Verify descending order: each payment's created should be >= the next one
		for i := 0; i < len(data)-1; i++ {
			p1 := data[i].(map[string]interface{})
			p2 := data[i+1].(map[string]interface{})
			t1 := int64(p1["created"].(float64))
			t2 := int64(p2["created"].(float64))
			assert.GreaterOrEqual(t, t1, t2, "Payments should be in descending order by created")
		}
	})

	t.Run("sorts by amount ascending", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments?sort_by=amount&sort_order=asc&limit=100", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		require.GreaterOrEqual(t, len(data), 2)

		// Verify ascending order by amount
		var prevAmount int64 = -1000000
		for _, p := range data {
			payment := p.(map[string]interface{})
			amount := int64(payment["amount"].(float64))
			assert.GreaterOrEqual(t, amount, prevAmount, "Amounts should be in ascending order")
			prevAmount = amount
		}
	})

	t.Run("filters by subscription_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/payments?subscription_id=%s", sub.ID.String()), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		require.GreaterOrEqual(t, len(data), 2, "Should have payments for this subscription")

		expectedSubID := api.FormatSubscriptionID(sub.ID)
		for _, p := range data {
			payment := p.(map[string]interface{})
			assert.Equal(t, expectedSubID, payment["subscription"], "Payment should belong to filtered subscription")
		}
	})
}

// TestAdminGetPayment tests GET /v1/admin/payments/:id
func TestAdminGetPayment(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	// Seed test data
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID
	userID := uuid.New().String()

	// Create test subscription and payment
	sub := suite.CreateTestSubscription(userID, priceID, models.StatusActive)
	payment := suite.CreateTestPaymentWithOptions(PaymentOptions{
		UserID:         userID,
		PriceID:        priceID,
		SubscriptionID: &sub.ID,
		Processor:      models.ProcessorMobius,
		Amount:         999,
	})

	t.Run("returns payment with Stripe-like format", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/payments/%s", payment.ID.String()), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify Stripe-like format
		assert.Equal(t, api.FormatPaymentID(payment.ID), response["id"], "Payment ID should have pay_ prefix")
		assert.Equal(t, "payment", response["object"], "Object should be 'payment'")
		assert.Equal(t, float64(999), response["amount"], "Amount should match")
		assert.Equal(t, "USD", response["currency"], "Currency should match")
		assert.Equal(t, api.FormatUserID(userID), response["user"], "User should have usr_ prefix")
		assert.Equal(t, "mobius", response["processor"], "Processor should match")
		assert.NotNil(t, response["subscription"], "Should include subscription ID")
		assert.Equal(t, false, response["refunded"], "Should not be refunded")
		assert.Equal(t, float64(0), response["amount_refunded"], "Amount refunded should be 0")

		// Should include expanded price
		assert.NotNil(t, response["price"], "Should include price details")
		price := response["price"].(map[string]interface{})
		assert.True(t, strings.HasPrefix(price["id"].(string), "price_"), "Price ID should have prefix")

		// Should have refunds list object (empty since no refunds created yet)
		refunds, ok := response["refunds"].(map[string]interface{})
		require.True(t, ok, "Should have refunds object")
		assert.Equal(t, "list", refunds["object"], "Refunds should be a list object")
		refundData := refunds["data"].([]interface{})
		assert.Empty(t, refundData, "Should have no refunds")
	})

	t.Run("returns payment with refunds and amount_refunded", func(t *testing.T) {
		// Create a refund for the payment
		refund := suite.CreateTestPaymentWithOptions(PaymentOptions{
			UserID:            userID,
			PriceID:           priceID,
			RefundedPaymentID: &payment.ID,
			Processor:         models.ProcessorMobius,
			Amount:            -500, // Partial refund (negative)
			TransactionID:     "refund-txn-" + uuid.New().String()[:8],
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/payments/%s", payment.ID.String()), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Should show partial refund status
		assert.Equal(t, false, response["refunded"], "Should not be fully refunded (partial)")
		assert.Equal(t, float64(500), response["amount_refunded"], "Amount refunded should be 500")

		// Should have refunds list with the refund
		refunds := response["refunds"].(map[string]interface{})
		assert.Equal(t, "list", refunds["object"])
		refundData := refunds["data"].([]interface{})
		require.Len(t, refundData, 1, "Should have 1 refund")

		refundObj := refundData[0].(map[string]interface{})
		assert.Equal(t, api.FormatPaymentID(refund.ID), refundObj["id"], "Refund ID should match")
		assert.Equal(t, float64(-500), refundObj["amount"], "Refund amount should be negative")
	})

	t.Run("returns 404 for non-existent payment", func(t *testing.T) {
		nonExistentID := uuid.New()

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/payments/%s", nonExistentID.String()), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 Not Found")
	})

	t.Run("returns 400 for invalid payment ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments/not-a-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})
}

// TestAdminPaymentsTransactionIDFilter tests filtering by transaction_id
func TestAdminPaymentsTransactionIDFilter(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	// Seed test data
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID
	userID := uuid.New().String()

	transactionID := "unique-txn-" + uuid.New().String()[:8]
	payment := suite.CreateTestPaymentWithOptions(PaymentOptions{
		UserID:        userID,
		PriceID:       priceID,
		Processor:     models.ProcessorMobius,
		Amount:        999,
		TransactionID: transactionID,
	})

	t.Run("finds payment by transaction_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/payments?transaction_id=%s", transactionID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		require.Len(t, data, 1, "Should find exactly 1 payment")

		foundPayment := data[0].(map[string]interface{})
		assert.Equal(t, api.FormatPaymentID(payment.ID), foundPayment["id"], "Should find the correct payment")
		assert.Equal(t, transactionID, foundPayment["transaction_id"], "Transaction ID should match")
	})

	t.Run("returns empty for non-existent transaction_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/payments?transaction_id=non-existent-txn", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Empty(t, data, "Should return empty array for non-existent transaction")
	})
}
