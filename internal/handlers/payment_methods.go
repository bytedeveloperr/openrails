package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

// CreatePaymentMethod godoc
// @Summary Create a new payment method
// @Schemes
// @Description Creates a new payment method with card details
// @Tags billing-payment-methods
// @Accept json
// @Produce json
// @Param req body handlers.CreatePaymentMethodBodyParams true "Payment method creation data"
// @Success 201 {object} handlers.CreatePaymentMethodResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/payment-methods [post]
func (h *PaymentMethodHandler) CreatePaymentMethod(c *gin.Context) {
	var req CreatePaymentMethodRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind create payment method request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	serviceRequest := &services.CreatePaymentMethodRequest{
		PaymentToken: req.PaymentToken,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Address1:     req.Address1,
		City:         req.City,
		State:        req.State,
		Zip:          req.Zip,
		Country:      req.Country,
		Email:        req.Email,
	}

	result, err := h.paymentMethodService.CreatePaymentMethod(c.Request.Context(), serviceRequest, userCtx.User.ID)
	if err != nil {
		log.WithError(err).WithField("user_id", userCtx.User.ID).Error("Failed to create payment method")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to create payment method"))
		return
	}

	response := NewCreatePaymentMethodResponse(result, "Payment method created successfully")
	c.JSON(http.StatusCreated, response)
}

// ListPaymentMethods godoc
// @Summary List user's payment methods
// @Schemes
// @Description Retrieves list of user's payment methods
// @Tags billing-payment-methods
// @Accept json
// @Produce json
// @Param req query handlers.ListPaymentMethodsQueryParams true "Query params"
// @Success 200 {object} handlers.ListPaymentMethodsResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/payment-methods [get]
func (h *PaymentMethodHandler) ListPaymentMethods(c *gin.Context) {
	var req ListPaymentMethodsRequest
	err := c.ShouldBindQuery(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind list payment methods request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	paymentMethods, totalItems, err := h.paymentMethodService.ListPaymentMethods(
		c.Request.Context(),
		userCtx.User.ID,
		req.IncludeInactive,
	)
	if err != nil {
		log.WithError(err).WithField("user_id", userCtx.User.ID).Error("Failed to get payment methods")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to retrieve payment methods"))
		return
	}

	response := NewListPaymentMethodsResponse(paymentMethods, 1, len(paymentMethods), totalItems)
	c.JSON(http.StatusOK, response)
}

// UpdatePaymentMethod godoc
// @Summary Update an existing payment method
// @Schemes
// @Description Updates card details for an existing payment method
// @Tags billing-payment-methods
// @Accept json
// @Produce json
// @Param req path handlers.UpdatePaymentMethodPathParams true "Path params"
// @Param req body handlers.UpdatePaymentMethodBodyParams true "Payment method update data"
// @Success 200 {object} handlers.UpdatePaymentMethodResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 404 {object} message.MessageResponse "Payment method not found"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/payment-methods/{id} [put]
func (h *PaymentMethodHandler) UpdatePaymentMethod(c *gin.Context) {
	var req UpdatePaymentMethodRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind update payment method request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	paymentMethodID, err := uuid.Parse(req.PaymentMethodID)
	if err != nil {
		log.WithError(err).Error("Invalid payment method ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid payment method ID format"))
		return
	}

	serviceRequest := &services.UpdatePaymentMethodRequest{}
	if req.CCNumber != "" {
		serviceRequest.CCNumber = &req.CCNumber
	}
	if req.CCExp != "" {
		serviceRequest.CCExp = &req.CCExp
	}
	if req.FirstName != "" {
		serviceRequest.FirstName = &req.FirstName
	}
	if req.LastName != "" {
		serviceRequest.LastName = &req.LastName
	}
	if req.Address1 != "" {
		serviceRequest.Address1 = &req.Address1
	}
	if req.City != "" {
		serviceRequest.City = &req.City
	}
	if req.State != "" {
		serviceRequest.State = &req.State
	}
	if req.Zip != "" {
		serviceRequest.Zip = &req.Zip
	}
	if req.Country != "" {
		serviceRequest.Country = &req.Country
	}
	if req.Phone != "" {
		serviceRequest.Phone = &req.Phone
	}
	if req.Email != "" {
		serviceRequest.Email = &req.Email
	}
	if req.Company != "" {
		serviceRequest.Company = &req.Company
	}
	if req.Address2 != "" {
		serviceRequest.Address2 = &req.Address2
	}

	result, err := h.paymentMethodService.UpdatePaymentMethod(c.Request.Context(), paymentMethodID, userCtx.User.ID, serviceRequest)
	if err != nil {
		if errors.Is(err, services.ErrPaymentMethodNotFound) {
			c.JSON(http.StatusNotFound, message.Message("Payment method not found"))
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"user_id":           userCtx.User.ID,
			"payment_method_id": paymentMethodID,
		}).Error("Failed to update payment method")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to update payment method"))
		return
	}

	response := NewUpdatePaymentMethodResponse(result, "Payment method updated successfully")
	c.JSON(http.StatusOK, response)
}

