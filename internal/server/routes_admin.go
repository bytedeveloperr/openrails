package server

import (
	authpolicy "github.com/doujins-org/doujins-billing/internal/auth/policy"
	"github.com/doujins-org/doujins-billing/internal/handlers"
)

func (s *Server) registerAdminRoutes() {
	// Admin routes are protected by JWT authentication + admin role requirement
	// These routes were previously on a separate port with API key auth.
	// Now they're unified on the main server with proper JWT-based authorization.
	admin := s.publicHandler.Group("/v1/admin")
	admin.Use(s.authProvider.Required())
	admin.Use(authpolicy.AdminRequired(s.runtime.DB.GetDB()))

	// Subscription management
	admin.GET("/subscriptions", s.wrap(handlers.GetAdminSubscriptions))
	admin.GET("/subscriptions/:id", s.wrap(handlers.GetAdminSubscription))
	admin.PUT("/subscriptions/:id/extend", s.wrap(handlers.ExtendSubscription))
	admin.POST("/subscriptions/:id/cancel", s.wrap(handlers.AdminCancelSubscription))

	// Payment management
	admin.GET("/payments", s.wrap(handlers.GetAdminPayments))
	admin.GET("/payments/:id", s.wrap(handlers.GetAdminPayment))
	admin.POST("/payments/:id/refund", s.wrap(handlers.AdminRefundPayment))

	// User management
	admin.GET("/users/:user_id", s.wrap(handlers.GetAdminUserBillingProfile))
	admin.GET("/users/:user_id/entitlements", s.wrap(handlers.GetAdminUserEntitlements))
	admin.POST("/users/:user_id/entitlements", s.wrap(handlers.GrantAdminEntitlement))
	admin.DELETE("/users/:user_id/entitlements/:id", s.wrap(handlers.RevokeAdminEntitlement))

	// Admin grants (product-based grants with audit trail)
	admin.POST("/users/:user_id/grants", s.wrap(handlers.CreateAdminGrant))
	admin.GET("/users/:user_id/grants", s.wrap(handlers.ListAdminGrantsByUser))
	admin.GET("/grants/:id", s.wrap(handlers.GetAdminGrant))

	// Metrics
	admin.GET("/metrics/summary", s.wrap(handlers.GetAdminMetricsSummary))
	admin.GET("/metrics/revenue", s.wrap(handlers.GetAdminMetricsRevenue))
	admin.GET("/metrics/subscriptions", s.wrap(handlers.GetAdminMetricsSubscriptions))
	admin.GET("/metrics/processors", s.wrap(handlers.GetAdminMetricsProcessors))
	admin.GET("/metrics/churn", s.wrap(handlers.GetAdminMetricsChurn))
}
