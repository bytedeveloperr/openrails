package handlers

import (
	"net/http"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/middleware"
)

func GenerateFlexFormURL(r *Request) {
	var req GenerateFlexFormURLRequest
	if !r.BindJSON(&req) {
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
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID")
		return
	}
	if price == nil {
		log.WithField("price_id", priceID).Warn("Price lookup returned nil result")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID")
		return
	}

	ccbillClient := ccbill.NewClient(r.State.Config.CCBill, r.State.Config.Env == "prod")
	flexFormParams := &ccbill.GenerateFlexFormURLParams{
		Username:      userCtx.User.ID,
		Email:         *userCtx.User.Email,
		CustomerFName: req.FirstName,
		CustomerLName: req.LastName,
		Address1:      req.Address1,
		City:          req.City,
		State:         req.State,
		ZipCode:       req.ZipCode,
		Country:       req.Country,
		FlexID:        *price.CCBillPriceID,
	}

	response, err := ccbillClient.GenerateFlexFormURL(flexFormParams)
	if err != nil {
		log.WithError(err).Error("Failed to generate FlexForm URL")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate payment form")
		return
	}

	log.WithFields(log.Fields{
		"user_id":  userCtx.User.ID,
		"price_id": price.ID,
	}).Info("Generated CCBill FlexForm URL")

	r.SuccessJSON(response)
}
