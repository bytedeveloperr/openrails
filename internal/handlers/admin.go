package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/api/validation"
	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

// ExtendSubscription godoc
// @Summary Extend user subscription (Admin)
// @Schemes
// @Description Extends a user's subscription by the specified number of days
// @Tags billing-admin
// @Accept json
// @Produce json
// @Security AdminAuth
// @Param req path handlers.ExtendSubscriptionPathParams true "Path params"
// @Param req body handlers.ExtendSubscriptionBodyParams true "Extension data"
// @Success 200 {object} handlers.ExtendSubscriptionResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 403 {object} message.MessageResponse "Forbidden - Admin access required"
// @Failure 404 {object} message.MessageResponse "User or subscription not found"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/admin/users/{user_id}/subscription/extend [post]
func (h *AdminHandler) ExtendSubscription(c *gin.Context) {
	req, err := validation.BindingInterface(c, &ExtendSubscriptionRequest{})
	if err != nil {
		log.WithError(err).Error("Failed to bind extend subscription request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	adminCtx := middleware.GetUserContext(c.Request.Context())
	if adminCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("Admin authentication required"))
		return
	}

	// Check admin permissions - assuming a role check
	if !adminCtx.User.IsAdmin {
		c.JSON(http.StatusForbidden, message.Message("Admin access required"))
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		log.WithError(err).Error("Invalid user ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid user ID format"))
		return
	}

	serviceRequest := &services.ExtendSubscriptionRequest{
		UserID:  userID,
		Days:    req.Days,
		Reason:  req.Reason,
		AdminID: adminCtx.User.ID,
	}

	result, err := h.subscriptionService.ExtendSubscription(c.Request.Context(), serviceRequest)
	if err != nil {
		if errors.Is(err, services.ErrSubscriptionNotFound) {
			c.JSON(http.StatusNotFound, message.Message("User subscription not found"))
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"admin_id": adminCtx.User.ID,
			"user_id":  userID,
			"days":     req.Days,
		}).Error("Failed to extend subscription")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to extend subscription"))
		return
	}

	response := NewExtendSubscriptionResponse(result.SubscriptionID, result.ExtendedUntil, "Subscription extended successfully")
	c.JSON(http.StatusOK, response)
}

// CancelSubscription godoc
// @Summary Cancel user subscription (Admin)
// @Schemes
// @Description Cancels a user's subscription immediately
// @Tags billing-admin
// @Accept json
// @Produce json
// @Security AdminAuth
// @Param req path handlers.CancelSubscriptionPathParams true "Path params"
// @Param req body handlers.CancelSubscriptionBodyParams true "Cancellation data"
// @Success 200 {object} handlers.CancelSubscriptionResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 403 {object} message.MessageResponse "Forbidden - Admin access required"
// @Failure 404 {object} message.MessageResponse "User or subscription not found"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/admin/users/{user_id}/subscription/cancel [post]
func (h *AdminHandler) CancelSubscription(c *gin.Context) {
	req, err := validation.BindingInterface(c, &CancelSubscriptionRequest{})
	if err != nil {
		log.WithError(err).Error("Failed to bind cancel subscription request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	adminCtx := middleware.GetUserContext(c.Request.Context())
	if adminCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("Admin authentication required"))
		return
	}

	if !adminCtx.User.IsAdmin {
		c.JSON(http.StatusForbidden, message.Message("Admin access required"))
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		log.WithError(err).Error("Invalid user ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid user ID format"))
		return
	}

	serviceRequest := &services.CancelSubscriptionRequest{
		UserID:  userID,
		Reason:  req.Reason,
		AdminID: adminCtx.User.ID,
	}

	err = h.subscriptionService.CancelSubscription(c.Request.Context(), serviceRequest)
	if err != nil {
		if errors.Is(err, services.ErrSubscriptionNotFound) {
			c.JSON(http.StatusNotFound, message.Message("User subscription not found"))
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"admin_id": adminCtx.User.ID,
			"user_id":  userID,
			"reason":   req.Reason,
		}).Error("Failed to cancel subscription")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to cancel subscription"))
		return
	}

	response := NewCancelSubscriptionResponse(true, "Subscription cancelled successfully")
	c.JSON(http.StatusOK, response)
}

