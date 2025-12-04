package handlers

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/middleware"
)

// GenerateCCBillUpgradeURL generates a CCBill FlexForm URL for upgrading an existing subscription.
// The user must have an active CCBill subscription to upgrade.
// POST /v1/subscriptions/ccbill/upgrade-url
func GenerateCCBillUpgradeURL(r *Request) {
	var req GenerateCCBillUpgradeURLRequest
	if !r.BindJSON(&req) {
		return
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	// Parse target price ID
	targetPriceID, err := uuid.Parse(req.TargetPriceID)
	if err != nil {
		log.WithError(err).Error("Invalid target price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid target_price_id format")
		return
	}

	// Get the target price and validate it has CCBill configuration
	targetPrice, err := r.State.PriceService.GetByID(r.Request.Context(), targetPriceID)
	if err != nil {
		log.WithError(err).Error("Failed to get target price information")
		r.ErrorJSON(http.StatusBadRequest, "Invalid target_price_id")
		return
	}
	if targetPrice == nil {
		log.WithField("price_id", targetPriceID).Warn("Target price lookup returned nil result")
		r.ErrorJSON(http.StatusBadRequest, "Invalid target_price_id")
		return
	}

	ccbillPriceID, hasCCBill := targetPrice.GetCCBillConfig()
	if !hasCCBill || ccbillPriceID == "" {
		log.WithField("price_id", targetPriceID).Error("Target price missing CCBill configuration")
		r.ErrorJSON(http.StatusBadRequest, "Target price is not configured for CCBill")
		return
	}

	// Get user's current active subscription
	subscription, err := r.State.SubscriptionService.GetByUserID(r.Request.Context(), userCtx.User.ID)
	if err != nil {
		log.WithError(err).WithField("user_id", userCtx.User.ID).Error("Failed to get user subscription")
		r.ErrorJSON(http.StatusBadRequest, "No active subscription found")
		return
	}

	// Verify user has an active CCBill subscription
	if subscription.Status != models.StatusActive {
		r.ErrorJSON(http.StatusBadRequest, "Subscription is not active")
		return
	}
	if subscription.Processor != models.ProcessorCCBill {
		r.ErrorJSON(http.StatusBadRequest, "Subscription is not a CCBill subscription")
		return
	}
	if subscription.ProcessorSubscriptionID == "" {
		log.WithField("subscription_id", subscription.ID).Error("CCBill subscription missing processor subscription ID")
		r.ErrorJSON(http.StatusBadRequest, "Subscription is missing CCBill reference")
		return
	}

	// Verify the target price is different from current price
	if subscription.PriceID == targetPriceID {
		r.ErrorJSON(http.StatusBadRequest, "Target price is the same as current subscription price")
		return
	}

	// Get user's email (required for CCBill)
	if userCtx.User.Email == nil || strings.TrimSpace(*userCtx.User.Email) == "" {
		log.Error("Authenticated user missing email for CCBill upgrade")
		r.ErrorJSON(http.StatusBadRequest, "Verified email required for CCBill payments")
		return
	}

	// Get or create CCBill username alias
	ccbillAliasService := r.State.CCBillAliasService
	if ccbillAliasService == nil {
		log.Error("CCBill alias service is not configured")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate upgrade form")
		return
	}

	alias, err := ccbillAliasService.GetOrCreate(r.Request.Context(), userCtx.User.ID)
	if err != nil {
		log.WithError(err).Error("Failed to get or create CCBill username alias")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate upgrade form")
		return
	}

	// Generate the upgrade FlexForm URL
	ccbillClient := ccbill.NewClient(r.State.Config.CCBill, r.State.Config.Env == "prod")
	upgradeParams := &ccbill.GenerateUpgradeFlexFormURLParams{
		Username:               alias,
		Email:                  *userCtx.User.Email,
		FlexID:                 ccbillPriceID,
		OriginalSubscriptionID: subscription.ProcessorSubscriptionID,
	}

	response, err := ccbillClient.GenerateUpgradeFlexFormURL(upgradeParams)
	if err != nil {
		log.WithError(err).Error("Failed to generate CCBill upgrade FlexForm URL")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate upgrade form")
		return
	}

	log.WithFields(log.Fields{
		"user_id":                   userCtx.User.ID,
		"subscription_id":           subscription.ID,
		"current_price_id":          subscription.PriceID,
		"target_price_id":           targetPriceID,
		"processor_subscription_id": subscription.ProcessorSubscriptionID,
	}).Info("Generated CCBill upgrade FlexForm URL")

	r.SuccessJSON(response)
}
