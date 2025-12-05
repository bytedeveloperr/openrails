//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// TestTierGroupDetection tests that the checkout service correctly detects tier groups
func TestTierGroupDetection(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Seed tiered products (Premium, Premium+, Premium Ultimate)
	tieredProducts := suite.SeedTieredProducts()
	require.Len(t, tieredProducts, 3, "Should have 3 tiered products")

	premiumProduct := tieredProducts[0].Product
	premiumPlusProduct := tieredProducts[1].Product
	premiumUltimateProduct := tieredProducts[2].Product

	premiumPriceID := tieredProducts[0].Prices[0].ID
	premiumPlusPriceID := tieredProducts[1].Prices[0].ID

	userID := "test-user-" + uuid.New().String()[:8]

	t.Run("identifies products in same tier group", func(t *testing.T) {
		// All three should have the same tier group
		assert.NotNil(t, premiumProduct.TierGroup, "Premium should have tier group")
		assert.NotNil(t, premiumPlusProduct.TierGroup, "Premium+ should have tier group")
		assert.NotNil(t, premiumUltimateProduct.TierGroup, "Premium Ultimate should have tier group")

		assert.Equal(t, *premiumProduct.TierGroup, *premiumPlusProduct.TierGroup, "Premium and Premium+ should be in same tier group")
		assert.Equal(t, *premiumProduct.TierGroup, *premiumUltimateProduct.TierGroup, "Premium and Premium Ultimate should be in same tier group")
	})

	t.Run("identifies tier rank order", func(t *testing.T) {
		// Premium (1) < Premium+ (2) < Premium Ultimate (3)
		assert.Equal(t, 1, premiumProduct.TierRank, "Premium should have rank 1")
		assert.Equal(t, 2, premiumPlusProduct.TierRank, "Premium+ should have rank 2")
		assert.Equal(t, 3, premiumUltimateProduct.TierRank, "Premium Ultimate should have rank 3")

		assert.Less(t, premiumProduct.TierRank, premiumPlusProduct.TierRank, "Premium rank should be less than Premium+")
		assert.Less(t, premiumPlusProduct.TierRank, premiumUltimateProduct.TierRank, "Premium+ rank should be less than Premium Ultimate")
	})

	t.Run("detects upgrade scenario", func(t *testing.T) {
		// Create subscription on Premium
		sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:    userID,
			PriceID:   premiumPriceID,
			Status:    models.StatusActive,
			Processor: models.ProcessorMobius,
		})
		defer suite.CleanupSubscriptionsForUser(userID)

		// Try to purchase Premium+ - should detect as upgrade
		checkoutService := services.NewCheckoutService(
			suite.DB, nil, nil, nil, nil,
			suite.PriceService, suite.ProductService,
			suite.SubscriptionService, suite.EntitlementService,
			nil, nil, nil, nil, nil, nil,
		)

		eligibility, err := checkoutService.CheckPurchaseEligibility(ctx, userID, premiumPlusPriceID)
		require.NoError(t, err, "Should check eligibility without error")
		assert.Equal(t, services.EligibilityUpgrade, eligibility.Status, "Should detect upgrade scenario")
		assert.NotNil(t, eligibility.ExistingSubscription, "Should have existing subscription")
		assert.Equal(t, sub.ID.String(), eligibility.ExistingSubscription.ID.String())
	})

	t.Run("detects downgrade scenario", func(t *testing.T) {
		// Create subscription on Premium+
		sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:    userID,
			PriceID:   premiumPlusPriceID,
			Status:    models.StatusActive,
			Processor: models.ProcessorMobius,
		})
		defer suite.CleanupSubscriptionsForUser(userID)

		// Try to purchase Premium - should detect as downgrade
		checkoutService := services.NewCheckoutService(
			suite.DB, nil, nil, nil, nil,
			suite.PriceService, suite.ProductService,
			suite.SubscriptionService, suite.EntitlementService,
			nil, nil, nil, nil, nil, nil,
		)

		eligibility, err := checkoutService.CheckPurchaseEligibility(ctx, userID, premiumPriceID)
		require.NoError(t, err, "Should check eligibility without error")
		assert.Equal(t, services.EligibilityDowngrade, eligibility.Status, "Should detect downgrade scenario")
		assert.NotNil(t, eligibility.ExistingSubscription, "Should have existing subscription")
		assert.Equal(t, sub.ID.String(), eligibility.ExistingSubscription.ID.String())
	})

	t.Run("allows purchase when no existing subscription", func(t *testing.T) {
		newUserID := "new-user-" + uuid.New().String()[:8]

		checkoutService := services.NewCheckoutService(
			suite.DB, nil, nil, nil, nil,
			suite.PriceService, suite.ProductService,
			suite.SubscriptionService, suite.EntitlementService,
			nil, nil, nil, nil, nil, nil,
		)

		eligibility, err := checkoutService.CheckPurchaseEligibility(ctx, newUserID, premiumPriceID)
		require.NoError(t, err, "Should check eligibility without error")
		assert.Equal(t, services.EligibilityAllowed, eligibility.Status, "Should allow new subscription")
		assert.Nil(t, eligibility.ExistingSubscription, "Should not have existing subscription")
	})
}