// GetSubscriptionDetails godoc
// @Summary Get user subscription details (Admin)
// @Schemes
// @Description Retrieves detailed subscription information for a specific user
// @Tags billing-admin
// @Accept json
// @Produce json
// @Security AdminAuth
// @Param req path handlers.GetSubscriptionDetailsPathParams true "Path params"
// @Success 200 {object} handlers.GetSubscriptionDetailsResponse
// @Success 204 "No active subscription"
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 403 {object} message.MessageResponse "Forbidden - Admin access required"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/admin/users/{user_id}/subscription [get]
func (h *AdminHandler) GetSubscriptionDetails(c *gin.Context) {
	req, err := validation.BindingInterface(c, &GetSubscriptionDetailsRequest{})
	if err != nil {
		log.WithError(err).Error("Failed to bind get subscription details request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	adminCtx := middleware.GetUserContext(c.Request.Context())
	if adminCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("Admin authentication required"))
		return
	}

	if !adminCtx.User.IsAdmin {
		c.JSON(http.StatusForbidden, message.Message("Admin access required"))
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		log.WithError(err).Error("Invalid user ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid user ID format"))
		return
	}

	subscription, err := h.subscriptionService.GetActiveSubscription(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, services.ErrSubscriptionNotFound) {
			c.Status(http.StatusNoContent)
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"admin_id": adminCtx.User.ID,
			"user_id":  userID,
		}).Error("Failed to get subscription details")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to retrieve subscription details"))
		return
	}

	// Optionally get payment method details
	var paymentMethod *services.PaymentMethodResponse
	if subscription != nil {
		// This would require extending the service to get payment method by subscription
		// For now, we'll pass nil
		paymentMethod = nil
	}

	response := NewGetSubscriptionDetailsResponse(subscription, paymentMethod)
	c.JSON(http.StatusOK, response)
}

// ProcessRefund godoc
// @Summary Process refund for user (Admin)
// @Schemes
// @Description Processes a refund for a specific transaction
// @Tags billing-admin
// @Accept json
// @Produce json
// @Security AdminAuth
// @Param req path handlers.ProcessRefundPathParams true "Path params"
// @Param req body handlers.ProcessRefundBodyParams true "Refund data"
// @Success 200 {object} handlers.ProcessRefundResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 403 {object} message.MessageResponse "Forbidden - Admin access required"
// @Failure 404 {object} message.MessageResponse "Transaction not found"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/admin/users/{user_id}/refund [post]
func (h *AdminHandler) ProcessRefund(c *gin.Context) {
	req, err := validation.BindingInterface(c, &ProcessRefundRequest{})
	if err != nil {
		log.WithError(err).Error("Failed to bind process refund request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	adminCtx := middleware.GetUserContext(c.Request.Context())
	if adminCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("Admin authentication required"))
		return
	}

	if !adminCtx.User.IsAdmin {
		c.JSON(http.StatusForbidden, message.Message("Admin access required"))
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		log.WithError(err).Error("Invalid user ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid user ID format"))
		return
	}

	serviceRequest := &services.ProcessRefundRequest{
		UserID:        userID,
		TransactionID: req.TransactionID,
		Amount:        req.Amount,
		Reason:        req.Reason,
		AdminID:       adminCtx.User.ID,
	}

	result, err := h.subscriptionService.ProcessRefund(c.Request.Context(), serviceRequest)
	if err != nil {
		if errors.Is(err, services.ErrTransactionNotFound) {
			c.JSON(http.StatusNotFound, message.Message("Transaction not found"))
			return
		}
		log.WithError(err).WithFields(log.Fields{
			"admin_id":       adminCtx.User.ID,
			"user_id":        userID,
			"transaction_id": req.TransactionID,
			"amount":         req.Amount,
		}).Error("Failed to process refund")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to process refund"))
		return
	}

	response := NewProcessRefundResponse(result.RefundID, result.Amount, result.Status, "Refund processed successfully")
	c.JSON(http.StatusOK, response)
}
