package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/services"
)

type AccessStatusResponse struct {
	IsPremium bool                        `json:"is_premium"`
	Access    []*services.UserAccessGrant `json:"access"`
}

func GetAccessStatus(r *Request) {
	user := r.GetUser()
	if user == nil {
		r.ErrorJSON(http.StatusUnauthorized, "unauthorized")
		return
	}

	if r.State == nil || r.State.UserSubscriptionService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "access service unavailable")
		return
	}

	grants, err := r.State.UserSubscriptionService.GetUserAccessStatus(r.Request.Context(), user.ID)
	switch {
	case err == nil:
		if grants == nil {
			grants = []*services.UserAccessGrant{}
		}
		r.SuccessJSON(AccessStatusResponse{IsPremium: len(grants) > 0, Access: grants})
	case errors.Is(err, sql.ErrNoRows):
		r.SuccessJSON(AccessStatusResponse{IsPremium: false, Access: []*services.UserAccessGrant{}})
	default:
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
	}
}
