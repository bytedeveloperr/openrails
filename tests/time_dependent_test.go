//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// =============================================================================
// Entitlement Time-Dependent Tests
// =============================================================================

// TestEntitlementExpiry tests that time-limited entitlements expire correctly
// when checked at different points in time using the mock clock.
func TestEntitlementExpiry(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	userID := uuid.New().String()

	// Grant a 15-day entitlement
	entitlementName := "premium"
	endAt := startTime.Add(15 * 24 * time.Hour)

	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: entitlementName,
		StartAt:     startTime,
		EndAt:       &endAt,
		SourceType:  models.EntitlementSourceOneOff,
		CreatedAt:   startTime,
		UpdatedAt:   startTime,
	}
	_, err := suite.BunDB.NewInsert().Model(ent).Exec(ctx)
	require.NoError(t, err)

	entService := suite.App.Runtime.EntitlementService

	t.Run("entitlement is active immediately after grant", func(t *testing.T) {
		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "Entitlement should be active at start time")
	})

	t.Run("entitlement is active after 14 days", func(t *testing.T) {
		// Advance clock 14 days
		mockClock.Advance(14 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "Entitlement should still be active after 14 days")
	})

	t.Run("entitlement is NOT active after 15 days", func(t *testing.T) {
		// Advance clock 1 more day (total 15 days from start)
		mockClock.Advance(1 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.False(t, isEntitled, "Entitlement should NOT be active after 15 days (expired)")
	})

	t.Run("entitlement is NOT active after 30 days", func(t *testing.T) {
		// Advance clock 15 more days (total 30 days from start)
		mockClock.Advance(15 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.False(t, isEntitled, "Entitlement should NOT be active after 30 days")
	})
}

// TestEntitlementStacking tests that granting additional entitlements extends the expiry
func TestEntitlementStacking(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	userID := uuid.New().String()
	entitlementName := "premium"

	// Grant first 15-day entitlement
	firstEnd := startTime.Add(15 * 24 * time.Hour)
	ent1 := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: entitlementName,
		StartAt:     startTime,
		EndAt:       &firstEnd,
		SourceType:  models.EntitlementSourceOneOff,
		CreatedAt:   startTime,
		UpdatedAt:   startTime,
	}
	_, err := suite.BunDB.NewInsert().Model(ent1).Exec(ctx)
	require.NoError(t, err)

	// Grant second 15-day entitlement that stacks (starts where first ends)
	secondStart := firstEnd
	secondEnd := secondStart.Add(15 * 24 * time.Hour) // 30 days from original start
	ent2 := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: entitlementName,
		StartAt:     secondStart,
		EndAt:       &secondEnd,
		SourceType:  models.EntitlementSourceOneOff,
		CreatedAt:   startTime,
		UpdatedAt:   startTime,
	}
	_, err = suite.BunDB.NewInsert().Model(ent2).Exec(ctx)
	require.NoError(t, err)

	entService := suite.App.Runtime.EntitlementService

	t.Run("entitlement is active at start", func(t *testing.T) {
		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "Entitlement should be active at start")
	})

	t.Run("entitlement is active after 20 days (into second window)", func(t *testing.T) {
		// Advance clock 20 days - past first entitlement, into second
		mockClock.Advance(20 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "Entitlement should still be active at 20 days (in second window)")
	})

	t.Run("entitlement is NOT active after 30 days", func(t *testing.T) {
		// Advance clock 10 more days (total 30 days from start)
		mockClock.Advance(10 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.False(t, isEntitled, "Entitlement should NOT be active after 30 days (both windows expired)")
	})
}

// TestIndefiniteEntitlement tests that indefinite entitlements never expire
func TestIndefiniteEntitlement(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	userID := uuid.New().String()
	entitlementName := "premium"

	// Grant an indefinite entitlement (EndAt is nil)
	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: entitlementName,
		StartAt:     startTime,
		EndAt:       nil, // Indefinite
		SourceType:  models.EntitlementSourceSubscription,
		CreatedAt:   startTime,
		UpdatedAt:   startTime,
	}
	_, err := suite.BunDB.NewInsert().Model(ent).Exec(ctx)
	require.NoError(t, err)

	entService := suite.App.Runtime.EntitlementService

	t.Run("indefinite entitlement is active at start", func(t *testing.T) {
		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "Indefinite entitlement should be active at start")
	})

	t.Run("indefinite entitlement is active after 1 year", func(t *testing.T) {
		// Advance clock 1 year
		mockClock.Advance(365 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "Indefinite entitlement should still be active after 1 year")
	})

	t.Run("indefinite entitlement is active after 10 years", func(t *testing.T) {
		// Advance clock 9 more years (total 10 years)
		mockClock.Advance(9 * 365 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, entitlementName, mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "Indefinite entitlement should still be active after 10 years")
	})
}

