package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/middleware"
)

func (s *Server) registerPublicRoutes() {
	// Root: simple JSON banner for API servers
	s.publicHandler.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":   "billing",
			"status":    "ok",
			"endpoints": []string{"/health/live", "/health/ready", "/v1"},
		})
	})

	api := s.publicHandler.Group("/v1")

	// Products and Prices - public catalog endpoints
	api.GET("/products", s.wrap(handlers.GetProducts))
	api.GET("/prices", s.wrap(handlers.GetPrices))

	subscriptions := api.Group("/subscriptions")
	// Legacy route - kept for backwards compatibility
	subscriptions.GET("/products", s.wrap(handlers.GetProducts))

	subscriptions.Use(middleware.AuthRequired(s.authVerifier))
	// Processor routes for subscription creation
	// mobius uses NMI gateway, ccbill and solana are self-contained processors
	subscriptions.POST("/mobius", s.wrap(handlers.Subscribe))
	subscriptions.POST("/ccbill", s.wrap(handlers.GenerateFlexFormURL))
	subscriptions.POST("/solana", s.wrap(handlers.Subscribe))
	subscriptions.POST("/ccbill/flexform-url", s.wrap(handlers.GenerateFlexFormURL))
	subscriptions.POST("/cancel", s.wrap(handlers.CancelSubscription))
	subscriptions.GET("/active", s.wrap(handlers.GetSubscription))
	subscriptions.GET("/history", s.wrap(handlers.GetSubscriptionHistory))
	subscriptions.GET("/purchases", s.wrap(handlers.GetUserPayments))

	// Webhooks - single provider path (mobius/ccbill/solana)
	webhooks := api.Group("/webhooks")
	webhooks.POST("/:provider", s.wrap(handlers.Webhook))

	pms := api.Group("/payment-methods")
	pms.Use(middleware.AuthRequired(s.authVerifier))
	pms.POST("", s.wrap(handlers.CreatePaymentMethod))
	pms.GET("", s.wrap(handlers.ListPaymentMethods))
	pms.PUT(":id", s.wrap(handlers.UpdatePaymentMethod))
	pms.DELETE(":id", s.wrap(handlers.DeletePaymentMethod))
	pms.PUT(":id/activate", s.wrap(handlers.ActivatePaymentMethod))

	notifications := api.Group("/notifications")
	notifications.Use(middleware.AuthRequired(s.authVerifier))
	notifications.GET("", s.wrap(handlers.GetNotifications))
	notifications.GET("/unread-count", s.wrap(handlers.GetUnreadNotificationCount))
	notifications.POST(":id/read", s.wrap(handlers.MarkNotificationRead))

	wallet := api.Group("/wallet/solana")
	wallet.Use(middleware.AuthRequired(s.authVerifier))
	wallet.GET("", s.wrap(handlers.ListSolanaWallets))
	wallet.GET("/linked", s.wrap(handlers.GetSolanaWallet))
	wallet.POST("/challenge", s.wrap(handlers.GenerateSolanaWalletChallenge))
	wallet.POST("/verify", s.wrap(handlers.VerifySolanaWallet))
	wallet.DELETE("", s.wrap(handlers.DeleteSolanaWallet))

	// Payment Intents - Stripe-like pattern for Solana payments
	// POST /payment-intents - Create a new payment intent (direct wallet flow)
	// POST /payment-intents/qr - Create a new payment intent (QR/Solana Pay flow)
	// GET /payment-intents/:id - Get payment intent status
	// POST /payment-intents/:id/confirm - Confirm/submit a signed transaction
	paymentIntents := api.Group("/payment-intents")
	paymentIntents.Use(middleware.AuthRequired(s.authVerifier))
	paymentIntents.POST("", s.wrap(handlers.CreatePaymentIntent))
	paymentIntents.POST("/qr", s.wrap(handlers.CreatePaymentIntentQR))
	paymentIntents.GET("/:id", s.wrap(handlers.GetPaymentIntent))
	paymentIntents.POST("/:id/confirm", s.wrap(handlers.ConfirmPaymentIntent))

	// Solana - legacy routes kept for backwards compatibility, will be removed
	solana := api.Group("/solana")
	solana.GET("/tokens", s.wrap(handlers.GetSupportedTokens))
	solana.Use(middleware.AuthRequired(s.authVerifier))
	solana.POST("/generate", s.wrap(handlers.GeneratePayment)) // Use POST /payment-intents instead
	solana.POST("/submit", s.wrap(handlers.SubmitPayment))     // Use POST /payment-intents/:id/confirm instead
	solana.POST("/qr", s.wrap(handlers.GenerateSolanaPayQR))   // Use POST /payment-intents/qr instead
	solana.GET("/check", s.wrap(handlers.CheckSolanaPayment))  // Use GET /payment-intents/:id instead

	me := api.Group("/me")
	me.Use(middleware.AuthRequired(s.authVerifier))
	me.GET("/status", s.wrap(handlers.GetMyBillingStatus))
	// New user-scoped endpoints
	me.GET("/subscriptions", s.wrap(handlers.GetMySubscriptions))
	me.POST("/subscriptions/cancel", s.wrap(handlers.CancelSubscription))
	me.GET("/payments", s.wrap(handlers.GetUserPayments))
	me.GET("/payment-methods", s.wrap(handlers.ListPaymentMethods))
	me.POST("/payment-methods", s.wrap(handlers.CreatePaymentMethod))
	me.PUT("/payment-methods/:id", s.wrap(handlers.UpdatePaymentMethod))
	me.DELETE("/payment-methods/:id", s.wrap(handlers.DeletePaymentMethod))
	me.PUT("/payment-methods/:id/activate", s.wrap(handlers.ActivatePaymentMethod))
	me.GET("/wallets", s.wrap(handlers.ListSolanaWallets))
	me.GET("/wallets/linked", s.wrap(handlers.GetSolanaWallet))
	me.POST("/wallets/challenge", s.wrap(handlers.GenerateSolanaWalletChallenge))
	me.POST("/wallets/verify", s.wrap(handlers.VerifySolanaWallet))
	me.DELETE("/wallets", s.wrap(handlers.DeleteSolanaWallet))
	me.GET("/notifications", s.wrap(handlers.GetNotifications))
	me.GET("/notifications/unread-count", s.wrap(handlers.GetUnreadNotificationCount))
	me.POST("/notifications/:id/read", s.wrap(handlers.MarkNotificationRead))

	s.publicHandler.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})

	s.publicHandler.GET("/health/ready", s.readyHandler)

	// Kubernetes-style health check endpoints (aliases)
	s.publicHandler.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})
	s.publicHandler.GET("/readyz", s.readyHandler)
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
	if s.authVerifier == nil {
		checks["authkit"] = "not_initialized"
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready", "checks": checks})
}
