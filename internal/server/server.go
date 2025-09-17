package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/state"
	"github.com/doujins-org/doujins-billing/pkg/cache"
)

type Server struct {
	cfg *config.Config

	cache cache.Cache

	state *state.State

	rdb *redis.Client

	publicHandler *gin.Engine
	adminHandler  *gin.Engine
}

func New(cfg *config.Config) (*Server, error) {
	log.Info("Initializing billing service...")

	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid billing configuration: %w", err)
	}

	// Build shared state (DB, services, integrations)
	st, err := state.NewState(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize state: %w", err)
	}

	// Dynamic Redis circuit: prefer Redis if reachable; otherwise in-memory. Swap live on health changes.
	memoryCache := cache.NewMemoryCache()
	var redisClient *redis.Client
	var redisCache cache.Cache
	if cfg.Redis != nil {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		redisCache = cache.NewRedisCache(redisClient)
	}
	switchable := cache.NewSwitchableCache(memoryCache)
	// Initial probe
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if _, err := redisClient.Ping(ctx).Result(); err == nil {
			switchable.SetBackend(redisCache)
			log.Info("Redis available: using Redis cache")
		} else {
			log.WithError(err).Warn("Redis unavailable at startup: using in-memory cache")
		}
		cancel()
		// Health monitor
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			usingRedis := false
			for range ticker.C {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, err := redisClient.Ping(ctx).Result()
				cancel()
				if err == nil {
					if !usingRedis {
						switchable.SetBackend(redisCache)
						usingRedis = true
						log.Info("Redis became available: switched to Redis cache")
					}
				} else {
					if usingRedis {
						switchable.SetBackend(memoryCache)
						usingRedis = false
						log.WithError(err).Warn("Redis lost: falling back to in-memory cache")
					}
				}
			}
		}()
	} else {
		log.Warn("Redis not configured: using in-memory cache")
	}

	s := &Server{
		cfg:   cfg,
		cache: switchable,
		state: st,
		rdb:   redisClient,
	}

	s.setupHandlers()
	s.setupPublicRoutes()
	s.setupAdminRoutes()

	log.Info("Billing service initialized successfully")
	return s, nil
}

func (s *Server) setupHandlers() {
	// Public handler
	s.publicHandler = gin.Default()
	s.publicHandler.
		Use(middleware.CORS(s.cfg.CorsOrigins)).
		Use(middleware.RateLimit(s.cfg.RateLimits, s.rdb))

	// Admin handler (internal only, protected by API key)
	s.adminHandler = gin.New()
	s.adminHandler.Use(gin.Recovery())
	s.adminHandler.Use(middleware.InternalOnly(s.cfg.Admin))
}

