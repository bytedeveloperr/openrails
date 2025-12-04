package handlers

import (
	"net/http"
	"strings"

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
	ccbillPriceID, hasCCBill := price.GetCCBillConfig()
	if !hasCCBill || ccbillPriceID == "" {
		log.WithField("price_id", priceID).Error("Price missing CCBill configuration")
		r.ErrorJSON(http.StatusBadRequest, "Price is not configured for CCBill")
		return
	}
	if userCtx.User.Email == nil || strings.TrimSpace(*userCtx.User.Email) == "" {
		log.Error("Authenticated user missing email for FlexForm request")
		r.ErrorJSON(http.StatusBadRequest, "Verified email required for CCBill payments")
		return
	}

	ccbillAliasService := r.State.CCBillAliasService
	if ccbillAliasService == nil {
		log.Error("CCBill alias service is not configured")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate payment form")
		return
	}

	alias, err := ccbillAliasService.GetOrCreate(r.Request.Context(), userCtx.User.ID)
	if err != nil {
		log.WithError(err).Error("Failed to create or fetch CCBill username alias")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate payment form")
		return
	}

	ccbillClient := ccbill.NewClient(r.State.Config.CCBill, r.State.Config.Env == "prod")
	flexFormParams := &ccbill.GenerateFlexFormURLParams{
		Username:      alias,
		Email:         *userCtx.User.Email,
		CustomerFName: req.FirstName,
		CustomerLName: req.LastName,
		Address1:      req.Address1,
		City:          req.City,
		State:         req.State,
		ZipCode:       req.ZipCode,
		Country:       req.Country,
		FlexID:        ccbillPriceID,
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