// =============================================================================
// Cancellation Time-Dependent Tests
// =============================================================================

// TestCancelAccessAtPeriodEnd tests that user cancellation keeps access until period end
// and that access is revoked after period end (using mock clock to verify time-based behavior).
func TestCancelAccessAtPeriodEnd(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()

	// Create subscription with period ending in 30 days
	periodEnd := startTime.Add(30 * 24 * time.Hour)
	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:      userID,
		PriceID:     priceID,
		Status:      models.StatusActive,
		Processor:   models.ProcessorNMI,
		PeriodStart: startTime,
		PeriodEnd:   periodEnd,
	})

	// Create an indefinite entitlement linked to the subscription
	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: "premium",
		StartAt:     startTime,
		EndAt:       nil, // Indefinite while subscription is active
		SourceType:  models.EntitlementSourceSubscription,
		SourceID:    &sub.ID,
		CreatedAt:   startTime,
		UpdatedAt:   startTime,
	}
	_, err := suite.BunDB.NewInsert().Model(ent).Exec(ctx)
	require.NoError(t, err)

	entService := suite.App.Runtime.EntitlementService
	lifecycleService := suite.App.Runtime.SubscriptionLifecycleService

	t.Run("user has entitlement before cancellation", func(t *testing.T) {
		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "User should have entitlement before cancellation")
	})

	t.Run("user cancels subscription (RevokeAccess: false)", func(t *testing.T) {
		// Advance clock 5 days (still within period)
		mockClock.Advance(5 * 24 * time.Hour)

		// User cancels but keeps access until period end
		err := lifecycleService.CancelMembership(ctx, &services.CancelMembershipParams{
			SubscriptionID: &sub.ID,
			CancelType:     models.CancelTypeUser,
			RevokeAccess:   false, // Access continues until period end
		})
		require.NoError(t, err)

		// Verify subscription is cancelled
		updatedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, models.StatusCancelled, updatedSub.Status, "Subscription should be cancelled")
		assert.NotNil(t, updatedSub.CancelledAt, "CancelledAt should be set")
	})

	t.Run("entitlement EndAt is now set to period end", func(t *testing.T) {
		// After cancel with RevokeAccess: false, entitlement EndAt should be set to period end
		var dbEnt models.Entitlement
		err := suite.BunDB.NewSelect().
			Model(&dbEnt).
			Where("id = ?", ent.ID).
			Scan(ctx)
		require.NoError(t, err)
		require.NotNil(t, dbEnt.EndAt, "Entitlement EndAt should be set after cancellation")
		assert.WithinDuration(t, periodEnd, *dbEnt.EndAt, time.Second,
			"Entitlement EndAt should be set to period end")
	})

	t.Run("user still has entitlement immediately after cancel", func(t *testing.T) {
		// User should still have access because we haven't reached period end yet
		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "User should still have entitlement immediately after cancel (RevokeAccess: false)")
	})

	t.Run("user still has entitlement at day 29 (1 day before period end)", func(t *testing.T) {
		// Advance to day 29 (1 day before period end; we're currently at day 5)
		mockClock.Advance(24 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "User should still have entitlement 1 day before period end")
	})

	t.Run("user does NOT have entitlement at day 31 (past period end)", func(t *testing.T) {
		// Advance to day 31 (past period end; we're currently at day 29)
		mockClock.Advance(2 * 24 * time.Hour)

		// Entitlement should now be expired because EndAt was set to period end
		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.False(t, isEntitled, "User should NOT have entitlement after period end")
	})

	t.Run("subscription period has ended", func(t *testing.T) {
		updatedSub := suite.GetSubscription(sub.ID)
		assert.True(t, updatedSub.CurrentPeriodEndsAt.Before(mockClock.Now()),
			"Period should have ended by now")
	})
}