// ActivatePaymentMethod godoc
// @Summary Activate a payment method as primary
// @Schemes
// @Description Sets a payment method as the primary method for subscriptions
// @Tags billing-payment-methods
// @Accept json
// @Produce json
// @Param req path handlers.ActivatePaymentMethodPathParams true "Path params"
// @Success 200 {object} handlers.ActivatePaymentMethodResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 404 {object} message.MessageResponse "Payment method not found"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/payment-methods/{id}/activate [post]
func (h *PaymentMethodHandler) ActivatePaymentMethod(c *gin.Context) {
	var req ActivatePaymentMethodRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind activate payment method request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	paymentMethodID, err := uuid.Parse(req.PaymentMethodID)
	if err != nil {
		log.WithError(err).Error("Invalid payment method ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid payment method ID format"))
		return
	}

	result, err := h.paymentMethodService.ActivatePaymentMethod(c.Request.Context(), paymentMethodID, userCtx.User.ID)
	if err != nil {
		if errors.Is(err, services.ErrPaymentMethodNotFound) {
			c.JSON(http.StatusNotFound, message.Message("Payment method not found"))
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"user_id":           userCtx.User.ID,
			"payment_method_id": paymentMethodID,
		}).Error("Failed to activate payment method")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to activate payment method"))
		return
	}

	response := NewActivatePaymentMethodResponse(result, "Payment method activated successfully")
	c.JSON(http.StatusOK, response)
}

// DeletePaymentMethod godoc
// @Summary Delete a payment method
// @Schemes
// @Description Deletes a payment method and deactivates it locally
// @Tags billing-payment-methods
// @Accept json
// @Produce json
// @Param req path handlers.DeletePaymentMethodPathParams true "Path params"
// @Success 200 {object} handlers.DeletePaymentMethodResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 404 {object} message.MessageResponse "Payment method not found"
// @Failure 409 {object} message.MessageResponse "Cannot delete payment method in use"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/payment-methods/{id} [delete]
func (h *PaymentMethodHandler) DeletePaymentMethod(c *gin.Context) {
	var req DeletePaymentMethodRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind delete payment method request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	paymentMethodID, err := uuid.Parse(req.PaymentMethodID)
	if err != nil {
		log.WithError(err).Error("Invalid payment method ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid payment method ID format"))
		return
	}

	err = h.paymentMethodService.DeletePaymentMethod(c.Request.Context(), paymentMethodID, userCtx.User.ID)
	if err != nil {
		if errors.Is(err, services.ErrPaymentMethodNotFound) {
			c.JSON(http.StatusNotFound, message.Message("Payment method not found"))
			return
		}
		if strings.Contains(err.Error(), "active subscription(s) are using this payment method") {
			c.JSON(http.StatusConflict, message.Message("Cannot delete payment method: active subscriptions are using this payment method"))
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"user_id":           userCtx.User.ID,
			"payment_method_id": paymentMethodID,
		}).Error("Failed to delete payment method")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to delete payment method"))
		return
	}

	response := NewDeletePaymentMethodResponse(true, "Payment method deleted successfully")
	c.JSON(http.StatusOK, response)
}