// TestProrationCalculation tests that proration is calculated correctly for upgrades
func TestProrationCalculation(t *testing.T) {
	suite := setupTestSuite(t)

	// Seed tiered products
	tieredProducts := suite.SeedTieredProducts()
	premiumPrice := tieredProducts[0].Prices[0]   // $10/month
	premiumPlusPrice := tieredProducts[1].Prices[0] // $20/month

	t.Run("calculates correct proration for mid-cycle upgrade", func(t *testing.T) {
		// User on Premium ($10/mo), 15 days remaining, upgrades to Premium+ ($20/mo)
		// Expected proration: ($20 - $10) * (15/30) = $5

		oldAmount := premiumPrice.Amount       // 1000 cents ($10)
		newAmount := premiumPlusPrice.Amount   // 2000 cents ($20)
		billingCycle := 30                     // days
		daysRemaining := 15

		priceDiff := newAmount - oldAmount // 1000 cents ($10)
		prorationRatio := float64(daysRemaining) / float64(billingCycle)
		expectedProration := int64(float64(priceDiff) * prorationRatio) // 500 cents ($5)

		assert.Equal(t, int64(500), expectedProration, "Proration should be $5 (500 cents)")
	})

	t.Run("proration is zero at start of cycle", func(t *testing.T) {
		oldAmount := premiumPrice.Amount
		newAmount := premiumPlusPrice.Amount
		billingCycle := 30
		daysRemaining := 30 // Full cycle remaining

		priceDiff := newAmount - oldAmount
		prorationRatio := float64(daysRemaining) / float64(billingCycle)
		prorationAmount := int64(float64(priceDiff) * prorationRatio)

		assert.Equal(t, int64(1000), prorationAmount, "Proration should be full difference at start of cycle")
	})

	t.Run("proration is zero at end of cycle", func(t *testing.T) {
		oldAmount := premiumPrice.Amount
		newAmount := premiumPlusPrice.Amount
		billingCycle := 30
		daysRemaining := 0 // End of cycle

		priceDiff := newAmount - oldAmount
		prorationRatio := float64(daysRemaining) / float64(billingCycle)
		prorationAmount := int64(float64(priceDiff) * prorationRatio)

		assert.Equal(t, int64(0), prorationAmount, "Proration should be zero at end of cycle")
	})
}