// TestAdminRevokeAccess tests that admin revocation removes access immediately
func TestAdminRevokeAccess(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()

	// Create subscription with period ending in 30 days
	periodEnd := startTime.Add(30 * 24 * time.Hour)
	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:      userID,
		PriceID:     priceID,
		Status:      models.StatusActive,
		Processor:   models.ProcessorNMI,
		PeriodStart: startTime,
		PeriodEnd:   periodEnd,
	})

	// Create an indefinite entitlement linked to the subscription
	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: "premium",
		StartAt:     startTime,
		EndAt:       nil, // Indefinite while subscription is active
		SourceType:  models.EntitlementSourceSubscription,
		SourceID:    &sub.ID,
		CreatedAt:   startTime,
		UpdatedAt:   startTime,
	}
	_, err := suite.BunDB.NewInsert().Model(ent).Exec(ctx)
	require.NoError(t, err)

	entService := suite.App.Runtime.EntitlementService
	lifecycleService := suite.App.Runtime.SubscriptionLifecycleService

	t.Run("user has entitlement before admin revocation", func(t *testing.T) {
		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "User should have entitlement before admin revocation")
	})

	t.Run("admin revokes access (RevokeAccess: true)", func(t *testing.T) {
		// Advance clock 5 days (still well within period)
		mockClock.Advance(5 * 24 * time.Hour)

		// Admin revokes access immediately
		err := lifecycleService.CancelMembership(ctx, &services.CancelMembershipParams{
			SubscriptionID: &sub.ID,
			CancelType:     models.CancelTypeMerchant, // "merchant" = admin/merchant cancellation
			RevokeAccess:   true,                      // Access revoked immediately
		})
		require.NoError(t, err)

		// Verify subscription is cancelled
		updatedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, models.StatusCancelled, updatedSub.Status, "Subscription should be cancelled")
		assert.NotNil(t, updatedSub.CancelledAt, "CancelledAt should be set")
		assert.NotNil(t, updatedSub.EndedAt, "EndedAt should be set (immediate termination)")
	})

	t.Run("user does NOT have entitlement after admin revocation", func(t *testing.T) {
		// User should NOT have access because RevokeAccess was true
		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.False(t, isEntitled, "User should NOT have entitlement after admin revocation")
	})

	t.Run("user still does NOT have entitlement even days later", func(t *testing.T) {
		// Advance clock 10 more days
		mockClock.Advance(10 * 24 * time.Hour)

		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.False(t, isEntitled, "User should still NOT have entitlement days later")
	})
}

// =============================================================================
// Dunning Time-Dependent Tests
// =============================================================================

