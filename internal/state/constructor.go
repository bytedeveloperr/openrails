package state

import (
	"context"
	"fmt"

	redis "github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/mobius"
	"github.com/doujins-org/doujins-billing/internal/services"
)

func NewState(cfg *config.Config) (*State, error) {
	db, err := createDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create db: %w", err)
	}

	redisClient, err := createRedisClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}

	ccbillClient := createCCBillClient(cfg)
	ccbillRESTClient := createCCBillRESTClient(cfg)
	// ccbillDataLinkClient := createCCBillDataLinkClient(cfg)
	mobiusClient, err := createMobiusClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create mobius client: %w", err)
	}

	// Build all repositories
	serviceInstances := createServices(db)

	// Assemble State
	state := &State{
		// Infrastructure
		DB:               db,
		RedisClient:      redisClient,
		Config:           cfg,
		CCBillClient:     ccbillClient,
		CCBillRESTClient: ccbillRESTClient,
		MobiusClient:     mobiusClient,

		// Servicesitories
		SubscriptionService: serviceInstances.SubscriptionService,
		UserService:         serviceInstances.UserService,

		// Wave 18 repositories
		ProductService:           serviceInstances.ProductService,
		PriceService:             serviceInstances.PriceService,
		NotificationQueueService: serviceInstances.NotificationQueueService,
		PaymentMethodService:     serviceInstances.PaymentMethodService,
		PaymentService:           serviceInstances.PurchaseService,
		EntitlementService:       serviceInstances.EntitlementService,
		SolanaWalletService:      serviceInstances.SolanaWalletService,
	}

	// Initialize optional analytics/event logging (ClickHouse)
	if cfg.ClickHouse != nil {
		if bes, err := services.NewBillingEventService(cfg.ClickHouse); err != nil {
			log.WithError(err).Warn("BillingEventService init failed; analytics disabled")
		} else {
			state.BillingEventService = bes
		}
	}

	return state, nil
}

// Infrastructure creation functions

func createDatabase(cfg *config.Config) (*db.DB, error) {
	db, err := db.NewDB(cfg.DB)
	if err != nil {
		return nil, err
	}

	// Register models
	models.RegisterModels(db.GetDB().(*bun.DB))
	return db, nil
}

func createRedisClient(cfg *config.Config) (*redis.Client, error) {
	redisOpts := &redis.Options{
		Addr: cfg.Redis.Addr,
		DB:   cfg.Redis.DB,
	}
	if cfg.Redis.Password != "" && cfg.Env == config.EnvProd {
		redisOpts.Password = cfg.Redis.Password
		log.Info("Redis authentication enabled")
	} else {
		log.Info("Redis authentication disabled - connecting without credentials")
	}
	client := redis.NewClient(redisOpts)
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 2_000_000_000) // 2s
	defer cancel()
	if _, err := client.Ping(ctx).Result(); err != nil {
		log.Warnf("Redis connection test failed: %v - rate limiting will fall back to permissive mode", err)
	} else {
		log.Info("Redis connection successful - rate limiting enabled")
	}
	return client, nil
}

func createCCBillClient(cfg *config.Config) *ccbill.CCBillClient {
	return ccbill.NewClient(cfg.CCBill, cfg.Env == config.EnvProd)
}

func createCCBillRESTClient(cfg *config.Config) *ccbill.RESTClient {
	return ccbill.NewRESTClient(cfg.CCBill)
}

func createCCBillDataLinkClient(cfg *config.Config) *ccbill.DataLinkClient {
	return ccbill.NewDataLinkClient(cfg.CCBill)
}

func createMobiusClient(cfg *config.Config) (*mobius.MobiusClient, error) {
	return mobius.NewClient(cfg.Mobius, cfg.Env == config.EnvProd)
}

type servicesInstances struct {
	SubscriptionService *services.SubscriptionService
	UserService         *services.UserService

	ProductService           *services.ProductService
	PriceService             *services.PriceService
	NotificationQueueService *services.NotificationQueueService
	PaymentMethodService     *services.PaymentMethodService
	PurchaseService          *services.PaymentService
	EntitlementService       *services.EntitlementService
	SolanaWalletService      *services.SolanaWalletService
}

func createServices(db *db.DB) *servicesInstances {
	return &servicesInstances{
		SubscriptionService: services.NewSubscriptionService(db),
		UserService:         services.NewUserService(db),

		// Wave 18 repositories
		ProductService:           services.NewProductService(db),
		PriceService:             services.NewPriceService(db),
		NotificationQueueService: services.NewNotificationQueueService(db),
		PaymentMethodService:     services.NewPaymentMethodService(db),
		PurchaseService:          services.NewPaymentService(db),
		EntitlementService:       services.NewEntitlementService(db),
		SolanaWalletService:      services.NewSolanaWalletService(db),
	}
}
