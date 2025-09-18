package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// ListPaymentMethods returns the user's payment methods (optionally including inactive)
func ListPaymentMethods(r *Request) {
	req := new(ListPaymentMethodsRequest)
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind list payment methods request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request parameters")
		return
	}

	// Fallback if Bind doesn't populate query embedded struct
	if p := r.Request.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			req.Page = v
		} else if err != nil {
			log.WithError(err).WithField("page", p).Error("Invalid page parameter")
			r.ErrorJSON(http.StatusBadRequest, "Invalid page parameter - must be a positive integer")
			return
		}
	}
	if ps := r.Request.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 500 {
			req.PageSize = v
		} else if err != nil {
			log.WithError(err).WithField("page_size", ps).Error("Invalid page_size parameter")
			r.ErrorJSON(http.StatusBadRequest, "Invalid page_size parameter - must be a positive integer")
			return
		} else if v > 500 {
			log.WithField("page_size", v).Error("Page size too large")
			r.ErrorJSON(http.StatusBadRequest, "Page size cannot exceed 500")
			return
		}
	}

	req.SetDefaults()

	user := r.GetUser()
	if user == nil {
		log.Error("User not found in request context")
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	// Validate pagination parameters
	if req.Page < 1 {
		r.ErrorJSON(http.StatusBadRequest, "Page must be greater than 0")
		return
	}
	if req.PageSize < 1 || req.PageSize > 500 {
		r.ErrorJSON(http.StatusBadRequest, "Page size must be between 1 and 500")
		return
	}

	// Read methods via service
	var paymentMethods []*models.PaymentMethod
	var err error

	if req.IncludeInactive {
		paymentMethods, err = r.State.PaymentMethodService.GetByUserID(r.Request.Context(), user.ID)
	} else {
		paymentMethods, err = r.State.PaymentMethodService.GetActiveByUserID(r.Request.Context(), user.ID)
	}

	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":          user.ID,
			"include_inactive": req.IncludeInactive,
		}).Error("Failed to retrieve payment methods")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to retrieve payment methods")
		return
	}

	totalItems := int64(len(paymentMethods))
	r.SuccessJSON(PaginatedResponse{
		Data:       paymentMethods,
		TotalItems: totalItems,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: int((totalItems + int64(req.PageSize) - 1) / int64(req.PageSize)),
	})
}

// DeletePaymentMethod removes a payment method by ID
func DeletePaymentMethod(r *Request) {
	req := new(DeletePaymentMethodRequest)
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind delete payment method request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request parameters")
		return
	}

	// Validate UUID format
	id, err := uuid.Parse(req.ID)
	if err != nil {
		log.WithError(err).WithField("id", req.ID).Error("Invalid payment method ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid payment method ID format")
		return
	}

	user := r.GetUser()
	if user == nil {
		log.Error("User not found in request context")
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	// Validate ownership and get payment method details
	paymentMethod, err := r.State.PaymentMethodService.ValidatePaymentMethodOperation(r.Request.Context(), id, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || err.Error() == "payment method not found" {
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Payment method not found for deletion")
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		}
		if err.Error() == "payment method not found or access denied" {
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Unauthorized payment method deletion attempt")
			r.ErrorJSON(http.StatusForbidden, "Access denied - you don't own this payment method")
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Error("Failed to validate payment method ownership")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to validate payment method")
		return
	}

	// Perform the deletion
	if err := r.State.PaymentMethodService.Delete(r.Request.Context(), id); err != nil {
		if err.Error() == "no rows affected" {
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Payment method not found during deletion")
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Error("Failed to delete payment method")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to delete payment method")
		return
	}

	// Log successful deletion for audit purposes
	log.WithFields(log.Fields{
		"payment_method_id": id,
		"user_id":           user.ID,
		"processor":         paymentMethod.Processor,
	}).Info("Payment method successfully deleted")

	r.SuccessJSON(map[string]any{
		"success": true,
		"message": "Payment method deleted successfully",
	})
}

// ActivatePaymentMethod activates a given payment method ID
func ActivatePaymentMethod(r *Request) {
	req := new(ActivatePaymentMethodRequest)
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind activate payment method request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request parameters")
		return
	}

	// Validate UUID format
	id, err := uuid.Parse(req.ID)
	if err != nil {
		log.WithError(err).WithField("id", req.ID).Error("Invalid payment method ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid payment method ID format")
		return
	}

	user := r.GetUser()
	if user == nil {
		log.Error("User not found in request context")
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	// Validate ownership and get payment method details
	paymentMethod, err := r.State.PaymentMethodService.ValidatePaymentMethodOperation(r.Request.Context(), id, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || err.Error() == "payment method not found" {
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Payment method not found for activation")
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		}
		if err.Error() == "payment method not found or access denied" {
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Unauthorized payment method activation attempt")
			r.ErrorJSON(http.StatusForbidden, "Access denied - you don't own this payment method")
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Error("Failed to validate payment method ownership")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to validate payment method")
		return
	}

	// Check if payment method is already active
	if paymentMethod.IsActive {
		log.WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Info("Payment method is already active")
		r.SuccessJSON(map[string]any{
			"success": true,
			"message": "Payment method is already active",
		})
		return
	}

	// Check if payment method has failure reasons that prevent activation
	if paymentMethod.FailureReason != nil && *paymentMethod.FailureReason != "" {
		log.WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
			"failure_reason":    *paymentMethod.FailureReason,
		}).Warn("Cannot activate payment method with failure reason")
		r.ErrorJSON(http.StatusConflict, "Cannot activate payment method: "+*paymentMethod.FailureReason)
		return
	}

	// Perform the activation
	if err := r.State.PaymentMethodService.ActivateByID(r.Request.Context(), id); err != nil {
		if err.Error() == "no rows affected" {
			log.WithFields(log.Fields{
				"payment_method_id": id,
				"user_id":           user.ID,
			}).Warn("Payment method not found during activation")
			r.ErrorJSON(http.StatusNotFound, "Payment method not found")
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Error("Failed to activate payment method")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to activate payment method")
		return
	}

	// Log successful activation for audit purposes
	log.WithFields(log.Fields{
		"payment_method_id": id,
		"user_id":           user.ID,
		"processor":         paymentMethod.Processor,
	}).Info("Payment method successfully activated")

	r.SuccessJSON(map[string]any{
		"success": true,
		"message": "Payment method activated successfully",
	})
}
