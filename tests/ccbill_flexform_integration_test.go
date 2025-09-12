//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	repo "github.com/doujins-org/doujins-billing/internal/db/repo"
	subscription "github.com/doujins-org/doujins-billing/internal/handlers"
)

// TestCCBillFlexFormIntegration tests the CCBill FlexForm URL generation endpoint
func TestCCBillFlexFormIntegration(t *testing.T) {
	if !*enableTestContainers {
		t.Skip("Testcontainers not enabled, skipping CCBill FlexForm integration tests")
	}

	testContainer, teardown := setupIntegrationTest(t)
	defer teardown()

	ctx := context.Background()
	testUser, testPrice := setupFlexFormTestData(t, testContainer, ctx)

	t.Run("GenerateFlexFormURL_Success", func(t *testing.T) {
		// In this repo, FlexForm URL is generated locally from config; no external mock required.

		// Create request payload
		requestPayload := subscription.GenerateFlexFormURLBodyParams{
			PriceID:   testPrice.ID.String(),
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Test Street",
			City:      "Testville",
			State:     "CA",
			ZipCode:   "12345",
			Country:   "US",
		}

		// Make authenticated request
		response := makeAuthenticatedRequest(t, testContainer, testUser, "POST", "/api/v1/subscriptions/ccbill/flexform-url", requestPayload)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// Parse response
		var flexFormResponse subscription.GenerateFlexFormURLResponse
		err := json.NewDecoder(response.Body).Decode(&flexFormResponse)
		require.NoError(t, err)

		// Verify response contains required fields
		assert.NotEmpty(t, flexFormResponse.IFrameURL, "Should have iframe_url")
		assert.NotEmpty(t, flexFormResponse.Width, "Should have width")
		assert.NotEmpty(t, flexFormResponse.Height, "Should have height")

		// Verify subscription was created in pending status
		subRepo := repo.NewSubscriptionRepo(testContainer.DB)
		subscriptions, _, err := subRepo.GetByUserID(ctx, testUser.ID)
		require.NoError(t, err)
		require.Len(t, subscriptions, 1, "Should have created a pending subscription")

		subscription := subscriptions[0]
		assert.Equal(t, models.StatusPending, subscription.Status)
		assert.Equal(t, testPrice.ID, subscription.PriceID)
		assert.Equal(t, models.ProcessorCCBill, subscription.Processor)
	})

	t.Run("GenerateFlexFormURL_InvalidPriceID", func(t *testing.T) {
		requestPayload := subscription.GenerateFlexFormURLBodyParams{
			PriceID:   "invalid-uuid",
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Test Street",
			City:      "Testville",
			State:     "CA",
			ZipCode:   "12345",
			Country:   "US",
		}

		response := makeAuthenticatedRequest(t, testContainer, testUser, "POST", "/api/v1/subscriptions/ccbill/flexform-url", requestPayload)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	})

	t.Run("GenerateFlexFormURL_PriceNotFound", func(t *testing.T) {
		requestPayload := subscription.GenerateFlexFormURLBodyParams{
			PriceID:   uuid.New().String(), // Non-existent price ID
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Test Street",
			City:      "Testville",
			State:     "CA",
			ZipCode:   "12345",
			Country:   "US",
		}

		response := makeAuthenticatedRequest(t, testContainer, testUser, "POST", "/api/v1/subscriptions/ccbill/flexform-url", requestPayload)
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})

	t.Run("GenerateFlexFormURL_MissingRequiredFields", func(t *testing.T) {
		requestPayload := subscription.GenerateFlexFormURLBodyParams{
			PriceID: testPrice.ID.String(),
			// Missing required fields
		}

		response := makeAuthenticatedRequest(t, testContainer, testUser, "POST", "/api/v1/subscriptions/ccbill/flexform-url", requestPayload)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	})

	t.Run("GenerateFlexFormURL_ExistingSubscription", func(t *testing.T) {
		// Create existing subscription
		existingSub := createTestSubscription(t, testContainer, ctx, testUser, testPrice)
		defer cleanupTestSubscription(t, testContainer, ctx, existingSub.ID)

		requestPayload := subscription.GenerateFlexFormURLBodyParams{
			PriceID:   testPrice.ID.String(),
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Test Street",
			City:      "Testville",
			State:     "CA",
			ZipCode:   "12345",
			Country:   "US",
		}

		// Should still generate FlexForm URL even with existing subscription
		response := makeAuthenticatedRequest(t, testContainer, testUser, "POST", "/api/v1/subscriptions/ccbill/flexform-url", requestPayload)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// Should return the existing subscription instead of creating new one
		subRepo := repo.NewSubscriptionRepo(testContainer.DB)
		subscriptions, _, err := subRepo.GetByUserID(ctx, testUser.ID)
		require.NoError(t, err)
		require.Len(t, subscriptions, 1, "Should still have only one subscription")
	})

	t.Run("GenerateFlexFormURL_Unauthenticated", func(t *testing.T) {
		requestPayload := subscription.GenerateFlexFormURLBodyParams{
			PriceID:   testPrice.ID.String(),
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Test Street",
			City:      "Testville",
			State:     "CA",
			ZipCode:   "12345",
			Country:   "US",
		}

		// Make request without authentication
		jsonData, _ := json.Marshal(requestPayload)
		request, _ := http.NewRequest("POST", testContainer.ServerURL+"/api/v1/subscriptions/ccbill/flexform-url", bytes.NewBuffer(jsonData))
		request.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		response, err := client.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
	})
}