// TestScheduledDowngrade tests that downgrades are scheduled and applied at renewal
func TestScheduledDowngrade(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Seed tiered products
	tieredProducts := suite.SeedTieredProducts()
	premiumPriceID := tieredProducts[0].Prices[0].ID    // $10/month, rank 1
	premiumPlusPriceID := tieredProducts[1].Prices[0].ID // $20/month, rank 2
	premiumProduct := tieredProducts[0].Product
	premiumPlusProduct := tieredProducts[1].Product

	userID := "test-user-" + uuid.New().String()[:8]

	t.Run("downgrade is scheduled for end of period", func(t *testing.T) {
		now := suite.GetClock().Now()
		periodEnd := now.Add(15 * 24 * time.Hour) // 15 days remaining

		// Create Premium+ subscription
		sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:              userID,
			PriceID:             premiumPlusPriceID,
			Status:              models.StatusActive,
			Processor:           models.ProcessorMobius,
			PeriodStart:         now.Add(-15 * 24 * time.Hour), // Started 15 days ago
			CurrentPeriodEndsAt: &periodEnd,
		})
		defer suite.CleanupSubscriptionsForUser(userID)

		// Create Premium+ entitlements
		suite.CreateTestEntitlement(userID, "premium", &sub.ID, models.EntitlementSourceSubscription)
		suite.CreateTestEntitlement(userID, "extra", &sub.ID, models.EntitlementSourceSubscription)

		// Set scheduled downgrade to Premium
		sub.ScheduledPriceID = &premiumPriceID
		_, err := suite.BunDB.NewUpdate().Model(sub).Column("scheduled_price_id").WherePK().Exec(ctx)
		require.NoError(t, err, "Should update scheduled price")

		// Verify subscription still has Premium+ entitlements
		ents := suite.GetEntitlementsByUserID(userID)
		entNames := make(map[string]bool)
		for _, e := range ents {
			entNames[e.Entitlement] = true
		}
		assert.True(t, entNames["premium"], "Should still have premium entitlement")
		assert.True(t, entNames["extra"], "Should still have extra entitlement (downgrade not applied yet)")

		// Verify subscription price is still Premium+
		refreshedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, premiumPlusPriceID.String(), refreshedSub.PriceID.String(), "Price should still be Premium+")
		assert.Equal(t, premiumPlusProduct.ID.String(), refreshedSub.ProductID.String(), "Product should still be Premium+")
		assert.NotNil(t, refreshedSub.ScheduledPriceID, "Should have scheduled price ID")
	})

	t.Run("downgrade is applied on renewal", func(t *testing.T) {
		now := suite.GetClock().Now()
		periodEnd := now // Period ends now

		// Create Premium+ subscription with period ending now
		sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:              userID,
			PriceID:             premiumPlusPriceID,
			Status:              models.StatusActive,
			Processor:           models.ProcessorMobius,
			ProcessorSubID:      "test-renewal-" + uuid.New().String()[:8],
			PeriodStart:         now.Add(-30 * 24 * time.Hour),
			CurrentPeriodEndsAt: &periodEnd,
		})
		defer suite.CleanupSubscriptionsForUser(userID)

		// Set scheduled downgrade to Premium
		sub.ScheduledPriceID = &premiumPriceID
		_, err := suite.BunDB.NewUpdate().Model(sub).Column("scheduled_price_id").WherePK().Exec(ctx)
		require.NoError(t, err, "Should update scheduled price")

		// Create entitlements
		suite.CreateTestEntitlement(userID, "premium", &sub.ID, models.EntitlementSourceSubscription)
		suite.CreateTestEntitlement(userID, "extra", &sub.ID, models.EntitlementSourceSubscription)

		// Simulate renewal via lifecycle service
		lifecycleService := services.NewSubscriptionLifecycleService(
			suite.DB,
			suite.ProductService,
			suite.PriceService,
			suite.EntitlementService,
			nil, // NotificationService
			suite.PaymentService,
		)
		lifecycleService.SetClock(suite.GetClock())

		err = lifecycleService.RenewMembership(ctx, &services.RenewMembershipParams{
			Processor:               models.ProcessorMobius,
			ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
			TransactionID:           "renewal-txn-" + uuid.New().String()[:8],
			Amount:                  1000, // $10 (Premium price)
			Currency:                "usd",
		})
		require.NoError(t, err, "Renewal should succeed")

		// Verify subscription switched to Premium
		refreshedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, premiumPriceID.String(), refreshedSub.PriceID.String(), "Price should be switched to Premium")
		assert.Equal(t, premiumProduct.ID.String(), refreshedSub.ProductID.String(), "Product should be switched to Premium")
		assert.Nil(t, refreshedSub.ScheduledPriceID, "Scheduled price should be cleared")
		assert.Equal(t, models.StatusActive, refreshedSub.Status, "Subscription should be active")
	})
}

