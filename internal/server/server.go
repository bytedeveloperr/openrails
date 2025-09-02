package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/mobius"
	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/cache"
)

// Server represents the billing service server
type Server struct {
	cfg *config.Config

	// Database and cache
	db    *db.DB
	cache cache.Cache

	// External integrations
	mobiusClient *mobius.MobiusClient
	ccbillClient *ccbill.CCBillClient

	// Services
	subscriptionService *services.SubscriptionService

	// HTTP handlers
	publicHandler  http.Handler
	privateHandler http.Handler
}

// New creates a new billing service server
func New(cfg *config.Config) (*Server, error) {
	log.Info("Initializing billing service...")

	// Validate billing configuration
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid billing configuration: %w", err)
	}

	// Initialize database
	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize cache
	var cacheClient cache.Cache
	if cfg.Redis != nil {
		// Create Redis client from config
		redisClient := redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		cacheClient = cache.NewRedisCache(redisClient)
	} else {
		log.Warn("Redis not configured, using in-memory cache")
		cacheClient = cache.NewMemoryCache()
	}

	// Initialize payment integrations
	isProd := cfg.Env == "production" || cfg.Env == "prod"

	mobiusClient, err := mobius.NewClient(cfg.Mobius, isProd)
	if err != nil {
		return nil, err
	}

	ccbillClient := ccbill.NewClient(cfg.CCBill, isProd)

	// Initialize services
	subscriptionService := services.NewSubscriptionService(database)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:                 cfg,
		db:                  database,
		cache:               cacheClient,
		mobiusClient:        mobiusClient,
		ccbillClient:        ccbillClient,
		subscriptionService: subscriptionService,
	}

	// Initialize HTTP handlers
	s.setupHandlers()

	log.Info("Billing service initialized successfully")
	return s, nil
}

// setupHandlers initializes the HTTP handlers for both public and private listeners
func (s *Server) setupHandlers() {
	// Create public router (frontend and webhooks)
	publicRouter := gin.New()
	publicRouter.Use(gin.Recovery())
	publicRouter.Use(middleware.Logger())
	publicRouter.Use(middleware.CORS(s.cfg.CorsOrigins))
	publicRouter.Use(middleware.RateLimit(s.cfg.RateLimits))

	// Create private router (api-server communication)
	privateRouter := gin.New()
	privateRouter.Use(gin.Recovery())
	privateRouter.Use(middleware.Logger())
	privateRouter.Use(middleware.InternalOnly()) // Restrict to internal network

	// Initialize handlers
	subscriptionHandler := handlers.NewSubscriptionHandler(s.subscriptionService, nil)
	adminHandler := handlers.NewAdminHandler(s.subscriptionService)

	s.setupPublicRoutes(publicRouter, subscriptionHandler, nil, nil, nil)
	s.setupPrivateRoutes(privateRouter, adminHandler, subscriptionHandler)

	s.publicHandler = publicRouter
	s.privateHandler = privateRouter
}

// setupPublicRoutes configures routes for the public listener
func (s *Server) setupPublicRoutes(router *gin.Engine, subscriptionHandler *handlers.SubscriptionHandler, paymentMethodHandler *handlers.PaymentMethodHandler, solanaHandler *handlers.SolanaHandler, webhookHandler *handlers.WebhookHandler) {
	api := router.Group("/api/v1")

	// Subscription routes (authenticated)
	subscriptions := api.Group("/subscriptions")
	subscriptions.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		subscriptions.POST("/:processor", subscriptionHandler.Subscribe)
		subscriptions.POST("/ccbill/flexform-url", subscriptionHandler.GenerateFlexFormURL)
		subscriptions.GET("/active", subscriptionHandler.GetActiveSubscription)
		subscriptions.GET("/:id/status", subscriptionHandler.GetSubscriptionStatus)
		subscriptions.GET("/history", subscriptionHandler.GetSubscriptionHistory)
	}

	// Payment methods (authenticated)
	paymentMethods := api.Group("/user/payment-methods")
	paymentMethods.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		paymentMethods.POST("", paymentMethodHandler.CreatePaymentMethod)
		paymentMethods.GET("", paymentMethodHandler.ListPaymentMethods)
		paymentMethods.PUT("/:id", paymentMethodHandler.UpdatePaymentMethod)
		paymentMethods.POST("/:id/activate", paymentMethodHandler.ActivatePaymentMethod)
		paymentMethods.DELETE("/:id", paymentMethodHandler.DeletePaymentMethod)
	}

	// Solana payment routes
	solana := api.Group("/payment/solana")
	{
		// Public routes
		solana.GET("/tokens", solanaHandler.GetSupportedTokens)
		solana.GET("/qr", solanaHandler.GenerateQR)

		// Authenticated routes
		authenticated := solana.Group("")
		authenticated.Use(middleware.AuthRequired(s.cfg.JWT))
		{
			authenticated.POST("/generate", solanaHandler.GenerateTransaction)
			authenticated.POST("/submit", solanaHandler.SubmitTransaction)
		}
	}

	// Webhook routes (public, but with signature verification)
	webhooks := api.Group("/subscriptions/webhook")
	{
		webhooks.POST("/:processor", webhookHandler.ProcessWebhook)
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})
}

// setupPrivateRoutes configures routes for the private listener (api-server only)
func (s *Server) setupPrivateRoutes(router *gin.Engine, adminHandler *handlers.AdminHandler, subscriptionHandler *handlers.SubscriptionHandler) {
	api := router.Group("/api/v1")

	// Admin-only routes
	admin := api.Group("")
	admin.Use(middleware.AdminRequired())
	{
		admin.PUT("/subscriptions/:id/extend", adminHandler.ExtendSubscription)
		admin.POST("/subscriptions/:id/cancel", adminHandler.CancelSubscription)
		admin.GET("/subscriptions/:id/details", adminHandler.GetSubscriptionDetails)
		admin.POST("/subscriptions/:id/refund", adminHandler.ProcessRefund)
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing-private"})
	})
}

// PublicHandler returns the public HTTP handler
func (s *Server) PublicHandler() http.Handler {
	return s.publicHandler
}

// PrivateHandler returns the private HTTP handler
func (s *Server) PrivateHandler() http.Handler {
	return s.privateHandler
}

// Close performs cleanup when shutting down the server
func (s *Server) Close(ctx context.Context) error {
	var errs []error

	// Close database connections
	if err := s.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close database: %w", err))
	}

	// Close cache connections
	if err := s.cache.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close cache: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during cleanup: %v", errs)
	}

	log.Info("Billing service cleanup completed")
	return nil
}