// TestCCBillFlexFormValidation tests FlexForm parameter validation
func TestCCBillFlexFormValidation(t *testing.T) {
	if !*enableTestContainers {
		t.Skip("Testcontainers not enabled, skipping CCBill FlexForm validation tests")
	}

	testContainer, teardown := setupIntegrationTest(t)
	defer teardown()

	ctx := context.Background()
	testUser, testPrice := setupFlexFormTestData(t, testContainer, ctx)

	validationTests := []struct {
		name           string
		payload        subscription.GenerateFlexFormURLBodyParams
		expectedStatus int
		description    string
	}{
		{
			name: "ValidRequest",
			payload: subscription.GenerateFlexFormURLBodyParams{
				PriceID:   testPrice.ID.String(),
				FirstName: "John",
				LastName:  "Doe",
				Address1:  "123 Test Street",
				City:      "Testville",
				State:     "CA",
				ZipCode:   "12345",
				Country:   "US",
			},
			expectedStatus: http.StatusOK,
			description:    "Valid request should succeed",
		},
		{
			name: "InvalidCountryCode",
			payload: subscription.GenerateFlexFormURLBodyParams{
				PriceID:   testPrice.ID.String(),
				FirstName: "John",
				LastName:  "Doe",
				Address1:  "123 Test Street",
				City:      "Testville",
				State:     "CA",
				ZipCode:   "12345",
				Country:   "USA", // Should be 2-letter code
			},
			expectedStatus: http.StatusBadRequest,
			description:    "Invalid country code should be rejected",
		},
		{
			name: "FirstNameTooLong",
			payload: subscription.GenerateFlexFormURLBodyParams{
				PriceID:   testPrice.ID.String(),
				FirstName: "ThisIsAVeryLongFirstNameThatExceedsTheMaximumAllowedLengthOfOneHundredCharactersAndShouldBeRejected",
				LastName:  "Doe",
				Address1:  "123 Test Street",
				City:      "Testville",
				State:     "CA",
				ZipCode:   "12345",
				Country:   "US",
			},
			expectedStatus: http.StatusBadRequest,
			description:    "First name too long should be rejected",
		},
		{
			name: "EmptyRequiredFields",
			payload: subscription.GenerateFlexFormURLBodyParams{
				PriceID: testPrice.ID.String(),
				// All other required fields are empty
			},
			expectedStatus: http.StatusBadRequest,
			description:    "Empty required fields should be rejected",
		},
	}

	for _, tt := range validationTests {
		t.Run(tt.name, func(t *testing.T) {
			response := makeAuthenticatedRequest(t, testContainer, testUser, "POST", "/api/v1/subscriptions/ccbill/flexform-url", tt.payload)
			assert.Equal(t, tt.expectedStatus, response.StatusCode, tt.description)
		})
	}
}

type testAuthUser struct {
	ID        string
	AuthToken string
}

func setupFlexFormTestData(t *testing.T, testContainer *TestContainer, ctx context.Context) (*testAuthUser, *models.Price) {
	// Create test user in Casdoor shim
	admin := testContainer.CreateTestUser(ctx, "flexform-test@example.com")
	user := &testAuthUser{ID: admin.ID, AuthToken: "test-auth-token"}

	// Create test product and price
	productRepo := repo.NewProductRepo(testContainer.DB)
	product := &models.Product{
		ID:          uuid.New(),
		Name:        "Test Premium FlexForm",
		Description: "Test premium subscription for FlexForm testing",
	}
	err = productRepo.Create(ctx, product)
	require.NoError(t, err)

	priceRepo := repo.NewPriceRepo(testContainer.DB)
	billingCycleDays := 30
	price := &models.Price{
		ID:               uuid.New(),
		ProductID:        product.ID,
		Amount:           19.99,
		Currency:         "USD",
		CCBillPriceID:    "flexform-test-price-id",
		BillingCycleDays: &billingCycleDays,
	}
	err = priceRepo.Create(ctx, price)
	require.NoError(t, err)

	// Load the product relationship
	price.Product = product

	return user, price
}

func makeAuthenticatedRequest(t *testing.T, testContainer *TestContainer, user *testAuthUser, method, endpoint string, payload interface{}) *http.Response {
	var requestBody *bytes.Buffer
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		require.NoError(t, err)
		requestBody = bytes.NewBuffer(jsonData)
	} else {
		requestBody = bytes.NewBuffer([]byte{})
	}

	request, err := http.NewRequest(method, testContainer.ServerURL+endpoint, requestBody)
	require.NoError(t, err)

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", user.AuthToken))

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	require.NoError(t, err)

	return response
}

func cleanupTestSubscription(t *testing.T, testContainer *TestContainer, ctx context.Context, subscriptionID uuid.UUID) {
	subRepo := repo.NewSubscriptionRepo(testContainer.DB)
	err := subRepo.Delete(ctx, subscriptionID)
	if err != nil {
		t.Logf("Failed to cleanup test subscription %s: %v", subscriptionID, err)
	}
}
