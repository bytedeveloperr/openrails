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

	"github.com/doujins-org/doujins/internal/api/payment_methods"
	"github.com/doujins-org/doujins/internal/database"
	"github.com/doujins-org/doujins/internal/database/models"
	"github.com/doujins-org/doujins/internal/database/repo"
	"github.com/doujins-org/doujins/internal/integrations/mobius"
	userdto "github.com/doujins-org/doujins/internal/services/user"
	mobiusMock "github.com/doujins-org/doujins/tests/mocks"
)

// TestPaymentMethodIntegration tests the complete vault management API
func TestPaymentMethodIntegration(t *testing.T) {
	// Create testcontainer environment for this test
	testSuite := NewTestContainerSuite(t)
	defer testSuite.Cleanup()

	ctx := context.Background()
	testUser := setupPaymentMethodTestUser(t, testSuite, ctx)

	t.Run("CreatePaymentMethod_Success", func(t *testing.T) {
		// Setup Mobius mock server
		mockServer := mobiusMock.NewMobiusClientMock()
		mockServer.SetResponse("CreateCustomerVault", &mobius.CreateCustomerVaultResponse{
			CustomerVaultID: "vault_123456",
		})

		// Create request payload
		requestPayload := payment_methods.CreatePaymentMethodBodyParams{
			CCNumber:  "4111111111111111",
			CCExp:     "1225",
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Test Street",
			City:      "Testville",
			State:     "CA",
			Zip:       "12345",
			Country:   "US",
			Phone:     "5551234567",
			Email:     testUser.Email,
			Company:   "Test Corp",
			Address2:  "Suite 456",
		}

		// Make authenticated request
		response := makeAuthenticatedPaymentMethodRequest(t, testSuite, testUser, "POST", "/api/v1/user/payment-methods", requestPayload)
		assert.Equal(t, http.StatusCreated, response.StatusCode)

		// Parse response
		var createResponse payment_methods.CreatePaymentMethodResponse
		err := json.NewDecoder(response.Body).Decode(&createResponse)
		require.NoError(t, err)

		// Verify response contains required fields
		assert.NotEmpty(t, createResponse.PaymentMethod.ID, "Should have payment method ID")
		assert.Equal(t, "vault_123456", createResponse.PaymentMethod.VaultID, "Should have correct Mobius vault ID")
		assert.True(t, createResponse.PaymentMethod.IsActive, "Should be active by default")
		assert.Equal(t, "Payment method created successfully", createResponse.Message)

		// Verify payment method was created in database
		vaultRepo := repo.NewPaymentMethodRepo(testSuite.DB.(*database.DB))
		dbVault, err := vaultRepo.GetByID(ctx, createResponse.PaymentMethod.ID)
		require.NoError(t, err)
		assert.Equal(t, testUser.ID, dbVault.UserID)
		assert.Equal(t, "vault_123456", dbVault.VaultID)
		assert.True(t, dbVault.IsActive)
	})

	t.Run("ListPaymentMethods_Success", func(t *testing.T) {
		// Create test payment method first
		testVault := createTestPaymentMethod(t, testSuite, ctx, testUser)
		defer cleanupTestPaymentMethod(t, testSuite, ctx, testVault.ID)

		// List payment methods
		response := makeAuthenticatedPaymentMethodRequest(t, testSuite, testUser, "GET", "/api/v1/user/payment-methods", nil)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// Parse response
		var listResponse payment_methods.ListPaymentMethodsResponse
		err := json.NewDecoder(response.Body).Decode(&listResponse)
		require.NoError(t, err)

		// Verify response
		assert.GreaterOrEqual(t, len(listResponse.Items), 1, "Should have at least one payment method")
		assert.Greater(t, listResponse.TotalItems, int64(0), "Should have total items")

		// Find our test payment method in the list
		found := false
		for _, v := range listResponse.Items {
			if v.ID == testVault.ID {
				found = true
				assert.Equal(t, testVault.VaultID, v.VaultID)
				assert.Equal(t, testVault.IsActive, v.IsActive)
				break
			}
		}
		assert.True(t, found, "Should find the test payment method in the list")
	})

	t.Run("UpdatePaymentMethod_Success", func(t *testing.T) {
		// Setup Mobius mock
		mockServer := mobiusMock.NewMobiusClientMock()
		mockServer.SetResponse("UpdateCustomerVault", nil) // UpdateCustomerVault returns no response

		// Create test payment method first
		testVault := createTestPaymentMethod(t, testSuite, ctx, testUser)
		defer cleanupTestPaymentMethod(t, testSuite, ctx, testVault.ID)

		// Update request payload
		updatePayload := payment_methods.UpdatePaymentMethodBodyParams{
			FirstName: "Jane",
			LastName:  "Smith",
			Address1:  "456 Updated Street",
			City:      "New City",
		}

		// Make update request
		response := makeAuthenticatedPaymentMethodRequest(t, testSuite, testUser, "PUT", fmt.Sprintf("/api/v1/user/payment-methods/%s", testVault.ID), updatePayload)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// Parse response
		var updateResponse payment_methods.UpdatePaymentMethodResponse
		err := json.NewDecoder(response.Body).Decode(&updateResponse)
		require.NoError(t, err)

		assert.Equal(t, testVault.ID, updateResponse.PaymentMethod.ID)
		assert.Equal(t, "Payment method updated successfully", updateResponse.Message)
	})

	t.Run("ActivatePaymentMethod_Success", func(t *testing.T) {
		// Create test payment method first
		testVault := createTestPaymentMethod(t, testSuite, ctx, testUser)
		defer cleanupTestPaymentMethod(t, testSuite, ctx, testVault.ID)

		// Activate payment method
		response := makeAuthenticatedPaymentMethodRequest(t, testSuite, testUser, "POST", fmt.Sprintf("/api/v1/user/payment-methods/%s/activate", testVault.ID), nil)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// Parse response
		var activateResponse payment_methods.ActivatePaymentMethodResponse
		err := json.NewDecoder(response.Body).Decode(&activateResponse)
		require.NoError(t, err)

		assert.Equal(t, testVault.ID, activateResponse.PaymentMethod.ID)
		assert.True(t, activateResponse.PaymentMethod.IsActive)
		assert.Equal(t, "Payment method activated successfully", activateResponse.Message)
	})

	t.Run("DeletePaymentMethod_Success", func(t *testing.T) {
		// Setup Mobius mock
		mockServer := mobiusMock.NewMobiusClientMock()
		mockServer.SetResponse("DeleteCustomerVault", nil) // DeleteCustomerVault returns no response

		// Create test payment method first
		testVault := createTestPaymentMethod(t, testSuite, ctx, testUser)

		// Delete payment method
		response := makeAuthenticatedPaymentMethodRequest(t, testSuite, testUser, "DELETE", fmt.Sprintf("/api/v1/user/payment-methods/%s", testVault.ID), nil)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// Parse response
		var deleteResponse payment_methods.DeletePaymentMethodResponse
		err := json.NewDecoder(response.Body).Decode(&deleteResponse)
		require.NoError(t, err)

		assert.True(t, deleteResponse.Success)
		assert.Equal(t, "Payment method deleted successfully", deleteResponse.Message)

		// Verify payment method was deactivated in database
		vaultRepo := repo.NewPaymentMethodRepo(testSuite.DB.(*database.DB))
		dbVault, err := vaultRepo.GetByID(ctx, testVault.ID)
		require.NoError(t, err)
		assert.False(t, dbVault.IsActive, "Payment method should be deactivated")
	})

	t.Run("DeletePaymentMethod_WithActiveSubscription_Fails", func(t *testing.T) {
		// Create test payment method and subscription
		testVault := createTestPaymentMethod(t, testSuite, ctx, testUser)
		defer cleanupTestPaymentMethod(t, testSuite, ctx, testVault.ID)

		testSubscription := createTestSubscriptionWithPaymentMethod(t, testSuite, ctx, testUser, testVault)
		defer cleanupTestPaymentMethodSubscription(t, testSuite, ctx, testSubscription.ID)

		// Try to delete payment method (should fail)
		response := makeAuthenticatedPaymentMethodRequest(t, testSuite, testUser, "DELETE", fmt.Sprintf("/api/v1/user/payment-methods/%s", testVault.ID), nil)
		assert.Equal(t, http.StatusConflict, response.StatusCode)
	})

	t.Run("CreatePaymentMethod_InvalidData_Fails", func(t *testing.T) {
		// Invalid card number
		invalidPayload := payment_methods.CreatePaymentMethodBodyParams{
			CCNumber:  "invalid",
			CCExp:     "1225",
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Test Street",
			City:      "Testville",
			State:     "CA",
			Zip:       "12345",
			Country:   "US",
			Email:     testUser.Email,
		}

		response := makeAuthenticatedPaymentMethodRequest(t, testSuite, testUser, "POST", "/api/v1/user/payment-methods", invalidPayload)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	})

	t.Run("UpdatePaymentMethod_UserDoesNotOwnPaymentMethod_Fails", func(t *testing.T) {
		// Create another user
		otherUser := setupPaymentMethodTestUser(t, testSuite, ctx)

		// Create test payment method for original user
		testVault := createTestPaymentMethod(t, testSuite, ctx, testUser)
		defer cleanupTestPaymentMethod(t, testSuite, ctx, testVault.ID)

		// Try to update payment method as other user
		updatePayload := payment_methods.UpdatePaymentMethodBodyParams{
			FirstName: "Hacker",
		}

		response := makeAuthenticatedPaymentMethodRequest(t, testSuite, otherUser, "PUT", fmt.Sprintf("/api/v1/user/payment-methods/%s", testVault.ID), updatePayload)
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})

	t.Run("PaymentMethodOperations_Unauthenticated_Fails", func(t *testing.T) {
		// Test all endpoints without authentication
		endpoints := []struct {
			method   string
			endpoint string
			payload  interface{}
		}{
			{"GET", "/api/v1/user/payment-methods", nil},
			{"POST", "/api/v1/user/payment-methods", payment_methods.CreatePaymentMethodBodyParams{}},
			{"PUT", "/api/v1/user/payment-methods/" + uuid.New().String(), payment_methods.UpdatePaymentMethodBodyParams{}},
			{"DELETE", "/api/v1/user/payment-methods/" + uuid.New().String(), nil},
			{"POST", "/api/v1/user/payment-methods/" + uuid.New().String() + "/activate", nil},
		}

		for _, endpoint := range endpoints {
			var requestBody *bytes.Buffer
			if endpoint.payload != nil {
				jsonData, _ := json.Marshal(endpoint.payload)
				requestBody = bytes.NewBuffer(jsonData)
			} else {
				requestBody = bytes.NewBuffer([]byte{})
			}

			request, _ := http.NewRequest(endpoint.method, testSuite.ServerURL+endpoint.endpoint, requestBody)
			request.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 10 * time.Second}
			response, err := client.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "Endpoint %s %s should require authentication", endpoint.method, endpoint.endpoint)
		}
	})
}

