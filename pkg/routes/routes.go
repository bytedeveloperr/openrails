// Package routes provides route registration functions for embedded hosts.
//
// These functions allow embedded hosts to mount billing routes on their own Gin router
// without creating a full billing Server object.
package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/doujins-org/doujins-billing/internal/app"
	authpolicy "github.com/doujins-org/doujins-billing/internal/auth/policy"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/pkg/authprovider"
)

// Options configures route registration behavior.
type Options struct {
	// AuthProvider is required for routes that need authentication.
	AuthProvider authprovider.Provider
}

// wrapHandler creates a Gin handler function from a Request handler.
func wrapHandler(rt *app.Runtime, fn func(r *handlers.Request)) gin.HandlerFunc {
	return func(c *gin.Context) {
		fn(handlers.NewRequest(c, rt))
	}
}

// RegisterUserRoutes registers user-facing billing routes on the provided Gin router group.
// These routes include products, prices, checkout, subscriptions, payments, etc.
//
// Example usage for embedded hosts:
//
//	router := gin.Default()
//	api := router.Group("/v1")
//	routes.RegisterUserRoutes(api, runtime, routes.Options{
//	    AuthProvider: myAuthProvider,
//	})
func RegisterUserRoutes(group *gin.RouterGroup, rt *app.Runtime, opts Options) {
	if opts.AuthProvider == nil {
		panic("AuthProvider is required for user routes")
	}

	wrap := func(fn func(r *handlers.Request)) gin.HandlerFunc {
		return wrapHandler(rt, fn)
	}

	// Products and Prices - public catalog endpoints
	group.GET("/products", opts.AuthProvider.Optional(), wrap(handlers.GetProducts))
	group.GET("/prices", opts.AuthProvider.Optional(), wrap(handlers.GetPrices))

	// Solana tokens endpoint (public, no auth required)
	group.GET("/solana/tokens", wrap(handlers.GetSupportedTokens))

	// Checkout Sessions - unified flow
	checkout := group.Group("/checkout")
	checkout.Use(opts.AuthProvider.Required())
	checkout.POST("", wrap(handlers.CreateCheckoutSession))
	checkout.GET("/:id", wrap(handlers.GetCheckoutSession))
	checkout.POST("/:id/confirm", wrap(handlers.ConfirmCheckoutSession))

	me := group.Group("/me")
	me.Use(opts.AuthProvider.Required())
	me.GET("/status", wrap(handlers.GetMyBillingStatus))
	// Subscription endpoints - RESTful with :id in path
	me.GET("/subscriptions", wrap(handlers.GetMySubscriptions))
	me.GET("/subscriptions/:id", wrap(handlers.GetSubscription))
	me.PUT("/subscriptions/:id/payment-method", wrap(handlers.UpdateSubscriptionPaymentMethod))
	me.POST("/subscriptions/:id/cancel", wrap(handlers.CancelSubscription))
	me.POST("/subscriptions/:id/resume", wrap(handlers.ResumeSubscription))
	me.POST("/subscriptions/:id/change-tier", wrap(handlers.ChangeTier))
	me.GET("/payments", wrap(handlers.GetUserPayments))
	me.GET("/payment-methods", wrap(handlers.ListPaymentMethods))
	me.POST("/payment-methods", wrap(handlers.CreatePaymentMethod))
	me.PUT("/payment-methods/:id", wrap(handlers.UpdatePaymentMethod))
	me.DELETE("/payment-methods/:id", wrap(handlers.DeletePaymentMethod))
	me.GET("/notifications", wrap(handlers.GetNotifications))
	me.GET("/notifications/unread-count", wrap(handlers.GetUnreadNotificationCount))
	me.POST("/notifications/:id/read", wrap(handlers.MarkNotificationRead))
	me.GET("/credits", wrap(handlers.GetMyCredits))
	me.GET("/credits/:type", wrap(handlers.GetMyCreditsType))
	me.GET("/credits/:type/transactions", wrap(handlers.GetMyCreditTransactions))
	me.POST("/portal", wrap(handlers.CreatePortalSession))
}

