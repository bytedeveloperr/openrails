package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/supabase-community/gotrue-go/types"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
)

func GenerateFlexFormURL(r *Request) {
	var req *GenerateFlexFormURLRequest
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind FlexForm URL request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request")
		return
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithError(err).Error("Invalid price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	price, err := r.State.PriceService.GetByID(r.Request.Context(), priceID)
	if err != nil {
		log.WithError(err).Error("Failed to get price information")
		r.ErrorJSON(http.StatusNotFound, "Price not found")
		return
	}

	// Create CCBill client
	ccbillClient := ccbill.NewClient(r.State.Config.CCBill, r.State.Config.Env == "prod")

	// Prepare FlexForm parameters
	flexFormParams := &ccbill.GenerateFlexFormURLParams{
		Username:      *userCtx.User.Email,
		Email:         *userCtx.User.Email,
		CustomerFName: req.FirstName,
		CustomerLName: req.LastName,
		Address1:      req.Address1,
		City:          req.City,
		State:         req.State,
		ZipCode:       req.ZipCode,
		Country:       req.Country,
		FlexID:        *price.MobiusPlanID,
	}

	// Generate FlexForm URL
	flexFormResponse, err := ccbillClient.GenerateFlexFormURL(flexFormParams)
	if err != nil {
		log.WithError(err).Error("Failed to generate FlexForm URL")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate payment form")
		return
	}

	log.WithFields(log.Fields{
		"user_id":  userCtx.User.ID,
		"price_id": price.ID,
	}).Info("Generated CCBill FlexForm URL")

	r.SuccessJSON(flexFormResponse)
}

// generatePassword creates a secure password from user ID for CCBill account creation
func generatePassword(userID uuid.UUID) string {
	// Use first 12 characters of UUID as password (secure enough for CCBill requirements)
	return userID.String()[:12]
}

// createPendingSubscription creates a new subscription in pending status for tracking
func createPendingSubscription(ctx context.Context, subscriptionService *services.SubscriptionService, user *types.User, price *models.Price) (*models.Subscription, error) {
	// Check if user already has a subscription
	existingSubscription, err := subscriptionService.GetByUserID(ctx, user.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if existingSubscription != nil {
		// Return existing subscription
		return existingSubscription, nil
	}

	// Create new subscription
	subscription := &models.Subscription{
		ID:                      uuid.New(),
		UserID:                  user.ID,
		PriceID:                 price.ID,
		Status:                  models.StatusPending,
		StartedAt:               time.Now(),
		Processor:               models.ProcessorCCBill,
		ProcessorSubscriptionID: "", // Will be set by webhook
	}

	if err := subscriptionService.Create(ctx, subscription); err != nil {
		return nil, err
	}

	return subscription, nil
}

// getInitialPeriod returns the billing period in days, defaulting to 30 if not specified
func getInitialPeriod(price *models.Price) int {
	if price.BillingCycleDays != nil {
		return *price.BillingCycleDays
	}
	// Default to 30 days if not specified
	return 30
}
