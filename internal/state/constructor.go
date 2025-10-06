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
	serviceInstances := createServices(db, cfg, ccbillRESTClient, mobiusClient)

	// Initialize optional email services
	var emailService *services.EmailService
	var subscriptionEmailService *services.SubscriptionEmailService
	if cfg.SendGrid != nil {
		if es, err := services.NewEmailService(cfg.SendGrid); err != nil {
			log.WithError(err).Warn("EmailService init failed; email disabled")
		} else {
			emailService = es
			subscriptionEmailService = services.NewSubscriptionEmailService(
				emailService,
				serviceInstances.SubscriptionService,
				serviceInstances.ProductService,
				serviceInstances.PriceService,
			)
		}
	}

	notificationService := services.NewNotificationService(
		serviceInstances.NotificationQueueService,
		subscriptionEmailService,
		emailService,
	)
	serviceInstances.SubscriptionLifecycleService.SetNotificationService(notificationService)
	serviceInstances.SolanaPaymentService.SetNotificationService(notificationService)

	// Assemble State
	state := &State{
		// Infrastructure
		DB:               db,
		RedisClient:      redisClient,
		Config:           cfg,
		CCBillClient:     ccbillClient,
		CCBillRESTClient: ccbillRESTClient,
		MobiusClient:     mobiusClient,

		// Services
		SubscriptionService: serviceInstances.SubscriptionService,
		UserService:         serviceInstances.UserService,

		ProductService:             serviceInstances.ProductService,
		PriceService:               serviceInstances.PriceService,
		NotificationQueueService:   serviceInstances.NotificationQueueService,
		NotificationService:        notificationService,
		PaymentMethodService:       serviceInstances.PaymentMethodService,
		PaymentService:             serviceInstances.PurchaseService,
		EntitlementService:         serviceInstances.EntitlementService,
		SolanaWalletService:        serviceInstances.SolanaWalletService,
		SolanaPaymentService:       serviceInstances.SolanaPaymentService,
		SolanaPaymentIntentService: serviceInstances.SolanaPaymentIntentService,

		// Wave 18 subscription services
		UserSubscriptionService:   serviceInstances.UserSubscriptionService,
		PublicSubscriptionService: serviceInstances.PublicSubscriptionService,
		AdminSubscriptionService:  serviceInstances.AdminSubscriptionService,

		// Wave 18 email services
		EmailService:                 emailService,
		SubscriptionEmailService:     subscriptionEmailService,
		SubscriptionLifecycleService: serviceInstances.SubscriptionLifecycleService,
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

	ProductService             *services.ProductService
	PriceService               *services.PriceService
	NotificationQueueService   *services.NotificationQueueService
	PaymentMethodService       *services.PaymentMethodService
	PurchaseService            *services.PaymentService
	EntitlementService         *services.EntitlementService
	SolanaWalletService        *services.SolanaWalletService
	SolanaPaymentService       *services.SolanaPaymentService
	SolanaPaymentIntentService *services.SolanaPaymentIntentService

	// Wave 18 subscription services
	UserSubscriptionService   *services.UserSubscriptionService
	PublicSubscriptionService *services.PublicSubscriptionService
	AdminSubscriptionService  *services.AdminSubscriptionService

	// Email services
	EmailService             *services.EmailService
	SubscriptionEmailService *services.SubscriptionEmailService

	SubscriptionLifecycleService *services.SubscriptionLifecycleService
}

func createServices(db *db.DB, cfg *config.Config, ccbillRESTClient *ccbill.RESTClient, mobiusClient *mobius.MobiusClient) *servicesInstances {
	// Create base services first
	userService := services.NewUserService(db)
	productService := services.NewProductService(db)
	priceService := services.NewPriceService(db)
	notificationQueueService := services.NewNotificationQueueService(db)
	paymentMethodService := services.NewPaymentMethodService(db)
	purchaseService := services.NewPaymentService(db)
	entitlementService := services.NewEntitlementService(db)
	solanaWalletService := services.NewSolanaWalletService(db)
	solanaPaymentService := services.NewSolanaPaymentService(db, cfg, priceService, purchaseService, productService, entitlementService, nil)
	solanaPaymentIntentService := services.NewSolanaPaymentIntentService(db, cfg, priceService)

	subscriptionLifecycleService := services.NewSubscriptionLifecycleService(
		db,
		productService,
		priceService,
		entitlementService,
		notificationQueueService,
	)

	// Create SubscriptionService with all its dependencies
	subscriptionService := services.NewSubscriptionService(
		db,
		priceService,
		productService,
		notificationQueueService,
		ccbillRESTClient,
		mobiusClient,
	)

	// Create Wave 18 subscription services that depend on base services
	userSubscriptionService := services.NewUserSubscriptionService(
		subscriptionService,
		productService,
		priceService,
		purchaseService,
		notificationQueueService,
		entitlementService,
		mobiusClient,
	)

	publicSubscriptionService := services.NewPublicSubscriptionService(
		productService,
		priceService,
	)

	adminSubscriptionService := services.NewAdminSubscriptionService(
		subscriptionService,
		productService,
		priceService,
		entitlementService,
		notificationQueueService,
		purchaseService,
	)

	return &servicesInstances{
		SubscriptionService: subscriptionService,
		UserService:         userService,

		// Wave 18 repositories
		ProductService:             productService,
		PriceService:               priceService,
		NotificationQueueService:   notificationQueueService,
		PaymentMethodService:       paymentMethodService,
		PurchaseService:            purchaseService,
		EntitlementService:         entitlementService,
		SolanaWalletService:        solanaWalletService,
		SolanaPaymentService:       solanaPaymentService,
		SolanaPaymentIntentService: solanaPaymentIntentService,

		// Wave 18 subscription services
		UserSubscriptionService:   userSubscriptionService,
		PublicSubscriptionService: publicSubscriptionService,
		AdminSubscriptionService:  adminSubscriptionService,

		// Email services will be set to nil initially - they require config
		EmailService:             nil,
		SubscriptionEmailService: nil,

		SubscriptionLifecycleService: subscriptionLifecycleService,
	}
}