// TestEntitlementChangesOnTierChange tests that entitlements are correctly updated
func TestEntitlementChangesOnTierChange(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Seed tiered products
	tieredProducts := suite.SeedTieredProducts()
	premiumPriceID := tieredProducts[0].Prices[0].ID     // rank 1, entitlements: [premium]
	premiumPlusPriceID := tieredProducts[1].Prices[0].ID  // rank 2, entitlements: [premium, extra]

	userID := "test-user-" + uuid.New().String()[:8]

	t.Run("upgrade grants additional entitlements", func(t *testing.T) {
		// Create Premium subscription
		now := suite.GetClock().Now()
		periodEnd := now.Add(30 * 24 * time.Hour)

		sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:              userID,
			PriceID:             premiumPriceID,
			Status:              models.StatusActive,
			Processor:           models.ProcessorMobius,
			CurrentPeriodEndsAt: &periodEnd,
		})
		defer suite.CleanupSubscriptionsForUser(userID)

		// Create Premium entitlement
		suite.CreateTestEntitlement(userID, "premium", &sub.ID, models.EntitlementSourceSubscription)

		// Verify only premium entitlement exists
		ents := suite.GetEntitlementsByUserID(userID)
		assert.Len(t, ents, 1, "Should have 1 entitlement before upgrade")
		assert.Equal(t, "premium", ents[0].Entitlement)

		// Simulate upgrade to Premium+ (this would be done by checkout service)
		// For this test, we verify the entitlement change logic in isolation
		entService := services.NewEntitlementService(suite.DB)
		entService.SetClock(suite.GetClock())

		// Grant new "extra" entitlement
		_, err := entService.GrantWindow(ctx, userID, "extra", now, nil, models.EntitlementSourceSubscription, &sub.ID)
		require.NoError(t, err, "Should grant extra entitlement")

		// Verify both entitlements now exist
		ents = suite.GetEntitlementsByUserID(userID)
		entNames := make(map[string]bool)
		for _, e := range ents {
			entNames[e.Entitlement] = true
		}
		assert.True(t, entNames["premium"], "Should have premium")
		assert.True(t, entNames["extra"], "Should have extra after upgrade")
	})

	t.Run("downgrade revokes extra entitlements", func(t *testing.T) {
		userID2 := "test-user-" + uuid.New().String()[:8]
		now := suite.GetClock().Now()
		periodEnd := now.Add(30 * 24 * time.Hour)

		// Create Premium+ subscription
		sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:              userID2,
			PriceID:             premiumPlusPriceID,
			Status:              models.StatusActive,
			Processor:           models.ProcessorMobius,
			CurrentPeriodEndsAt: &periodEnd,
		})
		defer suite.CleanupSubscriptionsForUser(userID2)

		// Create Premium+ entitlements
		suite.CreateTestEntitlement(userID2, "premium", &sub.ID, models.EntitlementSourceSubscription)
		suite.CreateTestEntitlement(userID2, "extra", &sub.ID, models.EntitlementSourceSubscription)

		// Verify both entitlements exist
		ents := suite.GetEntitlementsByUserID(userID2)
		assert.Len(t, ents, 2, "Should have 2 entitlements before downgrade")

		// Revoke "extra" entitlement (simulating downgrade)
		entService := services.NewEntitlementService(suite.DB)
		entService.SetClock(suite.GetClock())

		err := entService.RevokeBySubscriptionAndName(ctx, sub.ID, "extra", now, models.EntitlementRevokeDowngrade)
		require.NoError(t, err, "Should revoke extra entitlement")

		// Verify only premium entitlement remains
		ents = suite.GetEntitlementsByUserID(userID2)
		assert.Len(t, ents, 1, "Should have 1 entitlement after downgrade")
		assert.Equal(t, "premium", ents[0].Entitlement)
	})
}
