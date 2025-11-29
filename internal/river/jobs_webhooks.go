package riverjobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/services"
)

const (
	KindWebhookProcess = "billing.webhook_process"
	KindWebhookRetry   = "billing.webhook_retry"
)

// WebhookProcessArgs processes a single webhook event by ID.
type WebhookProcessArgs struct {
	EventID uuid.UUID `json:"event_id"`
}

func (WebhookProcessArgs) Kind() string { return KindWebhookProcess }

// WebhookProcessWorker dispatches webhook events through the shared processor.
type WebhookProcessWorker struct {
	river.WorkerDefaults[WebhookProcessArgs]
	Processor *services.WebhookProcessor
}

func (w *WebhookProcessWorker) Work(ctx context.Context, job *river.Job[WebhookProcessArgs]) error {
	if w.Processor == nil {
		return fmt.Errorf("webhook processor not configured")
	}
	retry, err := w.Processor.Process(ctx, job.Args.EventID)
	if err != nil {
		log.WithContext(ctx).WithError(err).WithField("event_id", job.Args.EventID).Error("Webhook processing failed")
	}
	if retry {
		log.WithContext(ctx).WithField("event_id", job.Args.EventID).Info("Webhook scheduled for retry")
	}
	return nil
}

// WebhookRetryArgs triggers a scan for retryable webhook events.
type WebhookRetryArgs struct{}

func (WebhookRetryArgs) Kind() string { return KindWebhookRetry }

// WebhookRetryWorker finds pending/failed events ready for retry and processes them inline.
type WebhookRetryWorker struct {
	river.WorkerDefaults[WebhookRetryArgs]
	Events    *services.WebhookEventService
	Processor *services.WebhookProcessor
}

func (w *WebhookRetryWorker) Work(ctx context.Context, job *river.Job[WebhookRetryArgs]) error {
	if w.Events == nil || w.Processor == nil {
		return fmt.Errorf("webhook retry worker not configured")
	}

	events, err := w.Events.ListRetryable(ctx, 0)
	if err != nil {
		return fmt.Errorf("list retryable webhooks: %w", err)
	}

	for _, event := range events {
		if _, err := w.Processor.Process(ctx, event.ID); err != nil {
			log.WithContext(ctx).WithError(err).WithField("event_id", event.ID).Warn("Retrying webhook event failed")
		}
	}

	return nil
}
