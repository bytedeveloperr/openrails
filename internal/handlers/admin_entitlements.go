package handlers

import (
	"net/http"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
)

type AdminUserEntitlementsPath struct {
	UserID string `uri:"user_id" binding:"required"`
}

type AdminEntitlementPath struct {
	UserID        string `uri:"user_id" binding:"required"`
	EntitlementID string `uri:"id" binding:"required"`
}

// GrantEntitlementRequest is the request body for granting an entitlement
type GrantEntitlementRequest struct {
	Entitlement string `json:"entitlement" binding:"required"` // e.g., "premium"
	Days        *int   `json:"days,omitempty"`                 // Optional: number of days (nil = indefinite)
}

// GrantAdminEntitlement grants an entitlement to a user (admin action)
// POST /v1/admin/users/:user_id/entitlements
func GrantAdminEntitlement(r *Request) {
	var path AdminUserEntitlementsPath
	if err := r.Inner().ShouldBindUri(&path); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	var req GrantEntitlementRequest
	if !r.BindJSON(&req) {
		return
	}

	svc := r.State.EntitlementService
	if svc == nil {
		r.ErrorJSON(http.StatusInternalServerError, "entitlement service unavailable")
		return
	}

	// Calculate end time if days specified
	var endAt *time.Time
	if req.Days != nil && *req.Days > 0 {
		now := time.Now()
		if r.State.Clock != nil {
			now = r.State.Clock.Now()
		}
		end := now.Add(time.Duration(*req.Days) * 24 * time.Hour)
		endAt = &end
	}

	ent, err := svc.GrantEntitlement(r.Request.Context(), path.UserID, req.Entitlement, endAt)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.Inner().JSON(http.StatusCreated, ent)
}

// RevokeAdminEntitlement revokes an entitlement immediately (admin action)
// DELETE /v1/admin/users/:user_id/entitlements/:id
func RevokeAdminEntitlement(r *Request) {
	var path AdminEntitlementPath
	if err := r.Inner().ShouldBindUri(&path); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	entitlementID, err := uuid.Parse(path.EntitlementID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid entitlement ID")
		return
	}

	svc := r.State.EntitlementService
	if svc == nil {
		r.ErrorJSON(http.StatusInternalServerError, "entitlement service unavailable")
		return
	}

	// Verify the entitlement belongs to the specified user
	ent, err := svc.GetByID(r.Request.Context(), entitlementID)
	if err != nil {
		r.ErrorJSON(http.StatusNotFound, "entitlement not found")
		return
	}
	if ent.UserID != path.UserID {
		r.ErrorJSON(http.StatusNotFound, "entitlement not found for this user")
		return
	}

	if err := svc.RevokeByID(r.Request.Context(), entitlementID, models.EntitlementRevokeAdmin); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("entitlement revoked")
}
