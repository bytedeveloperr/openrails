package app

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	riverjobs "github.com/doujins-org/doujins-billing/internal/river"
)

// buildRiverWorkers constructs the worker registry for River.
func (r *Runtime) buildRiverWorkers(ctx context.Context) (*river.Workers, error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &riverjobs.DunningWorker{DB: r.DB, NMIClients: r.NMIClients}); err != nil {
		return nil, fmt.Errorf("add dunning worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs.IdempotencyCleanupWorker{DB: r.DB}); err != nil {
		return nil, fmt.Errorf("add idempotency cleanup worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs.CCBillReconcileWorker{DB: r.DB, DataLink: r.CCBillDataLink}); err != nil {
		return nil, fmt.Errorf("add ccbill reconcile worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs.WebhookProcessWorker{Processor: r.WebhookProcessor}); err != nil {
		return nil, fmt.Errorf("add webhook process worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs.WebhookRetryWorker{Events: r.WebhookEventService, Processor: r.WebhookProcessor}); err != nil {
		return nil, fmt.Errorf("add webhook retry worker: %w", err)
	}
	return workers, nil
}

// buildRiverPeriodicJobs defines recurring schedules for workers using River periodic jobs.
func (r *Runtime) buildRiverPeriodicJobs(ctx context.Context) ([]*river.PeriodicJob, error) {
	var jobs []*river.PeriodicJob

	// Every 4 hours: run Dunning worker to process past_due subscriptions
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(4*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs.DunningArgs{}, &river.InsertOpts{
				Queue: riverjobs.QueueBilling,
			}
		},
		&river.PeriodicJobOpts{RunOnStart: false},
	))

	// Daily: Idempotency cleanup
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs.IdempotencyCleanupArgs{}, &river.InsertOpts{
				Queue: riverjobs.QueueBilling,
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	// Every 6 hours: CCBill reconcile
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(6*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs.CCBillReconcileArgs{}, &river.InsertOpts{
				Queue: riverjobs.QueueBilling,
			}
		},
		&river.PeriodicJobOpts{RunOnStart: false},
	))

	// Every minute: check for webhook retries
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs.WebhookRetryArgs{}, &river.InsertOpts{
				Queue: riverjobs.QueueBilling,
			}
		},
		&river.PeriodicJobOpts{RunOnStart: true},
	))

	return jobs, nil
}
