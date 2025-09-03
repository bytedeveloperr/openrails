package state

import (
	"context"
	"fmt"

	"github.com/go-redis/redis"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/mobius"
	"github.com/doujins-org/doujins-billing/internal/services"
)

type State struct {
	DB               *db.DB
	RedisClient      *redis.Client
	Config           *config.Config
	CCBillClient     *ccbill.CCBillClient
	CCBillRESTClient *ccbill.RESTClient
	MobiusClient     *mobius.MobiusClient
	RiverClient      *river.Client[pgx.Tx]

	UserService              *services.UserService
	SubscriptionService      *services.SubscriptionService
	UserRoleGrantService     *services.UserRoleGrantService
	UserRoleInterfaceService *services.UserRoleInterfaceService
	ProductService           *services.ProductService
	PriceService             *services.PriceService
	NotificationQueueService *services.NotificationQueueService
	PaymentMethodService     *services.PaymentMethodService
	PaymentService           *services.PaymentService
	// removed: WebhookEventProcessedService (replaced by idempotency store)

	// Wave 18 subscription services
	UserSubscriptionService   *services.UserSubscriptionService
	PublicSubscriptionService *services.PublicSubscriptionService
	AdminSubscriptionService  *services.AdminSubscriptionService

	// Wave 18 email service
	EmailService             *services.EmailService
	SubscriptionEmailService *services.SubscriptionEmailService

	// Billing event tracking service
	BillingEventService *services.BillingEventService
}

// Close gracefully shuts down all state resources
func (s *State) Close(ctx context.Context) error {
	var errs []error

	// Stop River client if it exists
	if s.RiverClient != nil {
		log.Info("Stopping River background workers...")
		if err := s.RiverClient.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop River client: %w", err))
		}
	}

	// Close db
	if s.DB != nil {
		if err := s.DB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close db: %w", err))
		}
	}

	// Close billing event service
	if s.BillingEventService != nil {
		if err := s.BillingEventService.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close billing event service: %w", err))
		}
	}

	// Close Redis client
	if s.RedisClient != nil {
		if err := s.RedisClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Redis client: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close some resources: %v", errs)
	}

	return nil
}
