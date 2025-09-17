package handlers

import (
	"context"
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
	var paymentMethods any
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

	// Convert to response format with enhanced information
	response := convertToPaymentMethodResponses(paymentMethods, r.State.PaymentMethodService)

	// Calculate total items (for now, we'll use the length of the result)
	// In a real pagination implementation, this would come from a separate count query
	totalItems := int64(len(response))

	r.SuccessJSON(PaginatedResponse{
		Data:       response,
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

	// Check if payment method can be deleted (business rules)
	canDelete, reason, err := r.State.PaymentMethodService.CanDelete(r.Request.Context(), id)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
		}).Error("Failed to check if payment method can be deleted")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to validate deletion request")
		return
	}

	if !canDelete {
		log.WithFields(log.Fields{
			"payment_method_id": id,
			"user_id":           user.ID,
			"reason":            reason,
		}).Warn("Payment method deletion blocked by business rules")
		r.ErrorJSON(http.StatusConflict, reason)
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
		"display_name":      paymentMethod.GetDisplayName(),
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
		"display_name":      paymentMethod.GetDisplayName(),
	}).Info("Payment method successfully activated")

	r.SuccessJSON(map[string]any{
		"success": true,
		"message": "Payment method activated successfully",
	})
}

// PaymentMethodDisplayService interface for getting display names and checking deletion rules
type PaymentMethodDisplayService interface {
	GetDisplayName(pm *models.PaymentMethod) string
	CanDelete(ctx context.Context, id uuid.UUID) (bool, string, error)
}

// convertToPaymentMethodResponses converts payment method models to enhanced response format
func convertToPaymentMethodResponses(paymentMethods interface{}, service PaymentMethodDisplayService) []PaymentMethodResponse {
	var responses []PaymentMethodResponse

	// Handle different types that might be passed in
	switch pm := paymentMethods.(type) {
	case []*models.PaymentMethod:
		responses = make([]PaymentMethodResponse, len(pm))
		for i, method := range pm {
			if method != nil {
				responses[i] = convertSinglePaymentMethod(method, service)
			}
		}
	case []models.PaymentMethod:
		responses = make([]PaymentMethodResponse, len(pm))
		for i, method := range pm {
			responses[i] = convertSinglePaymentMethod(&method, service)
		}
	default:
		// Return empty slice if unknown type
		responses = []PaymentMethodResponse{}
	}

	return responses
}

// GeneratePaymentMethodsFromWallets creates payment methods from connected Solana wallets
func GeneratePaymentMethodsFromWallets(r *Request) {
	user := r.GetUser()
	if user == nil {
		log.Error("User not found in request context")
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	// Create payment methods from all verified Solana wallets
	paymentMethods, err := r.State.PaymentMethodService.CreatePaymentMethodsFromConnectedWallets(
		r.Request.Context(),
		user.ID,
		r.State.SolanaWalletService,
	)
	if err != nil {
		log.WithError(err).WithField("user_id", user.ID).Error("Failed to generate payment methods from wallets")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate payment methods from wallets")
		return
	}

	// Convert to response format
	responses := make([]PaymentMethodResponse, len(paymentMethods))
	for i, pm := range paymentMethods {
		responses[i] = convertSinglePaymentMethod(pm, r.State.PaymentMethodService)
	}

	log.WithFields(log.Fields{
		"user_id": user.ID,
		"count":   len(responses),
	}).Info("Successfully generated payment methods from Solana wallets")

	r.SuccessJSON(map[string]any{
		"success":         true,
		"message":         "Payment methods generated from connected wallets",
		"payment_methods": responses,
		"count":           len(responses),
	})
}

// convertSinglePaymentMethod converts a single payment method to response format
func convertSinglePaymentMethod(pm *models.PaymentMethod, service PaymentMethodDisplayService) PaymentMethodResponse {
	response := PaymentMethodResponse{
		ID:          pm.ID.String(),
		Type:        pm.GetType(),
		Processor:   string(pm.Processor),
		IsActive:    pm.IsActive,
		DisplayName: service.GetDisplayName(pm),
		CreatedAt:   pm.CreatedAt,
	}

	// Add type-specific fields
	switch pm.Processor {
	case "mobius":
		// Mobius card fields
		response.LastFour = pm.LastFour
		response.CardType = pm.CardType
		response.ExpiryDate = pm.ExpiryDate
	case "solana":
		// Solana wallet fields
		response.WalletAddress = pm.WalletAddress
	case "ccbill":
		// CCBill subscription fields
		response.VaultID = &pm.VaultID
		response.InitialTransactionID = &pm.InitialTransactionID
	}

	// Add failure reason if present
	response.FailureReason = pm.FailureReason

	// Determine if payment method can be deleted
	// Use the service method to check business rules
	canDelete, _, err := service.CanDelete(context.Background(), pm.ID)
	if err != nil {
		// If we can't determine, default to false for safety
		canDelete = false
	}
	response.CanDelete = canDelete

	return response
}
