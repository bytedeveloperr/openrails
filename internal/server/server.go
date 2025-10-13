package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/auth"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/state"
	"github.com/doujins-org/doujins-billing/pkg/cache"
)

type Dependencies struct {
	Config       *config.Config
	Cache        cache.Cache
	State        *state.State
	Redis        *redis.Client
	AuthVerifier auth.Verifier
}

type Server struct {
	cfg          *config.Config
	cache        cache.Cache
	state        *state.State
	rdb          *redis.Client
	authVerifier auth.Verifier

	publicHandler *gin.Engine
	adminHandler  *gin.Engine
}

func New(deps Dependencies) (*Server, error) {
	if deps.Config == nil {
		return nil, fmt.Errorf("server config is required")
	}
	if deps.State == nil {
		return nil, fmt.Errorf("server state is required")
	}
	if deps.Cache == nil {
		return nil, fmt.Errorf("server cache is required")
	}
	if deps.AuthVerifier == nil {
		return nil, fmt.Errorf("auth verifier is required")
	}

	s := &Server{
		cfg:          deps.Config,
		cache:        deps.Cache,
		state:        deps.State,
		rdb:          deps.Redis,
		authVerifier: deps.AuthVerifier,
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
	api := s.publicHandler.Group("/v1")

	// Public subscription data (no auth)
	subscriptions := api.Group("/subscriptions")
	{
		subscriptions.GET("/products", s.wrap(handlers.GetProducts))
		subscriptions.GET("/page-data", s.wrap(handlers.GetSubscribePageData))
	}

	subscriptions.Use(middleware.AuthRequired(s.authVerifier))
	{
		// Avoid wildcard conflict with admin routes by namespacing processor
		subscriptions.POST("/process/:processor", s.wrap(handlers.Subscribe))
		subscriptions.POST("/ccbill/flexform-url", s.wrap(handlers.GenerateFlexFormURL))
		subscriptions.POST("/cancel", s.wrap(handlers.CancelSubscription))
		subscriptions.GET("/active", s.wrap(handlers.GetSubscription))
		subscriptions.GET("/history", s.wrap(handlers.GetSubscriptionHistory))
		subscriptions.GET("/purchases", s.wrap(handlers.GetUserPayments))
	}

	webhooks := api.Group("/subscriptions/webhook")
	{
		webhooks.POST("/:processor", s.wrap(handlers.Webhook))
	}

	// Payment methods
	pms := api.Group("/payment-methods")
	pms.Use(middleware.AuthRequired(s.authVerifier))
	{
		pms.GET("", s.wrap(handlers.ListPaymentMethods))
		pms.DELETE(":id", s.wrap(handlers.DeletePaymentMethod))
		pms.PUT(":id/activate", s.wrap(handlers.ActivatePaymentMethod))
	}

	// Notifications
	notifications := api.Group("/notifications")
	notifications.Use(middleware.AuthRequired(s.authVerifier))
	{
		notifications.GET("", s.wrap(handlers.GetNotifications))
		notifications.GET("/unread-count", s.wrap(handlers.GetUnreadNotificationCount))
		notifications.POST(":id/read", s.wrap(handlers.MarkNotificationRead))
	}

	// Wallet (Solana) - scaffold endpoints
	wallet := api.Group("/wallet/solana")
	wallet.Use(middleware.AuthRequired(s.authVerifier))
	{
		wallet.GET("", s.wrap(handlers.ListSolanaWallets))
		wallet.GET("/linked", s.wrap(handlers.GetSolanaWallet))
		wallet.POST("/challenge", s.wrap(handlers.GenerateSolanaWalletChallenge))
		wallet.POST("/verify", s.wrap(handlers.VerifySolanaWallet))
		wallet.DELETE("", s.wrap(handlers.DeleteSolanaWallet))
	}

	// Solana payments (generate transaction and QR)
	solana := api.Group("/solana")
	solana.GET("/tokens", s.wrap(handlers.GetSupportedTokens))
	solana.Use(middleware.AuthRequired(s.authVerifier))
	{
		solana.POST("/generate", s.wrap(handlers.GeneratePayment))
		solana.POST("/submit", s.wrap(handlers.SubmitPayment))
		solana.POST("/qr", s.wrap(handlers.GenerateSolanaPayQR))
		solana.GET("/check", s.wrap(handlers.CheckSolanaPayment))
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
	me.Use(middleware.AuthRequired(s.authVerifier))
	{
		me.GET("/billing-status", s.wrap(handlers.GetMyBillingStatus))
	}
}

func (s *Server) setupAdminRoutes() {
	api := s.adminHandler.Group("/v1")
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

// Close currently does not own underlying resources; callers should close the App.
func (s *Server) Close(_ context.Context) error {
	log.Info("Billing HTTP server shut down")
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
