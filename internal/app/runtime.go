package app

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	redis "github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/nmi"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/jonboulle/clockwork"
)

// Runtime aggregates infrastructure clients and application services.
type Runtime struct {
	DB               *db.DB
	RedisClient      *redis.Client
	Config           *config.Config
	Clock            clockwork.Clock
	CCBillClient     *ccbill.CCBillClient
	CCBillRESTClient *ccbill.RESTClient
	CCBillDataLink   *ccbill.DataLinkClient
	NMIClients       map[string]*nmi.NMIClient
	RiverClient      *river.Client[pgx.Tx]
	riverPool        *pgxpool.Pool

	SubscriptionService  *services.SubscriptionService
	ProductService       *services.ProductService
	PriceService         *services.PriceService
	NotificationService  *services.NotificationService
	PaymentMethodService *services.PaymentMethodService
	PaymentService       *services.PaymentService
	VaultService         *services.VaultService

	UserSubscriptionService   *services.UserSubscriptionService
	PublicSubscriptionService *services.PublicSubscriptionService
	AdminSubscriptionService  *services.AdminSubscriptionService

	EmailService *services.EmailService

	EventLogService    *services.EventLogService
	EntitlementService *services.EntitlementService
	AdminGrantService  *services.AdminGrantService

	SolanaWalletService        *services.SolanaWalletService
	SolanaPaymentService       *services.SolanaPaymentService
	SolanaPaymentIntentService *services.SolanaPaymentIntentService
	SolanaVerificationService  *services.SolanaVerificationService
	SolanaPayService           *services.SolanaPayService
	SolanaPayPoller            *services.SolanaPayPoller

	SubscriptionLifecycleService *services.SubscriptionLifecycleService
	WebhookEventService          *services.WebhookEventService
	WebhookDispatcher            *services.WebhookDispatcher
	DeduplicationService         *services.DeduplicationService
	WebhookProcessor             *services.WebhookProcessor

	CheckoutService *services.CheckoutService

	riverStarted bool
}

// Close gracefully shuts down runtime resources.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error

	// Stop Solana Pay poller
	if r.SolanaPayPoller != nil {
		log.Info("Stopping Solana Pay poller...")
		r.SolanaPayPoller.Stop()
	}

	if r.RiverClient != nil && r.riverStarted {
		log.Info("Stopping River background workers...")
		if err := r.RiverClient.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop River client: %w", err))
		}
		r.riverStarted = false
	}
	if r.riverPool != nil {
		r.riverPool.Close()
		r.riverPool = nil
	}
	if r.DB != nil {
		if err := r.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close db: %w", err))
		}
	}
	if r.EventLogService != nil {
		if err := r.EventLogService.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close billing event service: %w", err))
		}
	}
	if r.RedisClient != nil {
		if err := r.RedisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Redis client: %w", err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("failed to close some resources: %v", errs)
}

// InitRiver initialises the River client for background workers.
func (r *Runtime) InitRiver(ctx context.Context) error {
	if r.RiverClient != nil {
		return nil
	}
	workers, err := r.buildRiverWorkers(ctx)
	if err != nil {
		return fmt.Errorf("build river workers: %w", err)
	}
	client, pool, err := buildRiverClient(r.Config, workers)
	if err != nil {
		return err
	}
	r.RiverClient = client
	r.riverPool = pool
	return nil
}

// StartWorkers spins up background workers using the River queue system.
func (r *Runtime) StartWorkers(ctx context.Context) {
	if r == nil {
		return
	}
	if !r.riverStarted {
		if err := r.InitRiver(ctx); err != nil {
			log.WithError(err).Error("Failed to initialize River client")
			return
		}
		if r.RiverClient != nil {
			// Build periodic jobs
			periodicJobs, err := r.buildRiverPeriodicJobs(ctx)
			if err != nil {
				log.WithError(err).Error("Failed to configure River periodic jobs")
				return
			}
			// Add periodic jobs to the client
			for _, job := range periodicJobs {
				r.RiverClient.PeriodicJobs().Add(job)
			}

			r.riverStarted = true
			go func() {
				log.Info("Starting River background workers in-server")
				if err := r.RiverClient.Start(ctx); err != nil {
					log.WithError(err).Error("River workers stopped with error")
				} else {
					log.Info("River workers stopped")
				}
			}()
		}
	}

	// Start Solana Pay poller if configured
	if r.SolanaPayPoller != nil && r.Config.Solana != nil {
		go r.SolanaPayPoller.Start(ctx)
	}
}
