package server

import (
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/pkg/authprovider"
	"github.com/gin-gonic/gin"
)

func (s *Server) registerAdminRoutesOn(e *gin.Engine) {
	// Admin routes are protected by JWT authentication + admin role requirement.
	admin := e.Group("/v1/admin")
	admin.Use(s.authProvider.Required())

	admin.Use(func(c *gin.Context) {
		if uc, ok := authprovider.UserContextFromGin(c); ok && uc.UserID != "" {
			c.Set("auth.user_id", uc.UserID)
		}
		c.Next()
	})
	admin.Use(s.adminAuth.RequireAdmin(s.adminAuthPool))

	// Subscription management
	admin.GET("/subscriptions", s.wrap(handlers.GetAdminSubscriptions))
	admin.GET("/subscriptions/:id", s.wrap(handlers.GetAdminSubscription))
	admin.POST("/subscriptions/:id/cancel", s.wrap(handlers.AdminCancelSubscription))

	// Payment management
	admin.GET("/payments", s.wrap(handlers.GetAdminPayments))
	admin.GET("/payments/:id", s.wrap(handlers.GetAdminPayment))
	admin.POST("/payments/:id/refund", s.wrap(handlers.AdminRefundPayment))
	admin.GET("/users/:user_id/payments", s.wrap(handlers.GetAdminUserPayments))
	admin.POST("/users/:user_id/payments/off-channel", s.wrap(handlers.AdminCreateOffChannelPayment))

	// User management
	admin.GET("/users/:user_id", s.wrap(handlers.GetAdminUserBillingProfile))
	admin.GET("/users/:user_id/entitlements", s.wrap(handlers.GetAdminUserEntitlements))
	admin.GET("/users/:user_id/mobius", s.wrap(handlers.GetAdminUserMobius))
	admin.GET("/users/:user_id/mobius/metrics", s.wrap(handlers.GetAdminUserMobiusMetrics))

	admin.POST("/users/:user_id/entitlements", s.wrap(handlers.GrantAdminEntitlement))
	admin.DELETE("/users/:user_id/entitlements/:id", s.wrap(handlers.RevokeAdminEntitlement))

	// Metrics
	admin.GET("/metrics/summary", s.wrap(handlers.GetAdminMetricsSummary))
	admin.GET("/metrics/revenue", s.wrap(handlers.GetAdminMetricsRevenue))
	admin.GET("/metrics/subscriptions", s.wrap(handlers.GetAdminMetricsSubscriptions))
	admin.GET("/metrics/processors", s.wrap(handlers.GetAdminMetricsProcessors))
	admin.GET("/metrics/churn", s.wrap(handlers.GetAdminMetricsChurn))
}
