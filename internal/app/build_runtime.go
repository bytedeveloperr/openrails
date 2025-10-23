package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	redis "github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	repo "github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/nmi"
	"github.com/doujins-org/doujins-billing/internal/services"
)

func buildRuntime(cfg *config.Config) (*Runtime, error) {
	database, err := createDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create db: %w", err)
	}

	redisClient, err := createRedisClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}

	ccbillClient := createCCBillClient(cfg)
	ccbillRESTClient := createCCBillRESTClient(cfg)
	ccbillDataLinkClient := createCCBillDataLinkClient(cfg)
	nmiClients, err := createNMIClients(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create nmi clients: %w", err)
	}

	serviceInstances := createServices(database, cfg, ccbillRESTClient, nmiClients)
	workerManager := workers.NewManager(database, nmiClients, ccbillDataLinkClient, serviceInstances.SubscriptionService)

	var emailService *services.EmailService
	var subscriptionEmailService *services.SubscriptionEmailService
	if cfg.Email != nil {
		if es, err := services.NewEmailService(cfg.Email); err != nil {
			log.WithError(err).Warn("EmailService init failed; email disabled")
		} else {
			emailService = es
			subscriptionEmailService = services.NewSubscriptionEmailService(
				emailService,
				serviceInstances.SubscriptionService,
				serviceInstances.ProductService,
				serviceInstances.PriceService,
				repo.NewProfileRepo(database),
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

	runtime := &Runtime{
		DB:               database,
		RedisClient:      redisClient,
		Config:           cfg,
		CCBillClient:     ccbillClient,
		CCBillRESTClient: ccbillRESTClient,
		CCBillDataLink:   ccbillDataLinkClient,
		NMIClients:       nmiClients,

		SubscriptionService:        serviceInstances.SubscriptionService,
		UserService:                serviceInstances.UserService,
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

		UserSubscriptionService:   serviceInstances.UserSubscriptionService,
		PublicSubscriptionService: serviceInstances.PublicSubscriptionService,
		AdminSubscriptionService:  serviceInstances.AdminSubscriptionService,

		EmailService:                 emailService,
		SubscriptionEmailService:     subscriptionEmailService,
		SubscriptionLifecycleService: serviceInstances.SubscriptionLifecycleService,
	}

	// River client will be initialized later in StartWorkers with proper worker registration
	// if client, err := buildRiverClient(cfg); err != nil {
	// 	log.WithError(err).Warn("River client init failed; workers disabled")
	// } else {
	// 	runtime.RiverClient = client
	// }

	if cfg.ClickHouse != nil {
		if bes, err := services.NewBillingEventService(cfg.ClickHouse); err != nil {
			log.WithError(err).Warn("BillingEventService init failed; analytics disabled")
		} else {
			runtime.BillingEventService = bes
		}
	}

	return runtime, nil
}

func createDatabase(cfg *config.Config) (*db.DB, error) {
	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return nil, err
	}
	return database, nil
}

func createNMIClients(cfg *config.Config) (map[string]*nmi.NMIClient, error) {
	clients := make(map[string]*nmi.NMIClient)
	if cfg == nil || cfg.NMI == nil {
		return clients, nil
	}

	for name := range cfg.NMI.Providers {
		settings, err := cfg.NMI.ProviderSettings(name)
		if err != nil {
			return nil, err
		}
		providerKey := strings.TrimSpace(strings.ToLower(settings.Name))
		if providerKey == "" {
			providerKey = "mobius"
		}

		if _, exists := clients[providerKey]; exists {
			return nil, fmt.Errorf("duplicate nmi provider '%s' detected in configuration", providerKey)
		}

		client, err := nmi.NewClient(providerKey, settings, cfg.Env == config.EnvProd)
		if err != nil {
			return nil, err
		}
		clients[providerKey] = client
	}

	return clients, nil
}

func createRedisClient(cfg *config.Config) (*redis.Client, error) {
	if cfg.Redis == nil {
		return nil, nil
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
	if cfg.CCBill == nil {
		return nil
	}
	if cfg.CCBill.DataLinkUsername == "" || cfg.CCBill.DataLinkPassword == "" || cfg.CCBill.DataLinkClientAccNum == "" {
		log.Info("CCBill DataLink credentials missing; DataLink worker disabled")
		return nil
	}

	client := ccbill.NewDataLinkClient(cfg.CCBill)
	if err := client.ValidateConfig(); err != nil {
		log.WithError(err).Warn("Invalid CCBill DataLink configuration; worker disabled")
		return nil
	}
	return client
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

	UserSubscriptionService   *services.UserSubscriptionService
	PublicSubscriptionService *services.PublicSubscriptionService
	AdminSubscriptionService  *services.AdminSubscriptionService

	EmailService             *services.EmailService
	SubscriptionEmailService *services.SubscriptionEmailService

	SubscriptionLifecycleService *services.SubscriptionLifecycleService
}

func createServices(database *db.DB, cfg *config.Config, ccbillRESTClient *ccbill.RESTClient, nmiClients map[string]*nmi.NMIClient) *servicesInstances {
	userService := services.NewUserService(database)
	productService := services.NewProductService(database)
	priceService := services.NewPriceService(database)
	notificationQueueService := services.NewNotificationQueueService(database)
	paymentMethodService := services.NewPaymentMethodService(database)
	purchaseService := services.NewPaymentService(database)
	entitlementService := services.NewEntitlementService(database)
	solanaWalletService := services.NewSolanaWalletService(database)
	solanaPaymentService := services.NewSolanaPaymentService(database, cfg, priceService, purchaseService, productService, entitlementService, nil)
	solanaPaymentIntentService := services.NewSolanaPaymentIntentService(database, cfg, priceService)

	subscriptionLifecycleService := services.NewSubscriptionLifecycleService(
		database,
		productService,
		priceService,
		entitlementService,
		notificationQueueService,
	)

	subscriptionService := services.NewSubscriptionService(
		database,
		priceService,
		productService,
		notificationQueueService,
		ccbillRESTClient,
		nmiClients,
	)

	userSubscriptionService := services.NewUserSubscriptionService(
		subscriptionService,
		productService,
		priceService,
		purchaseService,
		notificationQueueService,
		entitlementService,
		nmiClients,
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
		SubscriptionService:          subscriptionService,
		UserService:                  userService,
		ProductService:               productService,
		PriceService:                 priceService,
		NotificationQueueService:     notificationQueueService,
		PaymentMethodService:         paymentMethodService,
		PurchaseService:              purchaseService,
		EntitlementService:           entitlementService,
		SolanaWalletService:          solanaWalletService,
		SolanaPaymentService:         solanaPaymentService,
		SolanaPaymentIntentService:   solanaPaymentIntentService,
		UserSubscriptionService:      userSubscriptionService,
		PublicSubscriptionService:    publicSubscriptionService,
		AdminSubscriptionService:     adminSubscriptionService,
		SubscriptionLifecycleService: subscriptionLifecycleService,
	}
}

func buildRiverClient(cfg *config.Config, workers *river.Workers) (*river.Client[pgx.Tx], error) {
	if cfg.DB == nil || cfg.DB.URL == "" {
		return nil, fmt.Errorf("missing database configuration for River")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.DB.URL)
	if err != nil {
		return nil, fmt.Errorf("failed creating pgx pool for River: %w", err)
	}
	drv := riverpgxv5.New(pool)
	client, err := river.NewClient[pgx.Tx](drv, &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
			"billing":          {MaxWorkers: 20},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("failed creating River client: %w", err)
	}
	return client, nil
}
