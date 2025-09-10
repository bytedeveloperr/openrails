//go:build integration

// Package tests contains integration tests for Mobius direct API subscription purchases.
//
// This test verifies the complete flow when a user purchases a subscription through
// Mobius using the direct API (CustomerVaultID method vs indirect PaymentToken method).
//
// The test ensures that:
// 1. The subscription is properly registered in the database
// 2. The purchase record is created when payment succeeds
// 3. The premium membership role is granted to the user
// 4. The premium membership auto-expiration date matches the plan duration
//
// To run this test:
//
//	go test -tags=integration ./tests/ -v -run TestMobiusDirectAPISubscription
//
// Prerequisites:
// - Docker daemon running (for testcontainers)
package tests

import (
	"context"
	"testing"
	"time"

	"github.com/doujins-org/doujins/config"
	"github.com/doujins-org/doujins/internal/database"
	"github.com/doujins-org/doujins/internal/database/models"
	"github.com/doujins-org/doujins/tests/mocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// MobiusDirectAPISubscriptionSuite tests the complete direct API subscription flow
type MobiusDirectAPISubscriptionSuite struct {
	suite.Suite
	containers *TestContainerSuite
	mockServer *mocks.MobiusMockServer

	// Test user and subscription data
	testUserEmail       string
	testUserID          string
	testCustomerVaultID string
	testPlanID          string
	testAmount          float64
	testCurrency        string
	expectedDuration    int // Duration in days
}

// SetupSuite initializes the test environment
func (suite *MobiusDirectAPISubscriptionSuite) SetupSuite() {
	// Create testcontainer environment
	suite.containers = NewTestContainerSuite(suite.T())

	// Initialize mock server
	suite.mockServer = mocks.NewMobiusMockServer()
	suite.mockServer.EnableWebhooks(suite.containers.ServerURL + "/api/v1/webhooks/mobius")

	// Initialize test data
	suite.testUserEmail = "direct-api-test@example.com"
	suite.testCustomerVaultID = "vault_test_direct_" + uuid.New().String()[:8]
	suite.testPlanID = "premium_monthly_direct_test"
	suite.testAmount = 9.99
	suite.testCurrency = "USD"
	suite.expectedDuration = 30 // 30 days for monthly plan

	// Create test user
	suite.createTestUser()

	// Create test price/product for the plan
	suite.createTestPriceAndProduct()
}

// TearDownSuite cleans up test resources
func (suite *MobiusDirectAPISubscriptionSuite) TearDownSuite() {
	if suite.mockServer != nil {
		suite.mockServer.Close()
	}
	if suite.containers != nil {
		suite.containers.Cleanup()
	}
}

// createTestUser creates a test user for subscription testing
func (suite *MobiusDirectAPISubscriptionSuite) createTestUser() {
	userManager := GetUserManager(suite.T(), suite.containers)
	user, err := userManager.CreateStandardTestUser(context.Background(), suite.testUserEmail)
	require.NoError(suite.T(), err, "Failed to create test user")

	suite.testUserID = user.User.ID
	suite.T().Logf("Created test user %s with ID %s", suite.testUserEmail, suite.testUserID)
}

// createTestPriceAndProduct creates test price and product entities using raw SQL
func (suite *MobiusDirectAPISubscriptionSuite) createTestPriceAndProduct() {
	// Get database connection
	hostDBUrl := suite.containers.GetDatabaseURL()
	dbConfig := &config.DBConfig{
		URL:     hostDBUrl,
		Dialect: "postgres",
		Schema:  "doujins",
	}
	db, err := database.NewDB(dbConfig)
	require.NoError(suite.T(), err, "Failed to connect to database")

	ctx := context.Background()

	// Generate UUIDs for our test entities
	roleID := uuid.New()
	productID := uuid.New()
	priceID := uuid.New()

	// Create a premium role for testing using Bun's NewRaw method
	_, err = db.GetDB().NewRaw(`
		INSERT INTO doujins.roles (id, name, slug, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, NOW(), NOW())
	`, roleID, "premium_test", "premium-test", "Premium test role").Exec(ctx)
	require.NoError(suite.T(), err, "Failed to create test role")

	// Create a product with the premium role using Bun's NewRaw method
	// Only insert columns that definitely exist to avoid schema compatibility issues
	_, err = db.GetDB().NewRaw(`
		INSERT INTO doujins.products (id, slug, display_name, description, role_id, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
	`, productID, "premium-subscription-test", "Premium Subscription Test", "Test premium subscription", roleID, true).Exec(ctx)
	require.NoError(suite.T(), err, "Failed to create test product")

	// Create a price for the product using Bun's NewRaw method
	_, err = db.GetDB().NewRaw(`
		INSERT INTO doujins.prices (id, product_id, display_name, amount, currency, billing_cycle_days, mobius_plan_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`, priceID, productID, "Premium Monthly Test Price", suite.testAmount, suite.testCurrency, suite.expectedDuration, suite.testPlanID).Exec(ctx)
	require.NoError(suite.T(), err, "Failed to create test price")

	suite.T().Logf("Created test product %s with price %s (Mobius plan: %s)",
		productID, priceID, suite.testPlanID)
}

// TestDirectAPISubscriptionFlow tests the complete direct API subscription purchase flow
func (suite *MobiusDirectAPISubscriptionSuite) TestDirectAPISubscriptionFlow() {
	suite.T().Run("Complete Direct API Subscription Purchase Flow", func(t *testing.T) {
		// Generate test IDs for this specific test
		subscriptionID := "sub_direct_api_" + uuid.New().String()[:8]
		transactionID := "txn_direct_api_" + uuid.New().String()[:8]

		t.Logf("Starting direct API subscription test for user %s", suite.testUserID)
		t.Logf("Test data: subscriptionID=%s, transactionID=%s, planID=%s",
			subscriptionID, transactionID, suite.testPlanID)

		// STEP 1: Trigger subscription creation webhook (recurring.subscription.add)
		// This simulates Mobius sending a webhook after successful subscription creation via direct API
		t.Log("Step 1: Triggering subscription creation webhook...")

		err := suite.mockServer.TriggerRecurringSubscriptionAdd(
			subscriptionID,
			suite.testUserEmail,
			"DirectAPI",
			"TestUser",
			suite.testPlanID,
			"9.99",
		)
		require.NoError(t, err, "Failed to trigger subscription add webhook")

		// Wait for webhook processing
		time.Sleep(2 * time.Second)

		// VERIFY: Subscription is registered
		t.Log("Verifying subscription was registered...")
		suite.verifySubscriptionRegistered(t, subscriptionID)

		// VERIFY: No roles granted yet (new logic separation)
		t.Log("Verifying no roles granted yet (separation of concerns)...")
		suite.verifyNoRolesGrantedYet(t)

		// STEP 2: Trigger transaction success webhook (transaction.sale.success)
		// This simulates the actual payment processing completion
		t.Log("Step 2: Triggering transaction success webhook...")

		err = suite.mockServer.TriggerTransactionSaleSuccess(
			transactionID,
			subscriptionID,
			"9.99",
			suite.testUserEmail,
			"DirectAPI",
			"TestUser",
		)
		require.NoError(t, err, "Failed to trigger transaction success webhook")

		// Wait for webhook processing
		time.Sleep(2 * time.Second)

		// VERIFY: Purchase is registered
		t.Log("Verifying purchase was registered...")
		purchase := suite.verifyPurchaseRegistered(t, transactionID)

		// VERIFY: Premium membership role is granted
		t.Log("Verifying premium membership role was granted...")
		userRoleGrant := suite.verifyPremiumRoleGranted(t)

		// VERIFY: Premium membership auto-expiration date is correct
		t.Log("Verifying premium membership expiration date...")
		suite.verifyRoleExpirationDate(t, userRoleGrant)

		// VERIFY: Purchase record is linked to role grant
		t.Log("Verifying purchase is linked to role grant...")
		suite.verifyPurchaseLinkedToRoleGrant(t, purchase, userRoleGrant)

		// STEP 3: Test webhook deduplication (replay same webhooks)
		t.Log("Step 3: Testing webhook deduplication...")

		// Replay subscription webhook - should be idempotent
		err = suite.mockServer.TriggerRecurringSubscriptionAdd(
			subscriptionID,
			suite.testUserEmail,
			"DirectAPI",
			"TestUser",
			suite.testPlanID,
			"9.99",
		)
		require.NoError(t, err, "Failed to replay subscription webhook")

		// Replay transaction webhook - should be idempotent
		err = suite.mockServer.TriggerTransactionSaleSuccess(
			transactionID,
			subscriptionID,
			"9.99",
			suite.testUserEmail,
			"DirectAPI",
			"TestUser",
		)
		require.NoError(t, err, "Failed to replay transaction webhook")

		time.Sleep(1 * time.Second)

		// VERIFY: No duplicate records created
		t.Log("Verifying no duplicate records were created...")
		suite.verifyNoDuplicateRecords(t, transactionID)

		t.Log("✅ Direct API subscription purchase flow completed successfully!")
	})
}

// verifySubscriptionRegistered verifies that a subscription was created in the database
func (suite *MobiusDirectAPISubscriptionSuite) verifySubscriptionRegistered(t *testing.T, subscriptionID string) {
	// Get database connection
	hostDBUrl := suite.containers.GetDatabaseURL()
	dbConfig := &config.DBConfig{
		URL:     hostDBUrl,
		Dialect: "postgres",
		Schema:  "doujins",
	}
	db, err := database.NewDB(dbConfig)
	require.NoError(t, err, "Failed to connect to database")

	ctx := context.Background()

	// Use raw SQL to avoid model compatibility issues
	var count int
	err = db.GetDB().NewRaw(`
		SELECT COUNT(*) FROM doujins.subscriptions 
		WHERE processor = ? AND processor_subscription_id = ? AND user_id = ?
	`, "mobius", subscriptionID, suite.testUserID).Scan(ctx, &count)

	require.NoError(t, err, "Should be able to query subscriptions")
	assert.Equal(t, 1, count, "Subscription should exist in database")

	// Query basic subscription details
	var subID string
	var userID string
	var status string
	err = db.GetDB().NewRaw(`
		SELECT id, user_id, status FROM doujins.subscriptions 
		WHERE processor = ? AND processor_subscription_id = ?
	`, "mobius", subscriptionID).Scan(ctx, &subID, &userID, &status)

	require.NoError(t, err, "Should be able to get subscription details")
	assert.Equal(t, suite.testUserID, userID, "Subscription should belong to test user")
	assert.Equal(t, "active", status, "Subscription should be active")

	t.Logf("✓ Subscription registered: ID=%s, UserID=%s, Status=%s",
		subID, userID, status)
}

// verifyNoRolesGrantedYet verifies that no roles were granted during subscription creation
func (suite *MobiusDirectAPISubscriptionSuite) verifyNoRolesGrantedYet(t *testing.T) {
	// Get database connection
	hostDBUrl := suite.containers.GetDatabaseURL()
	dbConfig := &config.DBConfig{
		URL:     hostDBUrl,
		Dialect: "postgres",
		Schema:  "doujins",
	}
	db, err := database.NewDB(dbConfig)
	require.NoError(t, err, "Failed to connect to database")

	ctx := context.Background()

	var userRoles []*models.UserRoleGrant
	err = db.GetDB().NewSelect().Model(&userRoles).
		Where("user_id = ?", suite.testUserID).
		Scan(ctx)

	require.NoError(t, err, "Should be able to query user roles")
	assert.Empty(t, userRoles, "No roles should be granted after subscription.add webhook (logic separation)")

	t.Log("✓ No roles granted yet (proper separation of concerns)")
}

// verifyPurchaseRegistered verifies that a purchase record was created
func (suite *MobiusDirectAPISubscriptionSuite) verifyPurchaseRegistered(t *testing.T, transactionID string) *models.Payment {
	// Get database connection
	hostDBUrl := suite.containers.GetDatabaseURL()
	dbConfig := &config.DBConfig{
		URL:     hostDBUrl,
		Dialect: "postgres",
		Schema:  "doujins",
	}
	db, err := database.NewDB(dbConfig)
	require.NoError(t, err, "Failed to connect to database")

	ctx := context.Background()

	var purchase models.Payment
	err = db.GetDB().NewSelect().Model(&purchase).
		Where("processor = ?", models.ProcessorMobius).
		Where("transaction_id = ?", transactionID).
		Scan(ctx)

	require.NoError(t, err, "Purchase should exist in database")
	assert.Equal(t, suite.testUserID, purchase.UserID, "Purchase should belong to test user")
	assert.Equal(t, suite.testAmount, purchase.Amount, "Purchase amount should match")
	assert.Equal(t, suite.testCurrency, purchase.Currency, "Purchase currency should match")
	assert.NotNil(t, purchase.PurchasedAt, "Purchase should have purchase timestamp")

	t.Logf("✓ Purchase registered: ID=%s, TransactionID=%s, Amount=%.2f %s",
		purchase.ID, purchase.TransactionID, purchase.Amount, purchase.Currency)

	return &purchase
}

// verifyPremiumRoleGranted verifies that the premium role was granted to the user
func (suite *MobiusDirectAPISubscriptionSuite) verifyPremiumRoleGranted(t *testing.T) *models.UserRoleGrant {
	// Get database connection
	hostDBUrl := suite.containers.GetDatabaseURL()
	dbConfig := &config.DBConfig{
		URL:     hostDBUrl,
		Dialect: "postgres",
		Schema:  "doujins",
	}
	db, err := database.NewDB(dbConfig)
	require.NoError(t, err, "Failed to connect to database")

	ctx := context.Background()

	var userRoles []*models.UserRoleGrant
	err = db.GetDB().NewSelect().Model(&userRoles).
		Where("user_id = ?", suite.testUserID).
		Where("auto_expires_at > ?", time.Now()).
		Scan(ctx)

	require.NoError(t, err, "Should be able to query user roles")
	require.NotEmpty(t, userRoles, "User should have at least one active role after purchase")

	// Should have exactly one active role grant
	assert.Len(t, userRoles, 1, "User should have exactly one active role grant")

	userRoleGrant := userRoles[0]
	assert.NotNil(t, userRoleGrant.AutoExpiresAt, "Role should have auto-expiration date")
	assert.True(t, userRoleGrant.AutoExpiresAt.After(time.Now()), "Role should not be expired")

	t.Logf("✓ Premium role granted: RoleGrantID=%s, ExpiresAt=%s",
		userRoleGrant.ID, userRoleGrant.AutoExpiresAt.Format("2006-01-02 15:04:05"))

	return userRoleGrant
}

// verifyRoleExpirationDate verifies the role expiration date matches expected duration
func (suite *MobiusDirectAPISubscriptionSuite) verifyRoleExpirationDate(t *testing.T, userRoleGrant *models.UserRoleGrant) {
	require.NotNil(t, userRoleGrant.AutoExpiresAt, "Role grant should have expiration date")

	// Calculate expected expiration (approximately expectedDuration days from now)
	now := time.Now()
	expectedExpiration := now.Add(time.Duration(suite.expectedDuration) * 24 * time.Hour)
	actualExpiration := *userRoleGrant.AutoExpiresAt

	// Allow for some variance (within 1 hour) due to processing time
	timeDiff := actualExpiration.Sub(expectedExpiration)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	assert.True(t, timeDiff < time.Hour,
		"Role expiration should be approximately %d days from now. Expected: %s, Actual: %s, Diff: %s",
		suite.expectedDuration, expectedExpiration.Format("2006-01-02 15:04:05"),
		actualExpiration.Format("2006-01-02 15:04:05"), timeDiff)

	// Verify it's in the future
	assert.True(t, actualExpiration.After(now), "Role expiration should be in the future")

	// Verify it's approximately the right duration
	daysUntilExpiration := actualExpiration.Sub(now).Hours() / 24
	assert.InDelta(t, float64(suite.expectedDuration), daysUntilExpiration, 1.0,
		"Role should expire in approximately %d days, got %.2f days",
		suite.expectedDuration, daysUntilExpiration)

	t.Logf("✓ Role expiration date correct: %.2f days from now (expected ~%d days)",
		daysUntilExpiration, suite.expectedDuration)
}

// verifyPurchaseLinkedToRoleGrant verifies the purchase record is linked to the role grant
func (suite *MobiusDirectAPISubscriptionSuite) verifyPurchaseLinkedToRoleGrant(t *testing.T, purchase *models.Payment, userRoleGrant *models.UserRoleGrant) {
	assert.NotNil(t, purchase.UserRoleGrantID, "Purchase should be linked to a role grant")
	assert.Equal(t, userRoleGrant.ID, *purchase.UserRoleGrantID, "Purchase should reference the correct role grant")
	assert.NotNil(t, purchase.ExtensionDays, "Purchase should record extension days")
	assert.Equal(t, suite.expectedDuration, *purchase.ExtensionDays, "Purchase should record correct extension days")

	t.Logf("✓ Purchase linked to role grant: PurchaseID=%s -> RoleGrantID=%s, ExtensionDays=%d",
		purchase.ID, *purchase.UserRoleGrantID, *purchase.ExtensionDays)
}

// verifyNoDuplicateRecords verifies webhook deduplication worked correctly
func (suite *MobiusDirectAPISubscriptionSuite) verifyNoDuplicateRecords(t *testing.T, transactionID string) {
	// Get database connection
	hostDBUrl := suite.containers.GetDatabaseURL()
	dbConfig := &config.DBConfig{
		URL:     hostDBUrl,
		Dialect: "postgres",
		Schema:  "doujins",
	}
	db, err := database.NewDB(dbConfig)
	require.NoError(t, err, "Failed to connect to database")

	ctx := context.Background()

	// Check for duplicate purchases
	var purchaseCount int
	purchaseCount, err = db.GetDB().NewSelect().Model((*models.Purchase)(nil)).
		Where("user_id = ?", suite.testUserID).
		Where("transaction_id = ?", transactionID).
		Count(ctx)

	require.NoError(t, err, "Should be able to count purchases")
	assert.Equal(t, 1, purchaseCount, "Should have exactly one purchase record after webhook replays")

	// Check for duplicate role grants
	var roleGrantCount int
	roleGrantCount, err = db.GetDB().NewSelect().Model((*models.UserRoleGrant)(nil)).
		Where("user_id = ?", suite.testUserID).
		Where("auto_expires_at > ?", time.Now()).
		Count(ctx)

	require.NoError(t, err, "Should be able to count role grants")
	assert.Equal(t, 1, roleGrantCount, "Should have exactly one active role grant after webhook replays")

	t.Log("✓ No duplicate records created (webhook deduplication working)")
}

// TestMobiusDirectAPISubscription runs the direct API subscription test suite
func TestMobiusDirectAPISubscription(t *testing.T) {
	suite.Run(t, new(MobiusDirectAPISubscriptionSuite))
}
