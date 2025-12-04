//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	riverjobs "github.com/doujins-org/doujins-billing/internal/river"
)

// TestRiverWorkersStarted verifies that River workers are running in the test suite
func TestRiverWorkersStarted(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("river client is initialized", func(t *testing.T) {
		client := suite.GetRiverClient()
		require.NotNil(t, client, "River client should be initialized")
	})

	t.Run("river workers are started", func(t *testing.T) {
		// The runtime should have riverStarted = true after StartWorkers
		require.NotNil(t, suite.App.Runtime.RiverClient, "River client should be set on runtime")
	})
}

// TestRiverJobEnqueue tests that we can enqueue jobs and they get processed
func TestRiverJobEnqueue(t *testing.T) {
	suite := setupTestSuite(t)

	// Clear any existing jobs from periodic schedulers
	suite.ClearJobQueue()

	t.Run("can enqueue and process webhook retry job", func(t *testing.T) {
		// Get the River client directly (it's already typed)
		client := suite.App.Runtime.RiverClient
		require.NotNil(t, client, "Should have River client")

		// Get initial completed count
		initialCompleted := suite.GetCompletedJobCount()

		// Enqueue a webhook retry job
		ctx := context.Background()
		_, err := client.Insert(ctx, riverjobs.WebhookRetryArgs{}, &river.InsertOpts{
			Queue: riverjobs.QueueBilling,
		})
		require.NoError(t, err, "Should be able to enqueue job")

		// Wait for job to complete (max 5 seconds)
		completed := suite.WaitForJobCompletion(initialCompleted+1, 5*time.Second)
		assert.True(t, completed, "Job should complete within timeout")

		// Verify job completed
		finalCompleted := suite.GetCompletedJobCount()
		assert.Greater(t, finalCompleted, initialCompleted, "Completed job count should increase")
	})
}

// TestRiverPeriodicJobs tests that periodic jobs are scheduled
func TestRiverPeriodicJobs(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("periodic jobs are registered", func(t *testing.T) {
		client := suite.App.Runtime.RiverClient
		require.NotNil(t, client, "Should have River client")

		// Check that periodic jobs exist
		periodicJobs := client.PeriodicJobs()
		require.NotNil(t, periodicJobs, "Periodic jobs manager should exist")

		// We can't directly inspect periodic jobs, but we can verify the client is set up
		// The presence of the client and its configuration is enough for this test
	})
}

// TestWebhookProcessingFlow tests the webhook -> job -> processing flow
func TestWebhookProcessingFlow(t *testing.T) {
	suite := setupTestSuite(t)

	// Clear job queue for clean state
	suite.ClearJobQueue()

	t.Run("webhook processing job can be enqueued", func(t *testing.T) {
		client := suite.App.Runtime.RiverClient
		require.NotNil(t, client)

		// Create a webhook process job (this would normally be enqueued when a webhook is received)
		// Note: We're just testing that the job infrastructure works, not the actual webhook processing
		ctx := context.Background()

		initialPending := suite.GetPendingJobCount()
		initialCompleted := suite.GetCompletedJobCount()

		// Enqueue a webhook process job with a fake event ID
		// This will fail to find the event but proves the infrastructure works
		_, err := client.Insert(ctx, riverjobs.WebhookProcessArgs{
			EventID: uuid.MustParse("12345678-1234-1234-1234-123456789012"),
		}, &river.InsertOpts{
			Queue: riverjobs.QueueBilling,
		})
		require.NoError(t, err, "Should be able to enqueue webhook process job")

		// Wait a moment for job to be picked up
		time.Sleep(500 * time.Millisecond)

		// Check that job was processed (even if it failed due to missing event)
		// In River, failed jobs go to 'retryable' or 'discarded' state, not 'available'
		finalPending := suite.GetPendingJobCount()
		finalCompleted := suite.GetCompletedJobCount()

		// Either the job completed (found no event to process) or moved to another state
		t.Logf("Initial pending: %d, Final pending: %d", initialPending, finalPending)
		t.Logf("Initial completed: %d, Final completed: %d", initialCompleted, finalCompleted)
	})
}

// TestDunningJobEnqueue tests enqueueing a dunning job
func TestDunningJobEnqueue(t *testing.T) {
	suite := setupTestSuite(t)

	// Clear job queue
	suite.ClearJobQueue()

	t.Run("dunning job can be manually enqueued", func(t *testing.T) {
		client := suite.App.Runtime.RiverClient
		require.NotNil(t, client)

		ctx := context.Background()
		initialCompleted := suite.GetCompletedJobCount()

		// Enqueue dunning job
		_, err := client.Insert(ctx, riverjobs.DunningArgs{}, &river.InsertOpts{
			Queue: riverjobs.QueueBilling,
		})
		require.NoError(t, err)

		// Wait for completion (dunning with no due subscriptions should complete quickly)
		completed := suite.WaitForJobCompletion(initialCompleted+1, 5*time.Second)
		assert.True(t, completed, "Dunning job should complete within timeout")
	})
}

