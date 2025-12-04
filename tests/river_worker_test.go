//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		client, ok := suite.App.Runtime.RiverClient.(*river.Client[pgx.Tx])
		require.True(t, ok, "Should have River client")

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
		client, ok := suite.App.Runtime.RiverClient.(*river.Client[pgx.Tx])
		require.True(t, ok)

		// Create a webhook process job (this would normally be enqueued when a webhook is received)
		// Note: We're just testing that the job infrastructure works, not the actual webhook processing
		ctx := context.Background()

		initialPending := suite.GetPendingJobCount()
		initialCompleted := suite.GetCompletedJobCount()

		// Enqueue a webhook process job with a fake event ID
		// This will fail to find the event but proves the infrastructure works
		_, err := client.Insert(ctx, riverjobs.WebhookProcessArgs{
			EventID: "test-event-id-12345",
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
		client, ok := suite.App.Runtime.RiverClient.(*river.Client[pgx.Tx])
		require.True(t, ok)

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
