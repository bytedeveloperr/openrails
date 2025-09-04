package handlers

import (
    "net/http"
    "strconv"

    "github.com/google/uuid"
)

// ListPaymentMethods returns the user's payment methods (optionally including inactive)
func ListPaymentMethods(r *Request) {
    req := new(ListPaymentMethodsRequest)
    if err := r.Bind(req); err != nil {
        r.ErrorJSON(http.StatusBadRequest, "Invalid request")
        return
    }
    // Fallback if Bind doesn't populate query embedded struct
    if p := r.Request.URL.Query().Get("page"); p != "" {
        if v, err := strconv.Atoi(p); err == nil {
            req.Page = v
        }
    }
    if ps := r.Request.URL.Query().Get("page_size"); ps != "" {
        if v, err := strconv.Atoi(ps); err == nil {
            req.PageSize = v
        }
    }
    req.SetDefaults()

    user := r.GetUser()
    // Read methods via service
    var list any
    if req.IncludeInactive {
        if m, err := r.State.PaymentMethodService.GetByUserID(r.Request.Context(), user.ID); err == nil {
            list = m
        } else {
            r.ErrorJSON(http.StatusInternalServerError, "Failed to retrieve payment methods")
            return
        }
    } else {
        if m, err := r.State.PaymentMethodService.GetActiveByUserID(r.Request.Context(), user.ID); err == nil {
            list = m
        } else {
            r.ErrorJSON(http.StatusInternalServerError, "Failed to retrieve payment methods")
            return
        }
    }

    r.SuccessJSON(PaginatedResponse{Data: list, TotalItems: 0})
}

// DeletePaymentMethod removes a payment method by ID
func DeletePaymentMethod(r *Request) {
    req := new(DeletePaymentMethodRequest)
    if err := r.Bind(req); err != nil {
        r.ErrorJSON(http.StatusBadRequest, "Invalid request")
        return
    }
    id, err := uuid.Parse(req.ID)
    if err != nil {
        r.ErrorJSON(http.StatusBadRequest, "Invalid ID format")
        return
    }
    if err := r.State.PaymentMethodService.Delete(r.Request.Context(), id); err != nil {
        r.ErrorJSON(http.StatusInternalServerError, "Failed to delete payment method")
        return
    }
    r.SuccessJSON(map[string]any{"success": true})
}

// ActivatePaymentMethod activates a given payment method ID
func ActivatePaymentMethod(r *Request) {
    req := new(ActivatePaymentMethodRequest)
    if err := r.Bind(req); err != nil {
        r.ErrorJSON(http.StatusBadRequest, "Invalid request")
        return
    }
    id, err := uuid.Parse(req.ID)
    if err != nil {
        r.ErrorJSON(http.StatusBadRequest, "Invalid ID format")
        return
    }
    if err := r.State.PaymentMethodService.ActivateByID(r.Request.Context(), id); err != nil {
        r.ErrorJSON(http.StatusInternalServerError, "Failed to activate payment method")
        return
    }
    r.SuccessJSON(map[string]any{"success": true})
}
