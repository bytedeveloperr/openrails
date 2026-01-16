package handlers

import (
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/api"
	"github.com/doujins-org/doujins-billing/pkg/query"
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

	paymentID, err := api.ParsePaymentID(path.PaymentID)
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

	r.Inner().JSON(http.StatusCreated, PaymentToAPI(refund, nil))
}

// GetAdminPayments returns a paginated list of payments with filters
// GET /v1/admin/payments
func GetAdminPayments(r *Request) {
	queryOpts := query.QueryOptions[services.GetPaymentsFilters]{}
	if err := r.Inner().ShouldBindQuery(&queryOpts); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	if r.State.PaymentService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "payment service unavailable")
		return
	}

	payments, total, err := r.State.PaymentService.GetPayments(r.Request.Context(), queryOpts)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to PaymentObject list
	paymentObjects := make([]api.PaymentObject, len(payments))
	for i, p := range payments {
		paymentObjects[i] = PaymentToAPI(p, nil)
	}

	r.SuccessJSONPaginated(paymentObjects, total, queryOpts.Limit, queryOpts.Offset)
}

// GetAdminPayment returns a single payment with full details including refunds
// GET /v1/admin/payments/:id
func GetAdminPayment(r *Request) {
	var path PaymentPath
	if err := r.Inner().ShouldBindUri(&path); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	paymentID, err := api.ParsePaymentID(path.PaymentID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid payment ID")
		return
	}

	if r.State.PaymentService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "payment service unavailable")
		return
	}

	payment, refunds, err := r.State.PaymentService.GetByIDWithDetails(r.Request.Context(), paymentID)
	if err != nil {
		r.ErrorJSON(http.StatusNotFound, "payment not found")
		return
	}

	r.SuccessJSON(PaymentToAPI(payment, refunds))
}