func setupPaymentMethodTestUser(t *testing.T, testSuite *TestContainerSuite, ctx context.Context) *userdto.AdminUser {
	email := fmt.Sprintf("payment-method-test-%s@example.com", uuid.New().String()[:8])
	user := testSuite.CreateTestUser(ctx, email)
	return user
}

func createTestPaymentMethod(t *testing.T, testSuite *TestContainerSuite, ctx context.Context, user *userdto.AdminUser) *models.PaymentMethod {
	vaultRepo := repo.NewPaymentMethodRepo(testSuite.DB.(*database.DB))
	vault := &models.PaymentMethod{
		ID:                   uuid.New(),
		UserID:               user.ID,
		Processor:            models.ProcessorMobius,
		VaultID:              "test_vault_" + uuid.New().String()[:8],
		InitialTransactionID: "test_txn_" + uuid.New().String()[:8],
		IsActive:             true,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	err := vaultRepo.Create(ctx, vault)
	require.NoError(t, err)
	return vault
}

func createTestSubscriptionWithPaymentMethod(t *testing.T, testSuite *TestContainerSuite, ctx context.Context, user *userdto.AdminUser, vault *models.PaymentMethod) *models.Subscription {
	// Create test product and price first
	productRepo := repo.NewProductRepo(testSuite.DB.(*database.DB))
	product := &models.Product{
		ID:          uuid.New(),
		Slug:        "test-product-for-payment-method",
		DisplayName: "Test Product for Payment Method",
		Description: "Test product for payment method testing",
	}
	err := productRepo.Create(ctx, product)
	require.NoError(t, err)

	priceRepo := repo.NewPriceRepo(testSuite.DB.(*database.DB))
	billingCycleDays := 30
	price := &models.Price{
		ID:               uuid.New(),
		ProductID:        product.ID,
		Amount:           19.99,
		Currency:         "USD",
		BillingCycleDays: &billingCycleDays,
	}
	err = priceRepo.Create(ctx, price)
	require.NoError(t, err)

	// Create subscription linked to vault
	subRepo := repo.NewSubscriptionRepo(testSuite.DB.(*database.DB))
	now := time.Now()
	futureDate := now.Add(30 * 24 * time.Hour)

	subscription := &models.Subscription{
		ID:                      uuid.New(),
		UserID:                  user.ID,
		PriceID:                 price.ID,
		Status:                  models.StatusActive,
		Processor:               models.ProcessorMobius,
		ProcessorSubscriptionID: "test_sub_" + uuid.New().String()[:8],
		PaymentMethodID:         &vault.ID, // Link to payment method
		CurrentPeriodStartsAt:   &now,
		CurrentPeriodEndsAt:     &futureDate,
		StartedAt:               now,
	}

	err = subRepo.Create(ctx, subscription)
	require.NoError(t, err)
	return subscription
}

func cleanupTestPaymentMethod(t *testing.T, testSuite *TestContainerSuite, ctx context.Context, vaultID uuid.UUID) {
	vaultRepo := repo.NewPaymentMethodRepo(testSuite.DB.(*database.DB))
	err := vaultRepo.Delete(ctx, vaultID)
	if err != nil {
		t.Logf("Failed to cleanup test payment method %s: %v", vaultID, err)
	}
}

func cleanupTestPaymentMethodSubscription(t *testing.T, testSuite *TestContainerSuite, ctx context.Context, subscriptionID uuid.UUID) {
	subRepo := repo.NewSubscriptionRepo(testSuite.DB.(*database.DB))
	subscription, err := subRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		t.Logf("Failed to get subscription for cleanup %s: %v", subscriptionID, err)
		return
	}

	// Delete related price and product
	if subscription.Price != nil {
		priceRepo := repo.NewPriceRepo(testSuite.DB.(*database.DB))
		productRepo := repo.NewProductRepo(testSuite.DB.(*database.DB))

		if subscription.Price.Product != nil {
			productRepo.Delete(ctx, subscription.Price.Product.ID)
		}
		priceRepo.Delete(ctx, subscription.Price.ID)
	}

	// Delete subscription
	err = subRepo.Delete(ctx, subscriptionID)
	if err != nil {
		t.Logf("Failed to cleanup test subscription %s: %v", subscriptionID, err)
	}
}

func makeAuthenticatedPaymentMethodRequest(t *testing.T, testSuite *TestContainerSuite, user *userdto.AdminUser, method, endpoint string, payload interface{}) *http.Response {
	var requestBody *bytes.Buffer
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		require.NoError(t, err)
		requestBody = bytes.NewBuffer(jsonData)
	} else {
		requestBody = bytes.NewBuffer([]byte{})
	}

	request, err := http.NewRequest(method, testSuite.ServerURL+endpoint, requestBody)
	require.NoError(t, err)

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", "test-auth-token-"+user.ID[:8]))

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	require.NoError(t, err)

	return response
}
