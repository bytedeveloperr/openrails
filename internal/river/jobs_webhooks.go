package riverjobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"
)

const (
	KindWebhookProcess = "billing.webhook_process"
	// QueueWebhooks could be separated later; for now reuse billing queue.
	QueueWebhooks = QueueBilling
)

// WebhookProcessArgs carries all data needed to process a webhook asynchronously.
type WebhookProcessArgs struct {
	Provider       string    `json:"provider"`
	EventID        string    `json:"event_id,omitempty"`
	EventType      string    `json:"event_type,omitempty"`
	Body           []byte    `json:"body"`
	ClientIP       string    `json:"client_ip,omitempty"`
	ReceivedAt     time.Time `json:"received_at,omitempty"`
	Signature      string    `json:"signature,omitempty"`
	SignatureValid *bool     `json:"signature_valid,omitempty"`
}

func (WebhookProcessArgs) Kind() string { return KindWebhookProcess }

// UniqueKey returns a deterministic dedupe key for this webhook.
func (a WebhookProcessArgs) UniqueKey() string {
	eventID := strings.TrimSpace(a.EventID)
	if eventID == "" {
		sum := sha256.Sum256(append([]byte(a.Provider+"|"+a.EventType+"|"), a.Body...))
		eventID = hex.EncodeToString(sum[:8])
	}
	return fmt.Sprintf("webhook:%s:%s", strings.TrimSpace(strings.ToLower(a.Provider)), eventID)
}

type WebhookProcessWorker struct {
	river.WorkerDefaults[WebhookProcessArgs]
	Dispatcher *services.WebhookDispatcher
}

func (WebhookProcessWorker) Kind() string { return KindWebhookProcess }

func (w WebhookProcessWorker) Work(ctx context.Context, job *river.Job[WebhookProcessArgs]) error {
	if w.Dispatcher == nil {
		return fmt.Errorf("webhook dispatcher not configured")
	}

	args := job.Args
	provider := strings.TrimSpace(strings.ToLower(args.Provider))
	if provider == "" {
		return fmt.Errorf("webhook provider required")
	}

	msg := &services.WebhookMessage{
		Processor:      provider,
		EventID:        strings.TrimSpace(args.EventID),
		EventType:      strings.TrimSpace(args.EventType),
		Payload:        args.Body,
		IPAddress:      strings.TrimSpace(args.ClientIP),
		Signature:      args.Signature,
		SignatureValid: args.SignatureValid,
		ReceivedAt:     args.ReceivedAt,
	}

	if err := w.Dispatcher.Process(ctx, msg); err != nil {
		log.WithContext(ctx).WithError(err).WithFields(log.Fields{
			"provider": provider,
			"event_id": msg.EventID,
			"kind":     KindWebhookProcess,
		}).Error("webhook processing failed")
		return err
	}

	log.WithContext(ctx).WithFields(log.Fields{
		"provider": provider,
		"event_id": msg.EventID,
	}).Info("webhook processed")
	return nil
}
