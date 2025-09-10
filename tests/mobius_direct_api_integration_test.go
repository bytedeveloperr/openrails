//go:build integration

// Package tests contains integration tests for Mobius direct API operations.
//
// These tests verify the 3 core Mobius API operations work correctly:
// 1. CreateSubscriptionWithToken - Payment token flow subscription creation
// 2. AttemptManualRebill - Manual rebilling of stored payment methods/vaults
// 3. CancelSubscription - API-driven subscription cancellation
//
// To run these tests:
//
//	go test -tags=integration ./tests/ -v -run TestMobiusDirectAPI
//
// Prerequisites:
// - Docker daemon running (for testcontainers)
// - Mock Mobius API server integration
package tests

import (
	"context"
	"testing"
	"time"

	"github.com/doujins-org/doujins/internal/database/models"
	"github.com/doujins-org/doujins/internal/services/billing"
	"github.com/doujins-org/doujins/internal/services/mobius"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// MobiusDirectAPISuite tests direct Mobius API operations with testcontainers
type MobiusDirectAPISuite struct {
	suite.Suite
	containers *TestContainerSuite

	// Services
	mobiusService   *mobius.MobiusService
	mobiusAPIClient *mobius.MobiusAPIClient

	// Test data
	testUserID    string
	testPlanID    string
	testPriceID   uuid.UUID
	testProductID uuid.UUID
}

func (suite *MobiusDirectAPISuite) SetupSuite() {
	suite.containers = &TestContainerSuite{}
	suite.containers.SetupSuite()

	// Initialize Mobius API client in test mode
	suite.mobiusAPIClient = mobius.NewMobiusAPIClient(
		"test-api-key",
		"http://localhost:8080/mock/mobius", // Use mock server
		true,                                // test mode
	)

	// Initialize Mobius service
	billingService := billing.NewBillingEventService(suite.containers.ClickHouseDB, suite.containers.DB)
	suite.mobiusService = mobius.NewMobiusService(
		suite.mobiusAPIClient,
		suite.containers.DB,
		billingService,
	)
}

func (suite *MobiusDirectAPISuite) TearDownSuite() {
	suite.containers.TearDownSuite()
}

func (suite *MobiusDirectAPISuite) SetupTest() {
	suite.T().Log("Setting up test data for Mobius API operations")

	ctx := context.Background()

	// Create test user
	testUser := suite.containers.CreateTestUser(ctx, "mobius-api-test@example.com")
	suite.testUserID = testUser.ID

	// Create test product
	testProduct := &models.Product{
		ID:          uuid.New(),
		Slug:        "premium-membership-api-test",
		DisplayName: "Premium Membership (API Test)",
		Description: "Test premium membership for API testing",
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	suite.containers.CreateProduct(ctx, testProduct)
	suite.testProductID = testProduct.ID

	// Create test price with Mobius plan ID
	suite.testPlanID = "premium_api_test"
	testPrice := &models.Price{
		ID:               uuid.New(),
		ProductID:        testProduct.ID,
		DisplayName:      "Monthly Premium (API Test)",
		IsActive:         true,
		Amount:           29.99,
		Currency:         "USD",
		BillingCycleDays: &[]int{30}[0], // Monthly billing
		MobiusPlanID:     &suite.testPlanID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	suite.containers.CreatePrice(ctx, testPrice)
	suite.testPriceID = testPrice.ID

	suite.T().Log("Initialized Mobius API test data")
}

func (suite *MobiusDirectAPISuite) TearDownTest() {
	ctx := context.Background()
	suite.containers.CleanupTestData(ctx)
}

// TestCreateSubscriptionWithToken tests the payment token flow for subscription creation
func (suite *MobiusDirectAPISuite) TestCreateSubscriptionWithToken() {
	suite.T().Run("successful subscription creation with payment token", func(t *testing.T) {
		ctx := context.Background()

		// Mock successful Mobius API response with vault creation
		expectedVaultID := "vault_test_12345"
		expectedTransactionID := "txn_test_67890"

		// Set up mock server to return successful subscription creation
		// (This would be configured via the mock server - simplified for now)

		req := &mobius.CreateSubscriptionWithTokenRequest{
			UserID:       suite.testUserID,
			PlanID:       suite.testPlanID,
			PaymentToken: "token_test_payment_12345",
			StartDate:    nil, // Start immediately
		}

		// Create subscription via API
		response, err := suite.mobiusService.CreateSubscriptionWithToken(ctx, req)
		require.NoError(t, err, "Failed to create subscription with payment token")
		require.NotNil(t, response, "Response should not be nil")

		// Verify subscription was created in database
		assert.NotNil(t, response.Subscription, "Subscription should be created")
		assert.Equal(t, suite.testUserID, response.Subscription.UserID, "User ID should match")
		assert.Equal(t, suite.testPriceID, response.Subscription.PriceID, "Price ID should match")
		assert.Equal(t, models.ProcessorMobius, response.Subscription.Processor, "Processor should be Mobius")
		assert.Equal(t, models.StatusActive, response.Subscription.Status, "Status should be active")

		// Verify payment method was created if vault was returned
		if response.APIResponse.HasVault() {
			assert.NotNil(t, response.PaymentMethod, "Payment method should be created when vault is returned")
			assert.Equal(t, suite.testUserID, response.PaymentMethod.UserID, "Payment method user ID should match")
			assert.Equal(t, models.ProcessorMobius, response.PaymentMethod.Processor, "Payment method processor should be Mobius")
			assert.True(t, response.PaymentMethod.IsActive, "Payment method should be active")
		}

		// Verify API response
		assert.True(t, response.APIResponse.IsSuccessfulResponse(), "API response should indicate success")

		t.Logf("Successfully created subscription %s with payment token", response.Subscription.ID)
	})

	suite.T().Run("failed subscription creation with invalid payment token", func(t *testing.T) {
		ctx := context.Background()

		req := &mobius.CreateSubscriptionWithTokenRequest{
			UserID:       suite.testUserID,
			PlanID:       suite.testPlanID,
			PaymentToken: "invalid_token",
			StartDate:    nil,
		}

		// Mock failed Mobius API response
		// (This would be configured via the mock server)

		response, err := suite.mobiusService.CreateSubscriptionWithToken(ctx, req)

		// Should fail due to invalid payment token
		assert.Error(t, err, "Should fail with invalid payment token")
		assert.Nil(t, response, "Response should be nil on failure")
		assert.Contains(t, err.Error(), "Mobius subscription creation failed", "Error should mention Mobius failure")

		t.Logf("Correctly failed subscription creation with invalid token: %v", err)
	})
}

// TestAttemptManualRebill tests manual rebilling of stored payment methods
func (suite *MobiusDirectAPISuite) TestAttemptManualRebill() {
	ctx := context.Background()

	// First create a subscription with payment method
	suite.T().Run("setup subscription with payment method", func(t *testing.T) {
		// Create payment method
		paymentMethod := &models.PaymentMethod{
			ID:        uuid.New(),
			UserID:    suite.testUserID,
			Processor: models.ProcessorMobius,
			VaultID:   "vault_test_for_rebill",
			IsActive:  true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		suite.containers.CreatePaymentMethod(ctx, paymentMethod)

		// Create subscription linked to payment method
		subscription := &models.Subscription{
			ID:                      uuid.New(),
			UserID:                  suite.testUserID,
			PriceID:                 suite.testPriceID,
			Status:                  models.StatusActive,
			ProcessorSubscriptionID: "sub_test_for_rebill",
			Processor:               models.ProcessorMobius,
			PaymentMethodID:         &paymentMethod.ID,
			CreatedAt:               time.Now(),
			UpdatedAt:               time.Now(),
		}
		suite.containers.CreateSubscription(ctx, subscription)

		t.Logf("Created subscription %s with payment method for rebill testing", subscription.ID)
	})

	suite.T().Run("successful manual rebill", func(t *testing.T) {
		// Get the subscription we just created
		subscriptions, err := suite.containers.GetUserSubscriptions(ctx, suite.testUserID)
		require.NoError(t, err, "Failed to get user subscriptions")
		require.Len(t, subscriptions, 1, "Should have exactly one subscription")

		subscription := subscriptions[0]

		req := &mobius.AttemptManualRebillRequest{
			SubscriptionID: subscription.ID,
			Amount:         nil, // Use default amount from price
		}

		// Mock successful rebill response
		expectedTransactionID := "rebill_txn_12345"

		response, err := suite.mobiusService.AttemptManualRebill(ctx, req)
		require.NoError(t, err, "Failed to attempt manual rebill")
		require.NotNil(t, response, "Response should not be nil")

		// Verify rebill success
		assert.True(t, response.Success, "Rebill should succeed")
		assert.True(t, response.APIResponse.IsSuccessfulResponse(), "API response should indicate success")

		// Verify purchase record was created
		if response.Success {
			assert.NotNil(t, response.Purchase, "Purchase record should be created on successful rebill")
			assert.Equal(t, suite.testUserID, response.Purchase.UserID, "Purchase user ID should match")
			assert.Equal(t, subscription.PriceID, response.Purchase.PriceID, "Purchase price ID should match")
			assert.Equal(t, models.ProcessorMobius, response.Purchase.Processor, "Purchase processor should be Mobius")
			assert.Equal(t, 29.99, response.Purchase.Amount, "Purchase amount should match price")
		}

		t.Logf("Successfully completed manual rebill for subscription %s", subscription.ID)
	})

	suite.T().Run("failed manual rebill on inactive payment method", func(t *testing.T) {
		// Create subscription with inactive payment method
		inactivePaymentMethod := &models.PaymentMethod{
			ID:        uuid.New(),
			UserID:    suite.testUserID,
			Processor: models.ProcessorMobius,
			VaultID:   "vault_test_inactive",
			IsActive:  false, // Inactive payment method
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		suite.containers.CreatePaymentMethod(ctx, inactivePaymentMethod)

		inactiveSubscription := &models.Subscription{
			ID:                      uuid.New(),
			UserID:                  suite.testUserID,
			PriceID:                 suite.testPriceID,
			Status:                  models.StatusActive,
			ProcessorSubscriptionID: "sub_test_inactive",
			Processor:               models.ProcessorMobius,
			PaymentMethodID:         &inactivePaymentMethod.ID,
			CreatedAt:               time.Now(),
			UpdatedAt:               time.Now(),
		}
		suite.containers.CreateSubscription(ctx, inactiveSubscription)

		req := &mobius.AttemptManualRebillRequest{
			SubscriptionID: inactiveSubscription.ID,
			Amount:         nil,
		}

		// Should fail due to inactive payment method
		response, err := suite.mobiusService.AttemptManualRebill(ctx, req)
		assert.Error(t, err, "Should fail with inactive payment method")
		assert.Nil(t, response, "Response should be nil on failure")
		assert.Contains(t, err.Error(), "payment method is inactive", "Error should mention inactive payment method")

		t.Logf("Correctly failed manual rebill on inactive payment method: %v", err)
	})
}

// TestCancelSubscription tests API-driven subscription cancellation
func (suite *MobiusDirectAPISuite) TestCancelSubscription() {
	ctx := context.Background()

	suite.T().Run("successful subscription cancellation by user", func(t *testing.T) {
		// Create active subscription
		activeSubscription := &models.Subscription{
			ID:                      uuid.New(),
			UserID:                  suite.testUserID,
			PriceID:                 suite.testPriceID,
			Status:                  models.StatusActive,
			ProcessorSubscriptionID: "sub_test_cancel_success",
			Processor:               models.ProcessorMobius,
			CreatedAt:               time.Now(),
			UpdatedAt:               time.Now(),
		}
		suite.containers.CreateSubscription(ctx, activeSubscription)

		req := &mobius.CancelMobiusSubscriptionRequest{
			SubscriptionID: activeSubscription.ID,
			Reason:         "User requested cancellation",
			CancelledBy:    "user",
		}

		// Mock successful cancellation response
		response, err := suite.mobiusService.CancelSubscription(ctx, req)
		require.NoError(t, err, "Failed to cancel subscription")
		require.NotNil(t, response, "Response should not be nil")

		// Verify cancellation success
		assert.True(t, response.Success, "Cancellation should succeed")
		assert.True(t, response.APIResponse.IsSuccessfulResponse(), "API response should indicate success")

		// Verify subscription status was updated in database
		updatedSub, err := suite.containers.GetSubscriptionByID(ctx, activeSubscription.ID)
		require.NoError(t, err, "Failed to get updated subscription")
		assert.Equal(t, models.StatusCancelled, updatedSub.Status, "Subscription should be cancelled")
		assert.NotNil(t, updatedSub.CancelledAt, "CancelledAt should be set")
		assert.Equal(t, models.CancelTypeUser, *updatedSub.CancelType, "Cancel type should be user")
		assert.Equal(t, "User requested cancellation", *updatedSub.CancelFeedback, "Cancel feedback should match")

		t.Logf("Successfully cancelled subscription %s by user", activeSubscription.ID)
	})

	suite.T().Run("successful subscription cancellation by admin", func(t *testing.T) {
		// Create another active subscription
		adminCancelSub := &models.Subscription{
			ID:                      uuid.New(),
			UserID:                  suite.testUserID,
			PriceID:                 suite.testPriceID,
			Status:                  models.StatusActive,
			ProcessorSubscriptionID: "sub_test_admin_cancel",
			Processor:               models.ProcessorMobius,
			CreatedAt:               time.Now(),
			UpdatedAt:               time.Now(),
		}
		suite.containers.CreateSubscription(ctx, adminCancelSub)

		req := &mobius.CancelMobiusSubscriptionRequest{
			SubscriptionID: adminCancelSub.ID,
			Reason:         "Administrative cancellation due to policy violation",
			CancelledBy:    "admin",
		}

		response, err := suite.mobiusService.CancelSubscription(ctx, req)
		require.NoError(t, err, "Failed to cancel subscription")

		// Verify admin cancellation
		assert.True(t, response.Success, "Admin cancellation should succeed")

		updatedSub, err := suite.containers.GetSubscriptionByID(ctx, adminCancelSub.ID)
		require.NoError(t, err, "Failed to get updated subscription")
		assert.Equal(t, models.StatusCancelled, updatedSub.Status, "Subscription should be cancelled")
		assert.Equal(t, models.CancelTypeMerchant, *updatedSub.CancelType, "Cancel type should be merchant for admin cancellation")

		t.Logf("Successfully cancelled subscription %s by admin", adminCancelSub.ID)
	})

	suite.T().Run("idempotent cancellation of already cancelled subscription", func(t *testing.T) {
		// Create already cancelled subscription
		cancelledSub := &models.Subscription{
			ID:                      uuid.New(),
			UserID:                  suite.testUserID,
			PriceID:                 suite.testPriceID,
			Status:                  models.StatusCancelled,
			ProcessorSubscriptionID: "sub_test_already_cancelled",
			Processor:               models.ProcessorMobius,
			CancelledAt:             &[]time.Time{time.Now().Add(-24 * time.Hour)}[0], // Cancelled yesterday
			CancelType:              &[]models.CancelType{models.CancelTypeUser}[0],
			CreatedAt:               time.Now().Add(-48 * time.Hour),
			UpdatedAt:               time.Now().Add(-24 * time.Hour),
		}
		suite.containers.CreateSubscription(ctx, cancelledSub)

		req := &mobius.CancelMobiusSubscriptionRequest{
			SubscriptionID: cancelledSub.ID,
			Reason:         "Attempting to cancel already cancelled subscription",
			CancelledBy:    "user",
		}

		response, err := suite.mobiusService.CancelSubscription(ctx, req)
		require.NoError(t, err, "Should handle already cancelled subscription gracefully")

		// Verify idempotent behavior
		assert.True(t, response.Success, "Should succeed for already cancelled subscription")
		assert.Equal(t, "Subscription already cancelled", response.APIResponse.ResponseText, "Should indicate already cancelled")

		t.Logf("Correctly handled idempotent cancellation of already cancelled subscription %s", cancelledSub.ID)
	})

	suite.T().Run("failed cancellation due to API error", func(t *testing.T) {
		// Create subscription that will fail to cancel via API
		failCancelSub := &models.Subscription{
			ID:                      uuid.New(),
			UserID:                  suite.testUserID,
			PriceID:                 suite.testPriceID,
			Status:                  models.StatusActive,
			ProcessorSubscriptionID: "sub_test_api_fail",
			Processor:               models.ProcessorMobius,
			CreatedAt:               time.Now(),
			UpdatedAt:               time.Now(),
		}
		suite.containers.CreateSubscription(ctx, failCancelSub)

		req := &mobius.CancelMobiusSubscriptionRequest{
			SubscriptionID: failCancelSub.ID,
			Reason:         "Test API failure scenario",
			CancelledBy:    "user",
		}

		// Mock API failure response
		// (This would be configured via the mock server to return failure)

		response, err := suite.mobiusService.CancelSubscription(ctx, req)

		// Should complete but indicate API failure
		require.NoError(t, err, "Should not error on API failure - should return response with failure status")
		assert.False(t, response.Success, "Should indicate failure")
		assert.False(t, response.APIResponse.IsSuccessfulResponse(), "API response should indicate failure")

		// Verify subscription status was NOT updated due to API failure
		unchangedSub, err := suite.containers.GetSubscriptionByID(ctx, failCancelSub.ID)
		require.NoError(t, err, "Failed to get subscription")
		assert.Equal(t, models.StatusActive, unchangedSub.Status, "Subscription should remain active on API failure")
		assert.Nil(t, unchangedSub.CancelledAt, "CancelledAt should remain nil on API failure")

		t.Logf("Correctly handled API failure for subscription cancellation %s", failCancelSub.ID)
	})
}

// TestMobiusAPIErrorHandling tests various error scenarios
func (suite *MobiusDirectAPISuite) TestMobiusAPIErrorHandling() {
	ctx := context.Background()

	suite.T().Run("subscription creation with non-existent user", func(t *testing.T) {
		nonExistentUserID := uuid.New()

		req := &mobius.CreateSubscriptionWithTokenRequest{
			UserID:       nonExistentUserID,
			PlanID:       suite.testPlanID,
			PaymentToken: "token_test_payment",
		}

		response, err := suite.mobiusService.CreateSubscriptionWithToken(ctx, req)
		assert.Error(t, err, "Should fail with non-existent user")
		assert.Nil(t, response, "Response should be nil")
		assert.Contains(t, err.Error(), "failed to get user", "Error should mention user lookup failure")
	})

	suite.T().Run("subscription creation with non-existent plan", func(t *testing.T) {
		req := &mobius.CreateSubscriptionWithTokenRequest{
			UserID:       suite.testUserID,
			PlanID:       "non_existent_plan",
			PaymentToken: "token_test_payment",
		}

		response, err := suite.mobiusService.CreateSubscriptionWithToken(ctx, req)
		assert.Error(t, err, "Should fail with non-existent plan")
		assert.Nil(t, response, "Response should be nil")
		assert.Contains(t, err.Error(), "failed to get price for plan", "Error should mention plan lookup failure")
	})

	suite.T().Run("manual rebill with non-existent subscription", func(t *testing.T) {
		nonExistentSubID := uuid.New()

		req := &mobius.AttemptManualRebillRequest{
			SubscriptionID: nonExistentSubID,
		}

		response, err := suite.mobiusService.AttemptManualRebill(ctx, req)
		assert.Error(t, err, "Should fail with non-existent subscription")
		assert.Nil(t, response, "Response should be nil")
		assert.Contains(t, err.Error(), "failed to get subscription", "Error should mention subscription lookup failure")
	})

	suite.T().Run("cancel non-Mobius subscription", func(t *testing.T) {
		// Create CCBill subscription
		ccbillSub := &models.Subscription{
			ID:                      uuid.New(),
			UserID:                  suite.testUserID,
			PriceID:                 suite.testPriceID,
			Status:                  models.StatusActive,
			ProcessorSubscriptionID: "ccbill_sub_123",
			Processor:               models.ProcessorCCBill, // Different processor
			CreatedAt:               time.Now(),
			UpdatedAt:               time.Now(),
		}
		suite.containers.CreateSubscription(ctx, ccbillSub)

		req := &mobius.CancelMobiusSubscriptionRequest{
			SubscriptionID: ccbillSub.ID,
			Reason:         "Test wrong processor",
			CancelledBy:    "user",
		}

		response, err := suite.mobiusService.CancelSubscription(ctx, req)
		assert.Error(t, err, "Should fail when trying to cancel non-Mobius subscription")
		assert.Nil(t, response, "Response should be nil")
		assert.Contains(t, err.Error(), "subscription is not a Mobius subscription", "Error should mention wrong processor")
	})
}

// TestMobiusAPIIntegrationTestSuite runs all Mobius direct API integration tests
func TestMobiusDirectAPIIntegration(t *testing.T) {
	suite.Run(t, new(MobiusDirectAPISuite))
}
