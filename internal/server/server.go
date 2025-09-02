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
	"github.com/doujins-org/doujins-billing/internal/state"
	"github.com/doujins-org/doujins-billing/pkg/cache"
)

// Server represents the billing service server
type Server struct {
	cfg *config.Config

	// Database and cache
	db    *db.DB
	cache cache.Cache

	state *state.State

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
			Addr:     cfg.Redis.Addr,
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

	s.publicHandler = publicRouter
	s.privateHandler = privateRouter
}

// setupPublicRoutes configures routes for the public listener
func (s *Server) setupPublicRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")

	// Subscription routes (authenticated)
	subscriptions := api.Group("/subscriptions")
	subscriptions.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		subscriptions.POST("/:processor", s.wrap(handlers.Subscribe))
		subscriptions.POST("/ccbill/flexform-url", s.wrap(handlers.GenerateFlexFormURL))
		subscriptions.GET("/active", s.wrap(handlers.GetSubscription))
		subscriptions.GET("/history", s.wrap(handlers.GetSubscriptionHistory))
	}

	// Webhook routes (public, but with signature verification)
	webhooks := api.Group("/subscriptions/webhook")
	{
		webhooks.POST("/:processor", s.wrap(handlers.Webhook))
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})
}

// setupPrivateRoutes configures routes for the private listener (api-server only)
func (s *Server) setupPrivateRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")

	// Admin-only routes
	admin := api.Group("")
	admin.Use(middleware.AdminRequired())
	{
		admin.PUT("/subscriptions/:id/extend", s.wrap(handlers.ExtendSubscription))
		admin.POST("/subscriptions/:id/cancel", s.wrap(handlers.CancelSubscription))
		admin.GET("/subscriptions/:id/details", s.wrap(handlers.GetSubscription))

	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing-private"})
	})
}

func (s *Server) wrap(fn func(r *handlers.Request)) func(c *gin.Context) {
	return func(c *gin.Context) {
		fn(handlers.NewRequest(c, s.state))
	}
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