// RegisterAdminRoutes registers admin billing routes on the provided Gin router group.
// These routes include subscription management, payment management, user management, and metrics.
// All routes require admin authorization.
//
// Example usage for embedded hosts:
//
//	router := gin.Default()
//	admin := router.Group("/v1/admin")
//	routes.RegisterAdminRoutes(admin, runtime, routes.Options{
//	    AuthProvider: myAuthProvider,
//	})
func RegisterAdminRoutes(group *gin.RouterGroup, rt *app.Runtime, opts Options) {
	if opts.AuthProvider == nil {
		panic("AuthProvider is required for admin routes")
	}

	wrap := func(fn func(r *handlers.Request)) gin.HandlerFunc {
		return wrapHandler(rt, fn)
	}

	// Admin routes are protected by JWT authentication + admin role requirement
	group.Use(opts.AuthProvider.Required())
	group.Use(authpolicy.AdminRequired(rt.DB.GetDB()))

	// Subscription management
	group.GET("/subscriptions", wrap(handlers.GetAdminSubscriptions))
	group.GET("/subscriptions/:id", wrap(handlers.GetAdminSubscription))
	group.POST("/subscriptions/:id/cancel", wrap(handlers.AdminCancelSubscription))

	// Payment management
	group.GET("/payments", wrap(handlers.GetAdminPayments))
	group.GET("/payments/:id", wrap(handlers.GetAdminPayment))
	group.POST("/payments/:id/refund", wrap(handlers.AdminRefundPayment))
	group.GET("/users/:user_id/payments", wrap(handlers.GetAdminUserPayments))
	group.POST("/users/:user_id/payments/off-channel", wrap(handlers.AdminCreateOffChannelPayment))

	// User management
	group.GET("/users/:user_id", wrap(handlers.GetAdminUserBillingProfile))
	group.GET("/users/:user_id/entitlements", wrap(handlers.GetAdminUserEntitlements))
	group.POST("/users/:user_id/entitlements", wrap(handlers.GrantAdminEntitlement))
	group.DELETE("/users/:user_id/entitlements/:id", wrap(handlers.RevokeAdminEntitlement))

	// Metrics
	group.GET("/metrics/summary", wrap(handlers.GetAdminMetricsSummary))
	group.GET("/metrics/revenue", wrap(handlers.GetAdminMetricsRevenue))
	group.GET("/metrics/subscriptions", wrap(handlers.GetAdminMetricsSubscriptions))
	group.GET("/metrics/processors", wrap(handlers.GetAdminMetricsProcessors))
	group.GET("/metrics/churn", wrap(handlers.GetAdminMetricsChurn))
}

// RegisterWebhookRoutes registers webhook routes on the provided Gin router group.
// These routes handle incoming webhooks from payment processors (Stripe, CCBill, NMI, etc.).
//
// Example usage for embedded hosts:
//
//	router := gin.Default()
//	webhooks := router.Group("/v1/webhooks")
//	routes.RegisterWebhookRoutes(webhooks, runtime)
func RegisterWebhookRoutes(group *gin.RouterGroup, rt *app.Runtime) {
	wrap := func(fn func(r *handlers.Request)) gin.HandlerFunc {
		return wrapHandler(rt, fn)
	}

	group.POST("/:provider", wrap(handlers.Webhook))
}

// RegisterHealthRoutes registers health check routes on the provided Gin engine.
// These are typically mounted at the root level.
//
// Example usage for embedded hosts:
//
//	router := gin.Default()
//	routes.RegisterHealthRoutes(router, runtime)
func RegisterHealthRoutes(e *gin.Engine, rt *app.Runtime) {
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

	e.GET("/health/ready", createReadyHandler(rt))

	// Kubernetes-style health check endpoints (aliases)
	e.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing"})
	})
	e.GET("/readyz", createReadyHandler(rt))
}

func createReadyHandler(rt *app.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		checks := gin.H{
			"db":    "ok",
			"redis": "ok",
		}

		// Check database (critical)
		var one int
		if rt != nil && rt.DB != nil {
			if err := rt.DB.GetDB().NewSelect().ColumnExpr("1").Scan(ctx, &one); err != nil {
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
		if rt != nil && rt.RedisClient != nil {
			if _, err := rt.RedisClient.Ping(ctx).Result(); err != nil {
				checks["redis"] = "down"
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": checks})
				return
			}
		} else {
			checks["redis"] = "missing"
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready", "checks": checks})
	}
}

// RegisterServiceRoutes registers internal service-to-service API routes.
// These routes are intended for X-API-KEY authentication and should only
// be accessible within trusted networks.
//
// Example usage:
//
//	router := gin.Default()
//	svc := router.Group("/v1")
//	routes.RegisterServiceRoutes(svc, runtime, apiKeyMiddleware)
func RegisterServiceRoutes(group *gin.RouterGroup, rt *app.Runtime, authMiddleware gin.HandlerFunc) {
	wrap := func(fn func(r *handlers.Request)) gin.HandlerFunc {
		return wrapHandler(rt, fn)
	}

	group.Use(authMiddleware)

	// User entitlements
	group.GET("/users/:user_id/entitlements", wrap(handlers.ServiceGetUserEntitlements))

	// Credits operations
	credits := group.Group("/credits")
	credits.POST("/withdraw", wrap(handlers.ServiceWithdrawCredits))
	credits.POST("/hold", wrap(handlers.ServiceHoldCredits))
	credits.POST("/holds/:id/capture", wrap(handlers.ServiceCaptureHold))
	credits.POST("/holds/:id/release", wrap(handlers.ServiceReleaseHold))
	credits.GET("/users/:user_id", wrap(handlers.ServiceGetUserCredits))
}