func (s *Server) setupPublicRoutes() {
	api := s.publicHandler.Group("/api/v1")

	// Public subscription data (no auth)
	subscriptions := api.Group("/subscriptions")
	{
		subscriptions.GET("/products", s.wrap(handlers.GetProducts))
		subscriptions.GET("/page-data", s.wrap(handlers.GetSubscribePageData))
	}

	subscriptions.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		// Avoid wildcard conflict with admin routes by namespacing processor
		subscriptions.POST("/process/:processor", s.wrap(handlers.Subscribe))
		subscriptions.POST("/ccbill/flexform-url", s.wrap(handlers.GenerateFlexFormURL))
		subscriptions.POST("/cancel", s.wrap(handlers.CancelSubscription))
		subscriptions.GET("/active", s.wrap(handlers.GetSubscription))
		subscriptions.GET("/history", s.wrap(handlers.GetSubscriptionHistory))
		subscriptions.GET("/purchases", s.wrap(handlers.GetUserPurchases))
	}

	webhooks := api.Group("/subscriptions/webhook")
	{
		webhooks.POST("/:processor", s.wrap(handlers.Webhook))
	}

	// Payment methods
	pms := api.Group("/payment-methods")
	pms.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		pms.GET("", s.wrap(handlers.ListPaymentMethods))
		pms.DELETE(":id", s.wrap(handlers.DeletePaymentMethod))
		pms.PUT(":id/activate", s.wrap(handlers.ActivatePaymentMethod))
	}

	// Notifications
	notifications := api.Group("/notifications")
	notifications.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		notifications.GET("", s.wrap(handlers.GetNotifications))
		notifications.GET("/unread-count", s.wrap(handlers.GetUnreadNotificationCount))
		notifications.POST(":id/read", s.wrap(handlers.MarkNotificationRead))
	}

	// Wallet (Solana) - scaffold endpoints
	wallet := api.Group("/wallet/solana")
	wallet.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		wallet.GET("", s.wrap(handlers.ListSolanaWallets))
		wallet.POST("/connect", s.wrap(handlers.ConnectSolanaWallet))
		wallet.POST("/challenge", s.wrap(handlers.GenerateSolanaWalletChallenge))
		wallet.POST("/verify", s.wrap(handlers.VerifySolanaWallet))
		wallet.DELETE("", s.wrap(handlers.DeleteSolanaWallet))
	}

	// Solana payments (generate transaction and QR)
	solana := api.Group("/solana")
	solana.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		solana.POST("/generate", s.wrap(handlers.GeneratePayment))
		solana.POST("/submit", s.wrap(handlers.SubmitPayment))
		solana.POST("/qr", s.wrap(handlers.GenerateSolanaPayQR))
		solana.GET("/supported-tokens", s.wrap(handlers.GetSupportedTokens))
	}

	// Kubernetes-style health endpoints
	s.publicHandler.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})

	s.publicHandler.GET("/health/ready", func(c *gin.Context) {
		// Readiness: check DB and Redis. ClickHouse is optional.
		ctx := c.Request.Context()
		// DB check
		var one int
		if s.state != nil && s.state.DB != nil {
			if err := s.state.DB.GetDB().NewSelect().ColumnExpr("1").Scan(ctx, &one); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "db": "down"})
				return
			}
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "db": "missing"})
			return
		}
		// Redis check (optional but recommended)
		if s.state != nil && s.state.RedisClient != nil {
			if _, err := s.state.RedisClient.Ping(ctx).Result(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "redis": "down"})
				return
			}
		}
		// ClickHouse: optional; readiness does not require it.
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// Me: consolidated billing status
	me := api.Group("/me")
	me.Use(middleware.AuthRequired(s.cfg.JWT))
	{
		me.GET("/billing-status", s.wrap(handlers.GetMyBillingStatus))
	}
}

func (s *Server) setupAdminRoutes() {
	api := s.adminHandler.Group("/api/v1")
	{
		api.PUT("/subscriptions/:id/extend", s.wrap(handlers.ExtendSubscription))
		api.POST("/subscriptions/:id/cancel", s.wrap(handlers.CancelSubscription))
		api.GET("/subscriptions/:id/details", s.wrap(handlers.GetSubscription))

		// Analytics for admin dashboard (private)
		api.GET("/subscriptions/dashboard-metrics", s.wrap(handlers.GetAdminDashboardMetrics))
		api.GET("/subscriptions/daily-metrics", s.wrap(handlers.GetAdminDailyMetrics))
		api.GET("/subscriptions/processor-metrics", s.wrap(handlers.GetAdminProcessorMetrics))

		// Entitlements: list active windows for a user
		api.GET("/users/:user_id/entitlements", s.wrap(handlers.GetAdminActiveEntitlements))

	}
	s.adminHandler.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing-admin"})
	})
	s.adminHandler.HEAD("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}

func (s *Server) wrap(fn func(r *handlers.Request)) func(c *gin.Context) {
	return func(c *gin.Context) {
		fn(handlers.NewRequest(c, s.state))
	}
}

func (s *Server) Handler() http.Handler      { return s.publicHandler }
func (s *Server) AdminHandler() http.Handler { return s.adminHandler }

func (s *Server) Close(ctx context.Context) error {
	var errs []error
	// Close state (includes River, DB, services)
	if s.state != nil {
		if err := s.state.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to close state: %w", err))
		}
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

// StartWorkers starts River background workers within this server process.
func (s *Server) StartWorkers(ctx context.Context) {
	if s.state == nil {
		log.Warn("No state available; skipping worker startup")
		return
	}
	if err := s.state.InitRiver(ctx); err != nil {
		log.WithError(err).Error("Failed to initialize River client")
		return
	}
	go func() {
		log.Info("Starting River background workers in-server")
		if err := s.state.RiverClient.Start(ctx); err != nil {
			log.WithError(err).Error("River workers stopped with error")
		} else {
			log.Info("River workers stopped")
		}
	}()
}

func (s *Server) Cfg() *config.Config {
	return s.cfg
}
