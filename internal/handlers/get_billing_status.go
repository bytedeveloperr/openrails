package handlers

import (
    "net/http"
    "time"
)

type BillingStatusResponse struct {
    IsPremium     bool        `json:"is_premium"`
    Subscription  any         `json:"subscription,omitempty"`
    NextRenewalAt *time.Time  `json:"next_renewal_at,omitempty"`
    Entitlements  any         `json:"entitlements,omitempty"`
}

func GetMyBillingStatus(r *Request) {
    user := r.GetUser()
    if user == nil {
        r.ErrorJSON(http.StatusUnauthorized, "unauthorized")
        return
    }

    now := time.Now()
    isPremium := false
    if r.State.EntitlementService != nil {
        ok, err := r.State.EntitlementService.IsEntitled(r.Request.Context(), user.ID, "premium", now)
        if err != nil {
            r.ErrorJSON(http.StatusInternalServerError, err.Error())
            return
        }
        isPremium = ok
    }

    // Subscription details
    var sub any
    var next *time.Time
    if r.State.UserSubscriptionService != nil {
        resp, err := r.State.UserSubscriptionService.GetUserSubscription(r.Request.Context(), user.ID)
        if err == nil && resp != nil {
            sub = resp
            if resp.Subscription != nil && resp.Subscription.CurrentPeriodEndsAt != nil {
                next = resp.Subscription.CurrentPeriodEndsAt
            }
        }
    }

    // List entitlements (optional)
    var ents any
    if r.State.EntitlementService != nil {
        // Query directly for now to avoid extra service APIs
        var list []map[string]any
        _ = r.State.EntitlementService.GetDB().GetDB().NewSelect().
            TableExpr("entitlements").
            Column("id","entitlement","start_at","end_at","source_type","subscription_id","payment_id","revoked_at","revoke_reason","created_at","updated_at").
            Where("user_id = ?", user.ID).
            Order("start_at DESC").
            Scan(r.Request.Context(), &list)
        ents = list
    }

    r.SuccessJSON(BillingStatusResponse{
        IsPremium:     isPremium,
        Subscription:  sub,
        NextRenewalAt: next,
        Entitlements:  ents,
    })
}
