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
	if r.RiverClient == nil {
		return nil, fmt.Errorf("river client is nil")
	}
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &riverjobs.DunningAttemptWorker{DB: r.DB, Mobius: r.MobiusClient}); err != nil {
		return nil, fmt.Errorf("add dunning attempt worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs.DunningSweepWorker{DB: r.DB, Mobius: r.MobiusClient}); err != nil {
		return nil, fmt.Errorf("add dunning sweep worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs.IdempotencyCleanupWorker{DB: r.DB}); err != nil {
		return nil, fmt.Errorf("add idempotency cleanup worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, &riverjobs.CCBillReconcileWorker{DB: r.DB, DataLink: r.CCBillDataLink}); err != nil {
		return nil, fmt.Errorf("add ccbill reconcile worker: %w", err)
	}
	return workers, nil
}

// buildRiverPeriodicJobs defines recurring schedules for workers using River periodic jobs.
func (r *Runtime) buildRiverPeriodicJobs(ctx context.Context) ([]*river.PeriodicJob, error) {
	if r.RiverClient == nil {
		return nil, fmt.Errorf("river client is nil")
	}

	var jobs []*river.PeriodicJob

	// Every minute: run DunningSweep
	jobs = append(jobs, river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			return riverjobs.DunningSweepArgs{}, &river.InsertOpts{
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

	return jobs, nil
}
