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

	subscriptions := api.Group("/subscriptions")
	subscriptions.GET("/products", s.wrap(handlers.GetProducts))
	subscriptions.GET("/page-data", s.wrap(handlers.GetSubscribePageData))

	subscriptions.Use(middleware.AuthRequired(s.authVerifier))
	subscriptions.POST("/process/:processor", s.wrap(handlers.Subscribe))
	subscriptions.POST("/ccbill/flexform-url", s.wrap(handlers.GenerateFlexFormURL))
	subscriptions.POST("/cancel", s.wrap(handlers.CancelSubscription))
	subscriptions.GET("/active", s.wrap(handlers.GetSubscription))
	subscriptions.GET("/history", s.wrap(handlers.GetSubscriptionHistory))
	subscriptions.GET("/purchases", s.wrap(handlers.GetUserPayments))

	webhooks := api.Group("/subscriptions/webhook")
	webhooks.POST("/:processor", s.wrap(handlers.Webhook))
	webhooks.POST("/:processor/:provider", s.wrap(handlers.Webhook))

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

	solana := api.Group("/solana")
	solana.GET("/tokens", s.wrap(handlers.GetSupportedTokens))
	solana.Use(middleware.AuthRequired(s.authVerifier))
	solana.POST("/generate", s.wrap(handlers.GeneratePayment))
	solana.POST("/submit", s.wrap(handlers.SubmitPayment))
	solana.POST("/qr", s.wrap(handlers.GenerateSolanaPayQR))
	solana.GET("/check", s.wrap(handlers.CheckSolanaPayment))

	access := api.Group("/access")
	access.Use(middleware.AuthRequired(s.authVerifier))
	access.GET("", s.wrap(handlers.GetAccessStatus))

	api.GET("/me/status", s.wrap(handlers.GetMyBillingStatus))

	s.publicHandler.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})

	s.publicHandler.GET("/health/ready", s.readyHandler)
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
