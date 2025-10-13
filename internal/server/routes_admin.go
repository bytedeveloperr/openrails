package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/doujins-org/doujins-billing/internal/handlers"
)

func (s *Server) registerAdminRoutes() {
	api := s.adminHandler.Group("/v1")
	api.PUT("/subscriptions/:id/extend", s.wrap(handlers.ExtendSubscription))
	api.POST("/subscriptions/:id/cancel", s.wrap(handlers.CancelSubscription))
	api.GET("/subscriptions/:id/details", s.wrap(handlers.GetSubscription))
	api.GET("/subscriptions/dashboard-metrics", s.wrap(handlers.GetAdminDashboardMetrics))
	api.GET("/subscriptions/daily-metrics", s.wrap(handlers.GetAdminDailyMetrics))
	api.GET("/subscriptions/processor-metrics", s.wrap(handlers.GetAdminProcessorMetrics))
	api.GET("/users/:user_id/entitlements", s.wrap(handlers.GetAdminActiveEntitlements))

	s.adminHandler.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "billing-admin"})
	})
	s.adminHandler.HEAD("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}