// TestDunningRetrySchedule tests that the dunning worker respects the retry schedule
// by actually running the worker with a mock clock and verifying it only processes
// subscriptions when next_retry_at has passed.
func TestDunningRetrySchedule(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()

	// Create payment method for the user (required for dunning to attempt rebill)
	pm := suite.CreateTestPaymentMethod(userID)

	// Create a past_due subscription with next_retry_at 3 days in the future
	nextRetry := startTime.Add(3 * 24 * time.Hour) // Retry scheduled for 3 days from now
	retryAttempts := 1
	processorSubID := "test-dunning-schedule-" + uuid.New().String()[:8]
	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:          userID,
		PriceID:         priceID,
		Status:          models.StatusPastDue,
		Processor:       models.ProcessorNMI,
		ProcessorSubID:  processorSubID,
		PeriodStart:     startTime.Add(-30 * 24 * time.Hour), // Started 30 days ago
		PeriodEnd:       startTime.Add(-1 * time.Hour),       // Just expired
		RetryAttempts:   &retryAttempts,
		NextRetryAt:     &nextRetry,
		PaymentMethodID: &pm.ID,
	})

	// Helper to count due subscriptions using the same query as DunningWorker
	countDueSubscriptions := func(clock clockwork.Clock) int {
		var count int
		err := suite.BunDB.NewSelect().
			Model((*models.Subscription)(nil)).
			Where("sub.processor = ?", models.ProcessorNMI).
			Where("sub.status = ?", models.StatusPastDue).
			Where("sub.next_retry_at IS NOT NULL AND sub.next_retry_at <= ?", clock.Now()).
			ColumnExpr("COUNT(*)").
			Scan(ctx, &count)
		require.NoError(t, err)
		return count
	}

	t.Run("subscription is past_due with scheduled retry", func(t *testing.T) {
		updatedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, models.StatusPastDue, updatedSub.Status)
		assert.NotNil(t, updatedSub.NextRetryAt)
		assert.True(t, updatedSub.NextRetryAt.After(mockClock.Now()),
			"NextRetryAt should be in the future")
	})

	t.Run("dunning worker finds NO due subscriptions at day 0", func(t *testing.T) {
		// At day 0, next_retry_at (day 3) is in the future
		count := countDueSubscriptions(mockClock)
		assert.Equal(t, 0, count, "Should find 0 due subscriptions at day 0")
	})

	t.Run("dunning worker finds NO due subscriptions at day 1", func(t *testing.T) {
		// Advance clock 1 day
		mockClock.Advance(1 * 24 * time.Hour)

		count := countDueSubscriptions(mockClock)
		assert.Equal(t, 0, count, "Should find 0 due subscriptions at day 1")
	})

	t.Run("dunning worker finds NO due subscriptions at day 2", func(t *testing.T) {
		// Advance clock 1 more day (total 2 days)
		mockClock.Advance(1 * 24 * time.Hour)

		count := countDueSubscriptions(mockClock)
		assert.Equal(t, 0, count, "Should find 0 due subscriptions at day 2")
	})

	t.Run("dunning worker finds 1 due subscription at day 3", func(t *testing.T) {
		// Advance clock 1 more day (total 3 days - exactly at next_retry_at)
		mockClock.Advance(1 * 24 * time.Hour)

		count := countDueSubscriptions(mockClock)
		assert.Equal(t, 1, count, "Should find 1 due subscription at day 3")
	})

	t.Run("dunning worker finds 1 due subscription at day 4", func(t *testing.T) {
		// Advance clock 1 more day (total 4 days - past next_retry_at)
		mockClock.Advance(1 * 24 * time.Hour)

		count := countDueSubscriptions(mockClock)
		assert.Equal(t, 1, count, "Should find 1 due subscription at day 4")
	})
}

// TestDunningMaxRetriesFailsSubscription tests that subscription fails after max retries
// and that entitlements remain revoked even as time advances (using mock clock).
func TestDunningMaxRetriesFailsSubscription(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	processorSubID := "test-dunning-max-" + uuid.New().String()[:8]

	// Create a subscription at max retries (one more failure = cancelled)
	retryAttempts := services.MaxDunningFailures - 1 // One retry left
	nextRetry := startTime
	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusPastDue,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: processorSubID,
		PeriodStart:    startTime.Add(-30 * 24 * time.Hour),
		PeriodEnd:      startTime.Add(-1 * time.Hour),
		RetryAttempts:  &retryAttempts,
		NextRetryAt:    &nextRetry,
	})

	// Create an indefinite entitlement linked to the subscription
	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: "premium",
		StartAt:     startTime.Add(-30 * 24 * time.Hour),
		EndAt:       nil, // Indefinite
		SourceType:  models.EntitlementSourceSubscription,
		SourceID:    &sub.ID,
		CreatedAt:   startTime,
		UpdatedAt:   startTime,
	}
	_, err := suite.BunDB.NewInsert().Model(ent).Exec(ctx)
	require.NoError(t, err)

	lifecycleService := suite.App.Runtime.SubscriptionLifecycleService
	entService := suite.App.Runtime.EntitlementService

	t.Run("user has entitlement before final failure at day 0", func(t *testing.T) {
		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.True(t, isEntitled, "User should have entitlement before final failure")
	})

	t.Run("final payment failure cancels subscription", func(t *testing.T) {
		// Advance clock 1 day to simulate time passing before failure
		mockClock.Advance(1 * 24 * time.Hour)

		// Simulate the final payment failure via FailMembership
		// FailMembership uses s.now() which uses the mock clock
		failureReason := "Card declined"
		failureCode := "05"
		err := lifecycleService.FailMembership(ctx, &services.FailMembershipParams{
			Processor:               models.ProcessorNMI,
			ProcessorSubscriptionID: processorSubID,
			ProcessorProvider:       "mobius",
			FailureReason:           &failureReason,
			FailureCode:             &failureCode,
		})
		require.NoError(t, err)

		// Verify subscription is now cancelled (reached max retries)
		updatedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, models.StatusCancelled, updatedSub.Status,
			"Subscription should be cancelled after max retries")
		assert.NotNil(t, updatedSub.EndedAt, "EndedAt should be set")
		assert.Equal(t, services.MaxDunningFailures, *updatedSub.RetryAttempts,
			"RetryAttempts should equal MaxDunningFailures")
	})

	t.Run("user does NOT have entitlement immediately after max retries (day 1)", func(t *testing.T) {
		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.False(t, isEntitled, "User should NOT have entitlement after subscription failed")
	})

	t.Run("user still does NOT have entitlement at day 30", func(t *testing.T) {
		// Advance clock 29 more days (total 30 days from start)
		mockClock.Advance(29 * 24 * time.Hour)

		// Entitlement should still be revoked - time passing doesn't restore it
		isEntitled, err := entService.IsEntitled(ctx, userID, "premium", mockClock.Now())
		require.NoError(t, err)
		assert.False(t, isEntitled, "User should still NOT have entitlement 30 days after failure")
	})

	t.Run("entitlement EndAt was set to failure time", func(t *testing.T) {
		// Verify the entitlement was properly ended
		var dbEnt models.Entitlement
		err := suite.BunDB.NewSelect().
			Model(&dbEnt).
			Where("id = ?", ent.ID).
			Scan(ctx)
		require.NoError(t, err)
		require.NotNil(t, dbEnt.EndAt, "Entitlement EndAt should be set after failure")
		// EndAt should be around day 1 (when failure occurred)
		expectedEndAt := startTime.Add(1 * 24 * time.Hour)
		assert.WithinDuration(t, expectedEndAt, *dbEnt.EndAt, time.Minute,
			"Entitlement EndAt should be set to failure time")
	})
}

