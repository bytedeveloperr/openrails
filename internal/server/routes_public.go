package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/doujins-org/doujins-billing/internal/handlers"
)

func (s *Server) registerUserRoutes(e *gin.Engine) {
	api := e.Group("/v1")

	// Products and Prices - public catalog endpoints
	api.GET("/products", s.authProvider.Optional(), s.wrap(handlers.GetProducts))
	api.GET("/prices", s.authProvider.Optional(), s.wrap(handlers.GetPrices))

	// Solana tokens endpoint (public, no auth required)
	api.GET("/solana/tokens", s.wrap(handlers.GetSupportedTokens))

	// Checkout Sessions - unified flow
	checkout := api.Group("/checkout")
	checkout.Use(s.authProvider.Required())
	checkout.POST("", s.wrap(handlers.CreateCheckoutSession))
	checkout.GET("/:id", s.wrap(handlers.GetCheckoutSession))
	checkout.POST("/:id/confirm", s.wrap(handlers.ConfirmCheckoutSession))

	me := api.Group("/me")
	me.Use(s.authProvider.Required())
	me.GET("/status", s.wrap(handlers.GetMyBillingStatus))
	// Subscription endpoints - RESTful with :id in path
	me.GET("/subscriptions", s.wrap(handlers.GetMySubscriptions))
	me.GET("/subscriptions/:id", s.wrap(handlers.GetSubscription))
	me.PUT("/subscriptions/:id/payment-method", s.wrap(handlers.UpdateSubscriptionPaymentMethod))
	me.POST("/subscriptions/:id/cancel", s.wrap(handlers.CancelSubscription))
	me.POST("/subscriptions/:id/resume", s.wrap(handlers.ResumeSubscription))
	me.POST("/subscriptions/:id/change-tier", s.wrap(handlers.ChangeTier))
	me.GET("/payments", s.wrap(handlers.GetUserPayments))
	me.GET("/payment-methods", s.wrap(handlers.ListPaymentMethods))
	me.POST("/payment-methods", s.wrap(handlers.CreatePaymentMethod))
	me.PUT("/payment-methods/:id", s.wrap(handlers.UpdatePaymentMethod))
	me.DELETE("/payment-methods/:id", s.wrap(handlers.DeletePaymentMethod))
	me.GET("/notifications", s.wrap(handlers.GetNotifications))
	me.GET("/notifications/unread-count", s.wrap(handlers.GetUnreadNotificationCount))
	me.POST("/notifications/:id/read", s.wrap(handlers.MarkNotificationRead))
	me.GET("/credits", s.wrap(handlers.GetMyCredits))
	me.GET("/credits/:type", s.wrap(handlers.GetMyCreditsType))
	me.GET("/credits/:type/transactions", s.wrap(handlers.GetMyCreditTransactions))

	// Stripe-specific endpoints
	stripe := api.Group("/stripe")
	stripe.Use(s.authProvider.Required())
	stripe.POST("/portal", s.wrap(handlers.CreatePortalSession))
}

func (s *Server) registerWebhookRoutes(e *gin.Engine) {
	api := e.Group("/v1")
	webhooks := api.Group("/webhooks")
	webhooks.POST("/:provider", s.wrap(handlers.Webhook))
}

// registerStandaloneMetaRoutes registers banner/health endpoints that are appropriate for the
// standalone billing service, but should not be forced onto embedded hosts.
func (s *Server) registerStandaloneMetaRoutes(e *gin.Engine) {
	// Root: simple JSON banner for API servers
	e.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":   "billing",
			"status":    "ok",
			"endpoints": []string{"/health/live", "/health/ready", "/v1"},
		})
	})

	e.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})

	e.GET("/health/ready", s.readyHandler)

	// Kubernetes-style health check endpoints (aliases)
	e.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})
	e.GET("/readyz", s.readyHandler)
}

func (s *Server) registerPublicRoutes() {
	// Standalone public handler: full surface area for convenience.
	s.registerStandaloneMetaRoutes(s.publicHandler)
	s.registerUserRoutes(s.publicHandler)
	s.registerWebhookRoutes(s.publicHandler)

	// Embedded split handlers: allow hosts to mount only what they need.
	s.registerUserRoutes(s.userHandler)
	s.registerWebhookRoutes(s.webhookHandler)
}

func (s *Server) readyHandler(c *gin.Context) {
	ctx := c.Request.Context()
	checks := gin.H{
		"db":      "ok",
		"redis":   "ok",
		"authkit": "ok",
	}

	// Check database (critical)
	var one int
	if s.runtime != nil && s.runtime.DB != nil {
		if err := s.runtime.DB.GetDB().NewSelect().ColumnExpr("1").Scan(ctx, &one); err != nil {
			checks["db"] = "down"
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
			return
		}
	} else {
		checks["db"] = "missing"
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
		return
	}

	// Check Redis (critical for billing operations)
	if s.runtime != nil && s.runtime.RedisClient != nil {
		if _, err := s.runtime.RedisClient.Ping(ctx).Result(); err != nil {
			checks["redis"] = "down"
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
			return
		}
	} else {
		checks["redis"] = "missing"
	}

	// Check AuthKit verifier (critical for authentication)
	if s.authProvider == nil {
		checks["authkit"] = "not_initialized"
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready", "checks": checks})
}
