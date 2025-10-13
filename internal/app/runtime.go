package app

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	redis "github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/mobius"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// Runtime aggregates infrastructure clients and application services.
type Runtime struct {
	DB               *db.DB
	RedisClient      *redis.Client
	Config           *config.Config
	CCBillClient     *ccbill.CCBillClient
	CCBillRESTClient *ccbill.RESTClient
	MobiusClient     *mobius.MobiusClient
	RiverClient      *river.Client[pgx.Tx]

	UserService              *services.UserService
	SubscriptionService      *services.SubscriptionService
	ProductService           *services.ProductService
	PriceService             *services.PriceService
	NotificationQueueService *services.NotificationQueueService
	NotificationService      *services.NotificationService
	PaymentMethodService     *services.PaymentMethodService
	PaymentService           *services.PaymentService

	UserSubscriptionService   *services.UserSubscriptionService
	PublicSubscriptionService *services.PublicSubscriptionService
	AdminSubscriptionService  *services.AdminSubscriptionService

	EmailService             *services.EmailService
	SubscriptionEmailService *services.SubscriptionEmailService

	BillingEventService *services.BillingEventService
	EntitlementService  *services.EntitlementService

	SolanaWalletService        *services.SolanaWalletService
	SolanaPaymentService       *services.SolanaPaymentService
	SolanaPaymentIntentService *services.SolanaPaymentIntentService

	SubscriptionLifecycleService *services.SubscriptionLifecycleService
}

// Close gracefully shuts down runtime resources.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.RiverClient != nil {
		log.Info("Stopping River background workers...")
		if err := r.RiverClient.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop River client: %w", err))
		}
	}
	if r.DB != nil {
		if err := r.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close db: %w", err))
		}
	}
	if r.BillingEventService != nil {
		if err := r.BillingEventService.Close(); err != nil {
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
	client, err := buildRiverClient(r.Config)
	if err != nil {
		return err
	}
	r.RiverClient = client
	return nil
}
