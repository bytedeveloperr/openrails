package handlers

// import (
// 	"context"
// 	"math"
// 	"net/http"

// 	"github.com/doujins-org/doujins-billing/internal/api"
// 	"github.com/doujins-org/doujins-billing/internal/services"
// 	"github.com/doujins-org/doujins-billing/internal/services/subscription"
// 	"github.com/doujins-org/doujins-billing/pkg/query"
// )

// func GetBillingHistory(r *Request) {
// 	queryOpts := query.ParseQueryOptions[services.GetSubscriptionLogsFilter](r.Inner())

// 	user := r.GetUser()
// 	if queryOpts.Filters.UserID != "" && queryOpts.Filters.UserID != user.ID.String() {
// 		r.ErrorJSON(http.StatusForbidden, "Unauthorized")
// 		return
// 	}

// 	if queryOpts.Filters.UserID == "" {
// 		queryOpts.Filters.UserID = user.ID.String()
// 	}

// 	service := subscription.NewSubscriptionLogService(r.State.SubscriptionLogService)

// 	history, _, err := service.GetBillingHistory(context.Background(), queryOpts)
// 	if err != nil {
// 		r.ErrorJSON(http.StatusInternalServerError, err.Error())
// 		return
// 	}

// 	page := queryOpts.Page
// 	pageSize := queryOpts.PageSize
// 	response := query.Response{
// 		Items:      history,
// 		Page:       page,
// 		TotalItems: queryOpts.TotalItems,
// 		PageSize:   pageSize,
// 		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
// 		HasMore:    page < int(math.Ceil(float64(total)/float64(pageSize))),
// 	}

// 	r.SuccessJSON(response)
// }