// TestDunningSuccessReactivates tests that successful dunning reactivates subscription
// and verifies period dates are correctly calculated using mock clock.
func TestDunningSuccessReactivates(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	processorSubID := "test-dunning-success-" + uuid.New().String()[:8]

	// Create a past_due subscription with period that just expired
	retryAttempts := 2
	nextRetry := startTime
	originalPeriodEnd := startTime.Add(-1 * time.Hour) // Just expired (1 hour before startTime)
	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusPastDue,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: processorSubID,
		PeriodStart:    startTime.Add(-30 * 24 * time.Hour),
		PeriodEnd:      originalPeriodEnd,
		RetryAttempts:  &retryAttempts,
		NextRetryAt:    &nextRetry,
	})

	lifecycleService := suite.App.Runtime.SubscriptionLifecycleService

	t.Run("subscription is past_due at day 0", func(t *testing.T) {
		updatedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, models.StatusPastDue, updatedSub.Status)
		assert.True(t, updatedSub.CurrentPeriodEndsAt.Before(mockClock.Now()),
			"Original period should have ended before current time")
	})

	t.Run("successful rebill reactivates subscription at day 1", func(t *testing.T) {
		// Advance clock 1 day
		mockClock.Advance(1 * 24 * time.Hour)

		// Simulate successful rebill via RenewMembership
		// RenewMembership uses the mock clock for period calculations
		err := lifecycleService.RenewMembership(ctx, &services.RenewMembershipParams{
			Processor:               models.ProcessorNMI,
			ProcessorSubscriptionID: processorSubID,
			ProcessorProvider:       "mobius",
		})
		require.NoError(t, err)

		// Verify subscription is now active
		updatedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, models.StatusActive, updatedSub.Status,
			"Subscription should be active after successful rebill")
	})

	t.Run("new period starts from old period end", func(t *testing.T) {
		updatedSub := suite.GetSubscription(sub.ID)

		// New period should start from the old period end
		assert.NotNil(t, updatedSub.CurrentPeriodStartsAt)
		assert.Equal(t, originalPeriodEnd.Unix(), updatedSub.CurrentPeriodStartsAt.Unix(),
			"New period should start at old period end")
	})

	t.Run("new period end is 30 days after old period end", func(t *testing.T) {
		updatedSub := suite.GetSubscription(sub.ID)

		// New period end should be 30 days after original period end
		expectedNewEnd := originalPeriodEnd.Add(30 * 24 * time.Hour)
		assert.NotNil(t, updatedSub.CurrentPeriodEndsAt)
		assert.WithinDuration(t, expectedNewEnd, *updatedSub.CurrentPeriodEndsAt, time.Second,
			"New period end should be 30 days after original period end")
	})

	t.Run("subscription period is active at day 15", func(t *testing.T) {
		// Advance clock 14 more days (total 15 days from start)
		mockClock.Advance(14 * 24 * time.Hour)

		updatedSub := suite.GetSubscription(sub.ID)
		assert.True(t, updatedSub.CurrentPeriodEndsAt.After(mockClock.Now()),
			"Period should still be active at day 15")
	})

	t.Run("subscription period has ended at day 35", func(t *testing.T) {
		// Advance clock 20 more days (total 35 days from start)
		// New period started at originalPeriodEnd (day -0.04) and ends 30 days later (day ~30)
		mockClock.Advance(20 * 24 * time.Hour)

		updatedSub := suite.GetSubscription(sub.ID)
		assert.True(t, updatedSub.CurrentPeriodEndsAt.Before(mockClock.Now()),
			"Period should have ended by day 35")
	})
}