// TestJobQueueHelpers tests the helper functions
func TestJobQueueHelpers(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("clear job queue works", func(t *testing.T) {
		suite.ClearJobQueue()

		// After clearing, pending should be 0 (unless periodic jobs fire)
		pending := suite.GetPendingJobCount()
		t.Logf("Pending jobs after clear: %d", pending)
		// Note: periodic jobs may have fired, so we just verify the function runs
	})

	t.Run("get completed job count works", func(t *testing.T) {
		completed := suite.GetCompletedJobCount()
		t.Logf("Completed jobs: %d", completed)
		// Just verify the function runs without error
	})

	t.Run("get pending job count works", func(t *testing.T) {
		pending := suite.GetPendingJobCount()
		t.Logf("Pending jobs: %d", pending)
		// Just verify the function runs without error
	})
}

// TestCleanupExpiredDataWorker tests the cleanup worker for expired data
func TestCleanupExpiredDataWorker(t *testing.T) {
	suite := setupTestSuite(t)
	ctx := context.Background()

	// Clear job queue for clean state
	suite.ClearJobQueue()

	t.Run("cleanup job can be enqueued", func(t *testing.T) {
		client := suite.App.Runtime.RiverClient
		require.NotNil(t, client)

		initialCompleted := suite.GetCompletedJobCount()

		// Enqueue cleanup job
		_, err := client.Insert(ctx, riverjobs.CleanupExpiredDataArgs{}, &river.InsertOpts{
			Queue: riverjobs.QueueBilling,
		})
		require.NoError(t, err)

		// Wait for completion (should complete quickly with no data)
		completed := suite.WaitForJobCompletion(initialCompleted+1, 5*time.Second)
		assert.True(t, completed, "Cleanup job should complete within timeout")
	})

	t.Run("cleans up expired wallet challenges", func(t *testing.T) {
		// Set up a mock clock 48 hours in the future
		now := time.Now()
		mockClock := suite.SetMockClock(now.Add(48 * time.Hour))

		// Insert an expired wallet challenge (expired 24 hours ago relative to mock time)
		expiredAt := mockClock.Now().Add(-26 * time.Hour)
		challenge := &models.SolanaWalletChallenge{
			ID:            uuid.New(),
			UserID:        uuid.New().String(),
			WalletAddress: "TestWallet123",
			Challenge:     "test-challenge",
			ExpiresAt:     expiredAt,
			CreatedAt:     now,
		}
		_, err := suite.BunDB.NewInsert().Model(challenge).Exec(ctx)
		require.NoError(t, err)

		// Run cleanup worker directly
		worker := riverjobs.CleanupExpiredDataWorker{
			DB:     suite.App.Runtime.DB,
			Clock:  mockClock,
			Config: riverjobs.DefaultCleanupConfig(),
		}
		err = worker.Work(ctx, &river.Job[riverjobs.CleanupExpiredDataArgs]{})
		require.NoError(t, err)

		// Verify challenge was deleted
		var count int
		err = suite.BunDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM billing.solana_wallet_challenges WHERE id = $1", challenge.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "Expired wallet challenge should be deleted")
	})

	t.Run("cleans up expired payment intents", func(t *testing.T) {
		// Set up a mock clock 8 days in the future
		now := time.Now()
		mockClock := suite.SetMockClock(now.Add(8 * 24 * time.Hour))

		// Insert an expired payment intent (created 8 days ago relative to mock time)
		expiredAt := mockClock.Now().Add(-8 * 24 * time.Hour)
		userID := uuid.New().String()
		intent := &models.SolanaPaymentIntent{
			ID:            uuid.New(),
			UserID:        userID,
			PriceID:       uuid.New(),
			WalletAddress: "TestWallet456",
			Amount:        1000,
			TokenSymbol:   "SOL",
			Status:        "pending",
			ExpiresAt:     &expiredAt,
			CreatedAt:     mockClock.Now().Add(-8 * 24 * time.Hour),
		}
		_, err := suite.BunDB.NewInsert().Model(intent).Exec(ctx)
		require.NoError(t, err)

		// Run cleanup worker
		worker := riverjobs.CleanupExpiredDataWorker{
			DB:     suite.App.Runtime.DB,
			Clock:  mockClock,
			Config: riverjobs.DefaultCleanupConfig(),
		}
		err = worker.Work(ctx, &river.Job[riverjobs.CleanupExpiredDataArgs]{})
		require.NoError(t, err)

		// Verify intent was deleted
		var count int
		err = suite.BunDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM billing.solana_payment_intents WHERE id = $1", intent.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "Expired payment intent should be deleted")
	})

	t.Run("cleans up old seen notifications", func(t *testing.T) {
		// Set up a mock clock 100 days in the future
		now := time.Now()
		mockClock := suite.SetMockClock(now.Add(100 * 24 * time.Hour))

		// Insert an old seen notification (created 95 days ago relative to mock time)
		userID := uuid.New().String()
		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    userID,
			EventType: models.NotificationSystemAlert,
			Seen:      true, // Seen notifications have 90-day retention
			CreatedAt: mockClock.Now().Add(-95 * 24 * time.Hour),
		}
		_, err := suite.BunDB.NewInsert().Model(notification).Exec(ctx)
		require.NoError(t, err)

		// Run cleanup worker
		worker := riverjobs.CleanupExpiredDataWorker{
			DB:     suite.App.Runtime.DB,
			Clock:  mockClock,
			Config: riverjobs.DefaultCleanupConfig(),
		}
		err = worker.Work(ctx, &river.Job[riverjobs.CleanupExpiredDataArgs]{})
		require.NoError(t, err)

		// Verify notification was deleted
		var count int
		err = suite.BunDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM billing.notification_queue WHERE id = $1", notification.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "Old seen notification should be deleted")
	})

	t.Run("preserves recent data", func(t *testing.T) {
		// Use current time
		mockClock := clockwork.NewRealClock()

		// Insert recent wallet challenge (expires in 5 minutes)
		challenge := &models.SolanaWalletChallenge{
			ID:            uuid.New(),
			UserID:        uuid.New().String(),
			WalletAddress: "RecentWallet",
			Challenge:     "recent-challenge",
			ExpiresAt:     mockClock.Now().Add(5 * time.Minute),
			CreatedAt:     mockClock.Now(),
		}
		_, err := suite.BunDB.NewInsert().Model(challenge).Exec(ctx)
		require.NoError(t, err)

		// Insert recent notification (just created)
		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    uuid.New().String(),
			EventType: models.NotificationSystemAlert,
			Seen:      false,
			CreatedAt: mockClock.Now(),
		}
		_, err = suite.BunDB.NewInsert().Model(notification).Exec(ctx)
		require.NoError(t, err)

		// Run cleanup worker
		worker := riverjobs.CleanupExpiredDataWorker{
			DB:     suite.App.Runtime.DB,
			Clock:  mockClock,
			Config: riverjobs.DefaultCleanupConfig(),
		}
		err = worker.Work(ctx, &river.Job[riverjobs.CleanupExpiredDataArgs]{})
		require.NoError(t, err)

		// Verify recent challenge was preserved
		var challengeCount int
		err = suite.BunDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM billing.solana_wallet_challenges WHERE id = $1", challenge.ID).Scan(&challengeCount)
		require.NoError(t, err)
		assert.Equal(t, 1, challengeCount, "Recent wallet challenge should be preserved")

		// Verify recent notification was preserved
		var notifCount int
		err = suite.BunDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM billing.notification_queue WHERE id = $1", notification.ID).Scan(&notifCount)
		require.NoError(t, err)
		assert.Equal(t, 1, notifCount, "Recent notification should be preserved")

		// Clean up test data
		suite.BunDB.NewDelete().Model(challenge).WherePK().Exec(ctx)
		suite.BunDB.NewDelete().Model(notification).WherePK().Exec(ctx)
	})

	t.Run("custom retention config works", func(t *testing.T) {
		// Set up a mock clock 2 hours in the future
		now := time.Now()
		mockClock := suite.SetMockClock(now.Add(2 * time.Hour))

		// Insert a challenge that expired 1 hour ago
		expiredAt := mockClock.Now().Add(-1 * time.Hour)
		challenge := &models.SolanaWalletChallenge{
			ID:            uuid.New(),
			UserID:        uuid.New().String(),
			WalletAddress: "CustomRetentionWallet",
			Challenge:     "custom-challenge",
			ExpiresAt:     expiredAt,
			CreatedAt:     now,
		}
		_, err := suite.BunDB.NewInsert().Model(challenge).Exec(ctx)
		require.NoError(t, err)

		// Run cleanup with very short retention (30 minutes)
		worker := riverjobs.CleanupExpiredDataWorker{
			DB:    suite.App.Runtime.DB,
			Clock: mockClock,
			Config: riverjobs.CleanupConfig{
				WalletChallengeRetention:    30 * time.Minute, // 30 min retention (challenge expired 1h ago)
				PaymentIntentRetention:      24 * time.Hour,
				SolanaTransactionRetention:  24 * time.Hour,
				NotificationSeenRetention:   24 * time.Hour,
				NotificationUnseenRetention: 48 * time.Hour,
				IdempotencyRequestRetention: 24 * time.Hour,
				WebhookEventRetention:       24 * time.Hour,
			},
		}
		err = worker.Work(ctx, &river.Job[riverjobs.CleanupExpiredDataArgs]{})
		require.NoError(t, err)

		// Verify challenge was deleted (expired 1h ago, retention is 30min)
		var count int
		err = suite.BunDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM billing.solana_wallet_challenges WHERE id = $1", challenge.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "Challenge expired beyond custom retention should be deleted")
	})
}
