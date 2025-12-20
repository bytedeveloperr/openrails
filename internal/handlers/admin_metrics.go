package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/services"
)

func GetAdminMetricsSummary(r *Request) {
	rng, err := parseMetricsRange(r, 30)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}
	svc := services.NewAdminMetricsService(r.State.Config.ClickHouse)
	resp, err := svc.GetSummary(r.Request.Context(), rng)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(resp)
}

func GetAdminMetricsRevenue(r *Request) {
	rng, err := parseMetricsRange(r, 30)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}
	granularity := r.Query("granularity")
	svc := services.NewAdminMetricsService(r.State.Config.ClickHouse)
	resp, err := svc.GetRevenueSeries(r.Request.Context(), rng, granularity)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(resp)
}

func GetAdminMetricsSubscriptions(r *Request) {
	rng, err := parseMetricsRange(r, 30)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}
	granularity := r.Query("granularity")
	svc := services.NewAdminMetricsService(r.State.Config.ClickHouse)
	resp, err := svc.GetSubscriptionSeries(r.Request.Context(), rng, granularity)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(resp)
}

func GetAdminMetricsProcessors(r *Request) {
	rng, err := parseMetricsRange(r, 30)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}
	svc := services.NewAdminMetricsService(r.State.Config.ClickHouse)
	resp, err := svc.GetProcessorMetrics(r.Request.Context(), rng)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(resp)
}

func GetAdminMetricsChurn(r *Request) {
	rng, err := parseMetricsRange(r, 180)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}
	svc := services.NewAdminMetricsService(r.State.Config.ClickHouse)
	resp, err := svc.GetChurn(r.Request.Context(), rng)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
	r.SuccessJSON(resp)
}

func parseMetricsRange(r *Request, defaultDays int) (services.MetricsDateRange, error) {
	startParam := strings.TrimSpace(r.Query("start"))
	endParam := strings.TrimSpace(r.Query("end"))
	periodParam := strings.TrimSpace(r.Query("period"))

	now := time.Now().UTC()
	var start, end time.Time
	var err error

	if endParam != "" {
		end, err = parseDateParam(endParam)
		if err != nil {
			return services.MetricsDateRange{}, err
		}
	} else {
		end = now
	}

	if startParam != "" {
		start, err = parseDateParam(startParam)
		if err != nil {
			return services.MetricsDateRange{}, err
		}
	} else if periodParam != "" {
		start = applyPeriod(end, periodParam)
	} else {
		start = end.AddDate(0, 0, -defaultDays)
	}

	if start.After(end) {
		start, end = end, start
	}

	return services.MetricsDateRange{Start: start, End: end}, nil
}

func parseDateParam(value string) (time.Time, error) {
	layouts := []string{time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date: %s", value)
}

func applyPeriod(end time.Time, period string) time.Time {
	lower := strings.ToLower(period)
	switch lower {
	case "7d":
		return end.AddDate(0, 0, -7)
	case "30d", "":
		return end.AddDate(0, 0, -30)
	case "90d":
		return end.AddDate(0, 0, -90)
	case "month":
		return end.AddDate(0, -1, 0)
	case "quarter":
		return end.AddDate(0, -3, 0)
	case "year":
		return end.AddDate(-1, 0, 0)
	default:
		if strings.HasSuffix(lower, "d") {
			days, err := strconv.Atoi(strings.TrimSuffix(lower, "d"))
			if err == nil {
				return end.AddDate(0, 0, -days)
			}
		}
		return end.AddDate(0, 0, -30)
	}
}