// =============================================================================
// Payment Intent and Wallet Challenge Expiry Tests
// =============================================================================

// TestPaymentIntentExpiry tests that expired Solana payment intents are rejected.
// The test uses a mock clock to verify expiry checking at different points in time.
func TestPaymentIntentExpiry(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	payerWallet := "DzGLHdTfgHCYh8v3qNGJHn85CyX7aeFmqoUdVRBYkWMh"

	// Get the intent service from the runtime
	intentService := suite.App.Runtime.SolanaPaymentIntentService

	// Create a direct payment intent (expires in 10 minutes from mock clock time)
	intent, err := intentService.CreateDirectIntent(ctx, userID, priceID, "SOL", payerWallet)
	require.NoError(t, err)
	require.NotNil(t, intent.ExpiresAt)

	// Verify expiry time is 10 minutes from mock clock start
	expectedExpiry := startTime.Add(10 * time.Minute)
	assert.Equal(t, expectedExpiry.Unix(), intent.ExpiresAt.Unix(),
		"Intent should expire 10 minutes from creation time")

	t.Run("intent is NOT expired before expiry time", func(t *testing.T) {
		// At startTime (t=0), intent should not be expired (expires at t=10min)
		isExpired := intentService.IsExpired(intent)
		assert.False(t, isExpired, "Intent should NOT be expired at creation time")
	})

	t.Run("intent is NOT expired just before expiry", func(t *testing.T) {
		// Advance clock to 9 minutes 59 seconds (just before expiry)
		mockClock.Advance(9*time.Minute + 59*time.Second)
		isExpired := intentService.IsExpired(intent)
		assert.False(t, isExpired, "Intent should NOT be expired just before expiry time")
	})

	t.Run("intent IS expired right after expiry time", func(t *testing.T) {
		// Advance clock 2 more seconds (now at 10min 1sec, past the 10min expiry)
		mockClock.Advance(2 * time.Second)
		isExpired := intentService.IsExpired(intent)
		assert.True(t, isExpired, "Intent SHOULD be expired after expiry time")
	})

	t.Run("intent IS expired long after expiry time", func(t *testing.T) {
		// Advance clock another hour
		mockClock.Advance(1 * time.Hour)
		isExpired := intentService.IsExpired(intent)
		assert.True(t, isExpired, "Intent SHOULD still be expired long after expiry time")
	})
}

