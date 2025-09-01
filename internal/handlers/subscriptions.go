package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

// Subscribe godoc
// @Summary Create a new subscription
// @Schemes
// @Description Creates a new subscription with the specified payment processor
// @Tags billing-subscriptions
// @Accept json
// @Produce json
// @Param req body handlers.SubscribeBodyParams true "Subscription data"
// @Success 201 {object} handlers.SubscribeResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/subscribe [post]
func (h *SubscriptionHandler) Subscribe(c *gin.Context) {
	var req SubscribeRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind subscribe request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithError(err).Error("Invalid price ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid price ID format"))
		return
	}

	serviceRequest := &services.SubscribeRequest{
		PriceID:      priceID,
		Processor:    req.Processor,
		PaymentToken: req.PaymentToken,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Email:        req.Email,
		Address1:     req.Address1,
		City:         req.City,
		State:        req.State,
		Zip:          req.Zip,
		Country:      req.Country,
	}

	result, err := h.subscriptionService.Subscribe(c.Request.Context(), serviceRequest, userCtx.User)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":   userCtx.User.ID,
			"processor": req.Processor,
		}).Error("Failed to create subscription")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to create subscription"))
		return
	}

	response := NewSubscribeResponse(result)
	c.JSON(http.StatusCreated, response)
}

// GenerateFlexFormURL godoc
// @Summary Generate CCBill FlexForm URL
// @Schemes
// @Description Generates a CCBill FlexForm URL for subscription payment
// @Tags billing-subscriptions
// @Accept json
// @Produce json
// @Param req body handlers.GenerateFlexFormURLBodyParams true "FlexForm data"
// @Success 200 {object} handlers.GenerateFlexFormURLResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/flexform [post]
func (h *SubscriptionHandler) GenerateFlexFormURL(c *gin.Context) {
	var req GenerateFlexFormURLRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind flex form request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithError(err).Error("Invalid price ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid price ID format"))
		return
	}

	serviceRequest := &services.FlexFormURLRequest{
		PriceID:   priceID,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Address1:  req.Address1,
		City:      req.City,
		State:     req.State,
		ZipCode:   req.ZipCode,
		Country:   req.Country,
	}

	result, err := h.subscriptionService.GenerateFlexFormURL(c.Request.Context(), serviceRequest, userCtx.User)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":  userCtx.User.ID,
			"price_id": req.PriceID,
		}).Error("Failed to generate FlexForm URL")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to generate FlexForm URL"))
		return
	}

	response := NewGenerateFlexFormURLResponse(result)
	c.JSON(http.StatusOK, response)
}

// GetActiveSubscription godoc
// @Summary Get user's active subscription
// @Schemes
// @Description Retrieves the user's currently active subscription
// @Tags billing-subscriptions
// @Accept json
// @Produce json
// @Success 200 {object} handlers.ActiveSubscriptionResponse
// @Success 204 "No active subscription"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/subscription/active [get]
func (h *SubscriptionHandler) GetActiveSubscription(c *gin.Context) {
	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	subscription, err := h.subscriptionService.GetActiveSubscription(c.Request.Context(), userCtx.User.ID)
	if err != nil {
		if errors.Is(err, services.ErrSubscriptionNotFound) {
			c.Status(http.StatusNoContent)
			return
		}
		log.WithError(err).WithField("user_id", userCtx.User.ID).Error("Failed to get active subscription")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to retrieve subscription"))
		return
	}

	response := NewActiveSubscriptionResponse(subscription)
	c.JSON(http.StatusOK, response)
}

// GetSubscriptionStatus godoc
// @Summary Get subscription status
// @Schemes
// @Description Gets the current status of user's subscription
// @Tags billing-subscriptions
// @Accept json
// @Produce json
// @Success 200 {object} handlers.ActiveSubscriptionResponse
// @Success 204 "No active subscription"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/subscription/status [get]
func (h *SubscriptionHandler) GetSubscriptionStatus(c *gin.Context) {
	h.GetActiveSubscription(c)
}

// GetSubscriptionHistory godoc
// @Summary Get user's subscription history
// @Schemes
// @Description Retrieves paginated history of user's subscriptions
// @Tags billing-subscriptions
// @Accept json
// @Produce json
// @Param req query handlers.GetSubscriptionHistoryQueryParams true "Query params"
// @Success 200 {object} handlers.GetSubscriptionHistoryResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/subscription/history [get]
func (h *SubscriptionHandler) GetSubscriptionHistory(c *gin.Context) {
	var req GetSubscriptionHistoryRequest
	err := c.ShouldBindQuery(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind subscription history request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	subscriptions, totalItems, err := h.subscriptionService.GetSubscriptionHistory(
		c.Request.Context(),
		userCtx.User.ID,
		req.StartDate,
		req.EndDate,
		req.Page,
		req.PageSize,
	)
	if err != nil {
		log.WithError(err).WithField("user_id", userCtx.User.ID).Error("Failed to get subscription history")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to retrieve subscription history"))
		return
	}

	response := NewGetSubscriptionHistoryResponse(subscriptions, req.Page, req.PageSize, totalItems)
	c.JSON(http.StatusOK, response)
}
