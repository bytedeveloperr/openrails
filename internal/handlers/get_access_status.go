package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/doujins-org/doujins-billing/internal/services"
	log "github.com/sirupsen/logrus"
)

type AccessStatusResponse struct {
	IsPremium bool                        `json:"is_premium"`
	Access    []*services.UserAccessGrant `json:"access"`
	Token     *AccessTokenEnvelope        `json:"token,omitempty"`
}

type AccessTokenEnvelope struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	KeyID     string    `json:"kid"`
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
		resp := AccessStatusResponse{IsPremium: len(grants) > 0, Access: grants}
		if resp.IsPremium && r.State.AccessTokenService != nil {
			if signed, signErr := r.State.AccessTokenService.SignAccessToken(r.Request.Context(), user.ID, grants); signErr != nil {
				log.WithError(signErr).Warn("failed to sign access token")
			} else if signed != nil {
				resp.Token = &AccessTokenEnvelope{Token: signed.Token, ExpiresAt: signed.ExpiresAt, KeyID: signed.KeyID}
			}
		}
		r.SuccessJSON(resp)
	case errors.Is(err, sql.ErrNoRows):
		r.SuccessJSON(AccessStatusResponse{IsPremium: false, Access: []*services.UserAccessGrant{}})
	default:
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
	}
}
