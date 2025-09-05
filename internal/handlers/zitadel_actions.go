package handlers

import (
    "net/http"
    "time"
)

// Minimal payload for ZITADEL Actions v2 when used to complement tokens.
// We only read the fields we need (user ID), keeping the struct permissive.
type zitadelUser struct {
    ID string `json:"id"`
}

type zitadelActionRequest struct {
    Function string      `json:"function"`
    User     zitadelUser `json:"user"`
    // Some payloads may provide userID at the top level; accept it as fallback.
    UserID string `json:"userID"`
}

type zitadelAppendClaim struct {
    Key   string      `json:"key"`
    Value interface{} `json:"value"`
}

type zitadelActionResponse struct {
    // append_claims instructs ZITADEL to add custom claims onto the token
    AppendClaims []zitadelAppendClaim `json:"append_claims"`
}

// PostZitadelTokenAction handles ZITADEL Actions v2 POSTs to complement tokens with entitlements.
// Route: POST /api/v1/zitadel/actions/token
// Auth: protected by admin API key middleware (X-API-KEY). If you prefer signature verification,
//       wire it in middleware and keep this handler focused on business logic.
func PostZitadelTokenAction(r *Request) {
    var req zitadelActionRequest
    if err := r.Inner().ShouldBindJSON(&req); err != nil {
        r.ErrorJSON(http.StatusBadRequest, "invalid JSON body")
        return
    }

    userID := req.User.ID
    if userID == "" {
        userID = req.UserID
    }
    if userID == "" {
        r.ErrorJSON(http.StatusBadRequest, "missing user id")
        return
    }

    svc := r.State.EntitlementService
    if svc == nil {
        r.SuccessJSON(zitadelActionResponse{AppendClaims: []zitadelAppendClaim{{Key: "entitlements", Value: []string{}}}})
        return
    }

    entitlements, err := svc.ListActiveEntitlements(r.Request.Context(), userID, time.Now().UTC())
    if err != nil {
        r.ErrorJSON(http.StatusInternalServerError, err.Error())
        return
    }

    resp := zitadelActionResponse{
        AppendClaims: []zitadelAppendClaim{{Key: "entitlements", Value: entitlements}},
    }
    r.SuccessJSON(resp)
}
