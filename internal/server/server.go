package server

import (
    "context"
    "fmt"
    "net/http"

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

    s := &Server{
        cfg:                 cfg,
        cache:               cacheClient,
        state:               st,
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
        Use(middleware.RateLimit(s.cfg.RateLimits))

    // Admin handler (internal only, protected by API key)
    s.adminHandler = gin.New()
    s.adminHandler.Use(gin.Recovery())
    s.adminHandler.Use(middleware.InternalOnly(s.cfg.Admin))
}

func (s *Server) setupPublicRoutes() {
    api := s.publicHandler.Group("/api/v1")

    // Public subscription data (no auth)
    publicSubs := api.Group("/subscriptions/public")
    {
        publicSubs.GET("/products", s.wrap(handlers.GetProducts))
        publicSubs.GET("/subscribe-page-data", s.wrap(handlers.GetSubscribePageData))
    }

    subscriptions := api.Group("/subscriptions")
    subscriptions.Use(middleware.AuthRequired(s.cfg.JWT))
    {
        // Avoid wildcard conflict with admin routes by namespacing processor
        subscriptions.POST("/processor/:processor", s.wrap(handlers.Subscribe))
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

    s.publicHandler.GET("/health", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing-private"})
    })
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
    }
    s.adminHandler.GET("/health", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing-admin"})
    })
}

func (s *Server) wrap(fn func(r *handlers.Request)) func(c *gin.Context) {
	return func(c *gin.Context) {
		fn(handlers.NewRequest(c, s.state))
	}
}

func (s *Server) Handler() http.Handler { return s.publicHandler }
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
