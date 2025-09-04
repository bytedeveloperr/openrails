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

type Server struct {
	cfg *config.Config

	db    *db.DB
	cache cache.Cache

	state *state.State

	mobiusClient *mobius.MobiusClient
	ccbillClient *ccbill.CCBillClient

	subscriptionService *services.SubscriptionService
	handler             *gin.Engine
}

func New(cfg *config.Config) (*Server, error) {
	log.Info("Initializing billing service...")

	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid billing configuration: %w", err)
	}

	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	var cacheClient cache.Cache
	if cfg.Redis != nil {
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

	isProd := cfg.Env == "production" || cfg.Env == "prod"
	mobiusClient, err := mobius.NewClient(cfg.Mobius, isProd)
	if err != nil {
		return nil, err
	}

	ccbillClient := ccbill.NewClient(cfg.CCBill, isProd)

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

	s.setupHandler()

	log.Info("Billing service initialized successfully")
	return s, nil
}

func (s *Server) setupHandler() {
	s.handler = gin.Default()
	s.handler.
		Use(middleware.CORS(s.cfg.CorsOrigins)).
		Use(middleware.RateLimit(s.cfg.RateLimits))
}

func (s *Server) Setup() {
	api := s.handler.Group("/api/v1")

	subscriptions := api.Group("/subscriptions")
	subscriptions.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		subscriptions.POST("/:processor", s.wrap(handlers.Subscribe))
		subscriptions.POST("/ccbill/flexform-url", s.wrap(handlers.GenerateFlexFormURL))
		subscriptions.GET("/active", s.wrap(handlers.GetSubscription))
		subscriptions.GET("/history", s.wrap(handlers.GetSubscriptionHistory))
	}

	admin := api.Group("")
	admin.Use(middleware.AdminRequired())
	{
		admin.PUT("/subscriptions/:id/extend", s.wrap(handlers.ExtendSubscription))
		admin.POST("/subscriptions/:id/cancel", s.wrap(handlers.CancelSubscription))
		admin.GET("/subscriptions/:id/details", s.wrap(handlers.GetSubscription))

	}

	webhooks := api.Group("/subscriptions/webhook")
	{
		webhooks.POST("/:processor", s.wrap(handlers.Webhook))
	}

	s.handler.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing-private"})
	})
}

func (s *Server) wrap(fn func(r *handlers.Request)) func(c *gin.Context) {
	return func(c *gin.Context) {
		fn(handlers.NewRequest(c, s.state))
	}
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Close(ctx context.Context) error {
	var errs []error
	if err := s.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close database: %w", err))
	}

	if err := s.cache.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close cache: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during cleanup: %v", errs)
	}

	log.Info("Billing service cleanup completed")
	return nil
}
