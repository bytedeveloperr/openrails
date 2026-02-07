package server

import (
	"github.com/gin-gonic/gin"
	"github.com/open-rails/openrails/internal/auth/policy"
	"github.com/open-rails/openrails/internal/handlers"
)

func (s *Server) registerAdminRoutesAt(e *gin.Engine, apiPrefix string) {
	// Admin routes are protected by JWT authentication + admin role requirement.
	admin := e.Group(apiPrefix + "/admin")
	admin.Use(s.authProvider.Required())

	admin.Use(policy.AdminRequired(s.runtime.DB.GetDB()))

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
	admin.GET("/users/:user_id/ccbill", s.wrap(handlers.GetAdminUserCCBill))
	admin.GET("/users/:user_id/ccbill/metrics", s.wrap(handlers.GetAdminUserCCBillMetrics))

	admin.POST("/users/:user_id/entitlements", s.wrap(handlers.GrantAdminEntitlement))
	admin.DELETE("/users/:user_id/entitlements/:id", s.wrap(handlers.RevokeAdminEntitlement))

	// Metrics
	admin.GET("/metrics/summary", s.wrap(handlers.GetAdminMetricsSummary))
	admin.GET("/metrics/revenue", s.wrap(handlers.GetAdminMetricsRevenue))
	admin.GET("/metrics/subscriptions", s.wrap(handlers.GetAdminMetricsSubscriptions))
	admin.GET("/metrics/processors", s.wrap(handlers.GetAdminMetricsProcessors))
	admin.GET("/metrics/churn", s.wrap(handlers.GetAdminMetricsChurn))
}

func (s *Server) registerAdminRoutesOn(e *gin.Engine) {
	s.registerAdminRoutesAt(e, StandaloneV1Prefix)
}
