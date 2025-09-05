package handlers

import (
    "net/http"
    "time"
)

type AdminUserEntitlementsPath struct {
    UserID string `uri:"user_id" binding:"required"`
}

// GetAdminActiveEntitlements returns the list of currently active entitlements for a user
func GetAdminActiveEntitlements(r *Request) {
    var path AdminUserEntitlementsPath
    if err := r.Inner().ShouldBindUri(&path); err != nil {
        r.ErrorJSON(http.StatusBadRequest, err.Error())
        return
    }

    svc := r.State.EntitlementService
    if svc == nil {
        r.SuccessJSON([]any{})
        return
    }

    now := time.Now()
    // Active if: revoked_at IS NULL AND start_at <= now AND (end_at IS NULL OR end_at > now)
    var ents []map[string]any
    err := svc.GetDB().GetDB().NewSelect().
        Table("entitlements").
        Column("id", "entitlement", "start_at", "end_at", "source_type", "subscription_id", "payment_id", "created_at", "updated_at").
        Where("user_id = ?", path.UserID).
        Where("revoked_at IS NULL").
        Where("start_at <= ?", now).
        Where("(end_at IS NULL OR end_at > ?)", now).
        Order("start_at ASC").
        Scan(r.Request.Context(), &ents)
    if err != nil {
        r.ErrorJSON(http.StatusInternalServerError, err.Error())
        return
    }

    r.SuccessJSON(ents)
}

