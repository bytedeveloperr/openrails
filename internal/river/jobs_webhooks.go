package riverjobs

// Webhook processing is now synchronous-only.
// This file previously contained WebhookProcessWorker and WebhookRetryWorker
// which have been removed as part of the webhook simplification.
//
// Rationale: Async webhook retry adds complexity without clear benefit.
// Payment processors (CCBill, NMI) will retry failed webhooks from their end.
// Processing webhooks synchronously provides immediate feedback and simpler error handling.
//
// See: agents/progress.json "simplify-webhook-processing" for details.