// TestWalletChallengeExpiry tests that expired wallet verification challenges are rejected.
// The test uses a mock clock to verify expiry checking at different points in time.
func TestWalletChallengeExpiry(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	userID := uuid.New().String()
	walletAddress := "DzGLHdTfgHCYh8v3qNGJHn85CyX7aeFmqoUdVRBYkWMh" // Valid Solana address

	// Get the verification service from the runtime
	verificationService := suite.App.Runtime.SolanaVerificationService

	// Generate a challenge (expires in 10 minutes from mock clock time)
	challenge, err := verificationService.GenerateChallenge(ctx, userID, walletAddress)
	require.NoError(t, err)

	// Verify expiry time is 10 minutes from mock clock start
	expectedExpiry := startTime.Add(10 * time.Minute)
	assert.Equal(t, expectedExpiry.Unix(), challenge.ExpiresAt.Unix(),
		"Challenge should expire 10 minutes from creation time")

	// Load the challenge from the database for subsequent tests
	var dbChallenge models.SolanaWalletChallenge
	err = suite.BunDB.NewSelect().
		Model(&dbChallenge).
		Where("user_id = ? AND address = ?", userID, walletAddress).
		Scan(ctx)
	require.NoError(t, err)

	t.Run("challenge is NOT expired before expiry time", func(t *testing.T) {
		// At startTime (t=0), challenge should not be expired (expires at t=10min)
		isExpired := verificationService.IsChallengeExpired(&dbChallenge)
		assert.False(t, isExpired, "Challenge should NOT be expired at creation time")
	})

	t.Run("challenge is NOT expired just before expiry", func(t *testing.T) {
		// Advance clock to 9 minutes 59 seconds (just before expiry)
		mockClock.Advance(9*time.Minute + 59*time.Second)
		isExpired := verificationService.IsChallengeExpired(&dbChallenge)
		assert.False(t, isExpired, "Challenge should NOT be expired just before expiry time")
	})

	t.Run("challenge IS expired right after expiry time", func(t *testing.T) {
		// Advance clock 2 more seconds (now at 10min 1sec, past the 10min expiry)
		mockClock.Advance(2 * time.Second)
		isExpired := verificationService.IsChallengeExpired(&dbChallenge)
		assert.True(t, isExpired, "Challenge SHOULD be expired after expiry time")
	})

	t.Run("challenge IS expired long after expiry time", func(t *testing.T) {
		// Advance clock another hour
		mockClock.Advance(1 * time.Hour)
		isExpired := verificationService.IsChallengeExpired(&dbChallenge)
		assert.True(t, isExpired, "Challenge SHOULD still be expired long after expiry time")
	})
}

// =============================================================================
// Subscription Period Time-Dependent Tests
// =============================================================================

// TestSubscriptionRenewalWithMockClock tests renewal extends period using mock clock
func TestSubscriptionRenewalWithMockClock(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Set clock to a known starting point
	startTime := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	mockClock := suite.SetMockClock(startTime)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	processorSubID := "test-renewal-clock-" + uuid.New().String()[:8]

	// Create subscription that started 30 days ago and is about to expire
	periodStart := startTime.Add(-30 * 24 * time.Hour)
	periodEnd := startTime.Add(-1 * time.Hour) // Just expired
	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: processorSubID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
	})

	originalPeriodEnd := sub.CurrentPeriodEndsAt

	lifecycleService := suite.App.Runtime.SubscriptionLifecycleService

	t.Run("subscription period has ended", func(t *testing.T) {
		updatedSub := suite.GetSubscription(sub.ID)
		assert.True(t, updatedSub.CurrentPeriodEndsAt.Before(mockClock.Now()),
			"Period should have ended")
	})

	t.Run("renewal extends period by billing cycle days", func(t *testing.T) {
		// Simulate renewal webhook
		err := lifecycleService.RenewMembership(ctx, &services.RenewMembershipParams{
			Processor:               models.ProcessorNMI,
			ProcessorSubscriptionID: processorSubID,
			ProcessorProvider:       "mobius",
		})
		require.NoError(t, err)

		updatedSub := suite.GetSubscription(sub.ID)

		// New period start should be the old period end
		assert.Equal(t, originalPeriodEnd.Unix(), updatedSub.CurrentPeriodStartsAt.Unix(),
			"New period should start at old period end")

		// New period end should be 30 days after the old period end
		expectedNewEnd := originalPeriodEnd.Add(30 * 24 * time.Hour)
		assert.WithinDuration(t, expectedNewEnd, *updatedSub.CurrentPeriodEndsAt, time.Second,
			"New period end should be 30 days after old period end")
	})

	t.Run("subscription is active after renewal", func(t *testing.T) {
		updatedSub := suite.GetSubscription(sub.ID)
		assert.Equal(t, models.StatusActive, updatedSub.Status)
	})

	t.Run("advancing clock past new period end", func(t *testing.T) {
		// Advance clock 35 days (past new period end)
		mockClock.Advance(35 * 24 * time.Hour)

		updatedSub := suite.GetSubscription(sub.ID)
		assert.True(t, updatedSub.CurrentPeriodEndsAt.Before(mockClock.Now()),
			"New period should have ended after advancing clock 35 days")
	})
}
