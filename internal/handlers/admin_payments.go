package handlers

import (
	"net/http"

	"github.com/google/uuid"
)

// PaymentPath is the path parameter for single payment operations
type PaymentPath struct {
	PaymentID string `uri:"id" binding:"required"`
}

// RefundRequest is the request body for refunding a payment
type RefundRequest struct {
	Amount              int64  `json:"amount" binding:"required,gt=0"` // Amount in cents to refund
	RefundTransactionID string `json:"refund_transaction_id" binding:"required"`
}

// AdminRefundPayment issues a refund for a payment
// POST /v1/admin/payments/:id/refund
func AdminRefundPayment(r *Request) {
	var path PaymentPath
	if err := r.Inner().ShouldBindUri(&path); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	paymentID, err := uuid.Parse(path.PaymentID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid payment ID")
		return
	}

	var req RefundRequest
	if !r.BindJSON(&req) {
		return
	}

	if r.State.PaymentService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "payment service unavailable")
		return
	}

	refund, err := r.State.PaymentService.Refund(r.Request.Context(), paymentID, req.RefundTransactionID, req.Amount)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	r.Inner().JSON(http.StatusCreated, refund)
}
