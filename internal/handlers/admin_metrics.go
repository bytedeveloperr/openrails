package handlers

import (
	"net/http"
	"time"

	"github.com/doujins-org/doujins-billing/internal/services"
)

// GetAdminMetrics handles all admin metrics requests
// GET /v1/admin/metrics?type=dashboard|daily|processor
// For daily metrics: &start=YYYY-MM-DD&end=YYYY-MM-DD
func GetAdminMetrics(r *Request) {
	metricsType := r.Query("type")
	if metricsType == "" {
		metricsType = "dashboard" // default
	}

	svc := services.NewDashboardMetricsService(r.State.DB)

	switch metricsType {
	case "dashboard":
		metrics, err := svc.GetDashboardMetrics(r.Request.Context())
		if err != nil {
			r.ErrorJSON(http.StatusInternalServerError, err.Error())
			return
		}
		r.SuccessJSON(metrics)

	case "daily":
		startStr := r.Query("start")
		endStr := r.Query("end")
		if startStr == "" || endStr == "" {
			r.ErrorJSON(http.StatusBadRequest, "start and end query params are required for daily metrics (YYYY-MM-DD)")
			return
		}
		start, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			r.ErrorJSON(http.StatusBadRequest, "invalid start date")
			return
		}
		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			r.ErrorJSON(http.StatusBadRequest, "invalid end date")
			return
		}

		metrics, err := svc.GetDailyMetrics(r.Request.Context(), start, end)
		if err != nil {
			r.ErrorJSON(http.StatusInternalServerError, err.Error())
			return
		}
		r.SuccessJSON(metrics)

	case "processor":
		metrics, err := svc.GetMetricsByProcessor(r.Request.Context())
		if err != nil {
			r.ErrorJSON(http.StatusInternalServerError, err.Error())
			return
		}
		r.SuccessJSON(metrics)

	default:
		r.ErrorJSON(http.StatusBadRequest, "invalid metrics type: must be 'dashboard', 'daily', or 'processor'")
	}
}
