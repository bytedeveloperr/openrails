package riverjobs

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/nmi"
	"github.com/doujins-org/doujins-billing/internal/jobs"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/google/uuid"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"
)

// FulfillPaymentWorker processes failed post-charge operations.
// When a card is charged successfully but subsequent operations fail,
// we enqueue a job here to retry the fulfillment.
type FulfillPaymentWorker struct {
	river.WorkerDefaults[jobs.FulfillPaymentArgs]
	DB         *db.DB
	NMIClients map[string]*nmi.NMIClient
}

func (FulfillPaymentWorker) Kind() string { return jobs.KindFulfillPayment }

func (w *FulfillPaymentWorker) Work(ctx context.Context, job *river.Job[jobs.FulfillPaymentArgs]) error {
	args := job.Args
	logEntry := log.WithContext(ctx).WithFields(log.Fields{
		"payment_id":       args.PaymentID,
		"user_id":          args.UserID,
		"fulfillment_type": args.FulfillmentType,
		"attempt":          job.Attempt,
	})

	logEntry.Info("FulfillPayment: processing fulfillment job")

	var err error
	switch args.FulfillmentType {
	case jobs.FulfillmentGrantEntitlements:
		err = w.grantEntitlements(ctx, &args)
	case jobs.FulfillmentRenewMembership:
		err = w.renewMembership(ctx, &args)
	case jobs.FulfillmentCreateSubscription:
		err = w.createSubscription(ctx, &args)
	default:
		return fmt.Errorf("unknown fulfillment type: %s", args.FulfillmentType)
	}

	if err != nil {
		logEntry.WithError(err).Error("FulfillPayment: fulfillment failed, will retry")
		return err // River will retry based on job config
	}

	logEntry.Info("FulfillPayment: fulfillment completed successfully")
	return nil
}

// grantEntitlements grants entitlements for a one-time purchase
func (w *FulfillPaymentWorker) grantEntitlements(ctx context.Context, args *jobs.FulfillPaymentArgs) error {
	logEntry := log.WithContext(ctx).WithFields(log.Fields{
		"payment_id": args.PaymentID,
		"user_id":    args.UserID,
		"price_id":   args.PriceID,
	})

	// Get price and product
	priceSvc := services.NewPriceService(w.DB)
	productSvc := services.NewProductService(w.DB)
	entitlementSvc := services.NewEntitlementService(w.DB)

	price, err := priceSvc.GetByID(ctx, args.PriceID)
	if err != nil {
		return fmt.Errorf("get price: %w", err)
	}

	product, err := productSvc.GetByID(ctx, price.ProductID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}

	if product.EntitlementsSpec == nil || len(product.EntitlementsSpec) == 0 {
		// No entitlements to grant - this is fine
		logEntry.Info("grantEntitlements: no entitlements spec on product, nothing to do")
		return nil
	}

	now := time.Now()
	paymentID := args.PaymentID

	for entitlementName, durationDays := range product.EntitlementsSpec {
		// Check if entitlement already exists (idempotency)
		existingCount, _ := w.DB.GetDB().NewSelect().
			TableExpr("billing.entitlements").
			Where("source_id = ?", paymentID).
			Where("source_type = ?", models.EntitlementSourceOneOff).
			Where("entitlement = ?", entitlementName).
			Where("deleted_at IS NULL").
			Count(ctx)
		if existingCount > 0 {
			logEntry.WithField("entitlement", entitlementName).Info("grantEntitlements: entitlement already exists, skipping")
			continue // Already granted
		}

		startAt := now
		var endAt *time.Time
		if durationDays != nil && *durationDays > 0 {
			end := startAt.Add(time.Duration(*durationDays) * 24 * time.Hour)
			endAt = &end
		}

		_, err := entitlementSvc.GrantWindow(
			ctx,
			args.UserID,
			entitlementName,
			startAt,
			endAt,
			models.EntitlementSourceOneOff,
			&paymentID,
		)
		if err != nil {
			return fmt.Errorf("grant entitlement %s: %w", entitlementName, err)
		}
		logEntry.WithField("entitlement", entitlementName).Info("grantEntitlements: granted entitlement")
	}

	return nil
}

// renewMembership extends subscription period after successful rebill
func (w *FulfillPaymentWorker) renewMembership(ctx context.Context, args *jobs.FulfillPaymentArgs) error {
	logEntry := log.WithContext(ctx).WithFields(log.Fields{
		"payment_id":             args.PaymentID,
		"user_id":                args.UserID,
		"processor_subscription": args.ProcessorSubscriptionID,
	})

	if args.ProcessorSubscriptionID == "" {
		return fmt.Errorf("processor_subscription_id required for renew_membership")
	}

	// Build lifecycle service
	priceSvc := services.NewPriceService(w.DB)
	productSvc := services.NewProductService(w.DB)
	entitlementSvc := services.NewEntitlementService(w.DB)
	notifSvc := services.NewNotificationService(w.DB, nil)
	paymentSvc := services.NewPaymentService(w.DB)
	lifecycle := services.NewSubscriptionLifecycleService(w.DB, productSvc, priceSvc, entitlementSvc, notifSvc, paymentSvc, nil)

	// Check if the subscription was already renewed (idempotency)
	// Look for entitlements granted from this subscription after the payment was made
	subscriptionSvc := services.NewSubscriptionService(w.DB, priceSvc, productSvc, notifSvc, nil, nil, nil)
	sub, err := subscriptionSvc.GetByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), "mobius", args.ProcessorSubscriptionID)
	if err != nil {
		return fmt.Errorf("get subscription by processor ID: %w", err)
	}

	// Check if the subscription's current period already reflects this renewal
	// (i.e., period was extended after our payment was created)
	payment, err := paymentSvc.GetByID(ctx, args.PaymentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get payment: %w", err)
	}

	if payment != nil && sub.CurrentPeriodStartsAt != nil && sub.CurrentPeriodStartsAt.After(payment.PurchasedAt) {
		// Subscription period was already extended after this payment
		logEntry.Info("renewMembership: subscription already renewed after this payment, skipping")
		return nil
	}

	logEntry.Info("renewMembership: calling RenewMembership")
	return lifecycle.RenewMembership(ctx, &services.RenewMembershipParams{
		Processor:               models.ProcessorMobius,
		ProcessorSubscriptionID: args.ProcessorSubscriptionID,
	})
}

// createSubscription creates subscription record after NMI subscription creation failed or
// local DB record creation failed. This handles upgrade scenarios.
func (w *FulfillPaymentWorker) createSubscription(ctx context.Context, args *jobs.FulfillPaymentArgs) error {
	logEntry := log.WithContext(ctx).WithFields(log.Fields{
		"payment_id":      args.PaymentID,
		"user_id":         args.UserID,
		"price_id":        args.PriceID,
		"subscription_id": args.SubscriptionID,
	})

	// Build services
	priceSvc := services.NewPriceService(w.DB)
	productSvc := services.NewProductService(w.DB)
	subscriptionSvc := services.NewSubscriptionService(w.DB, priceSvc, productSvc, nil, nil, nil, nil)
	entitlementSvc := services.NewEntitlementService(w.DB)
	notifSvc := services.NewNotificationService(w.DB, nil)
	paymentSvc := services.NewPaymentService(w.DB)

	// Get price and product
	price, err := priceSvc.GetByID(ctx, args.PriceID)
	if err != nil {
		return fmt.Errorf("get price: %w", err)
	}

	product, err := productSvc.GetByID(ctx, price.ProductID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}

	// STEP 1: Check if user already has an active/pending subscription for this product
	// (maybe they successfully retried via the UI, or another process fixed it)
	if product.TierGroup != nil && *product.TierGroup != "" {
		existingSub, err := subscriptionSvc.GetActiveOrPendingByUserIDAndTierGroup(ctx, args.UserID, *product.TierGroup)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check existing subscription: %w", err)
		}

		if existingSub != nil && existingSub.PriceID == args.PriceID {
			// User already has an active subscription for this exact price - we're done
			logEntry.WithField("existing_subscription_id", existingSub.ID).
				Info("createSubscription: user already has active subscription for this price, nothing to do")
			return nil
		}

		if existingSub != nil {
			// User has a subscription for a different price in the same tier group
			// This might be their old subscription or a different tier - log and continue
			logEntry.WithFields(log.Fields{
				"existing_subscription_id": existingSub.ID,
				"existing_price_id":        existingSub.PriceID,
			}).Info("createSubscription: user has subscription for different price in tier group")
		}
	}

	// Also check by product ID directly
	var existingSubForProduct models.Subscription
	err = w.DB.GetDB().NewSelect().
		Model(&existingSubForProduct).
		Where("user_id = ?", args.UserID).
		Where("product_id = ?", product.ID).
		Where("status IN (?)", []string{string(models.StatusActive), string(models.StatusPending)}).
		Limit(1).
		Scan(ctx)
	if err == nil {
		// Found existing subscription for this product
		logEntry.WithField("existing_subscription_id", existingSubForProduct.ID).
			Info("createSubscription: user already has active/pending subscription for this product, nothing to do")
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check existing subscription by product: %w", err)
	}

	// STEP 2: Try to find subscription at NMI
	nmiPlanID, provider, hasNMI := price.GetNMIConfig()
	if !hasNMI {
		return fmt.Errorf("price %s is not NMI-backed", args.PriceID)
	}

	client := w.NMIClients[provider]
	if client == nil {
		return fmt.Errorf("NMI provider '%s' not configured", provider)
	}

	// Query NMI for subscriptions with this plan
	nmiSubscriptionID, err := w.findNMISubscription(ctx, client, nmiPlanID, args.UserID)
	if err != nil {
		logEntry.WithError(err).Warn("createSubscription: failed to query NMI for existing subscription")
		// Continue - we'll try to create a new one
	}

	now := time.Now()

	if nmiSubscriptionID != "" {
		// Found existing NMI subscription - create local record
		logEntry.WithField("nmi_subscription_id", nmiSubscriptionID).
			Info("createSubscription: found existing NMI subscription, creating local record")

		return w.createLocalSubscriptionRecord(ctx, args, price, product, nmiSubscriptionID, now,
			subscriptionSvc, entitlementSvc, notifSvc, paymentSvc)
	}

	// STEP 3: No subscription at NMI - create new one
	logEntry.Info("createSubscription: no NMI subscription found, creating new subscription")

	// We need payment method info - get from the old subscription if available
	var customerVaultID string
	if args.SubscriptionID != nil {
		oldSub, err := subscriptionSvc.GetByID(ctx, *args.SubscriptionID)
		if err == nil && oldSub.PaymentMethod != nil {
			customerVaultID = oldSub.PaymentMethod.VaultID
		}
	}

	if customerVaultID == "" {
		// Try to find user's default payment method
		var pm models.PaymentMethod
		err = w.DB.GetDB().NewSelect().
			Model(&pm).
			Where("user_id = ?", args.UserID).
			Where("is_active = true").
			Where("vault_id IS NOT NULL AND vault_id != ''").
			Order("created_at DESC").
			Limit(1).
			Scan(ctx)
		if err == nil {
			customerVaultID = pm.VaultID
		}
	}

	if customerVaultID == "" {
		return fmt.Errorf("no payment method found for user - cannot create NMI subscription")
	}

	// Create subscription at NMI
	newSubID := uuid.New()
	params := nmi.RecurringPaymentData{
		CardUserData: nmi.CardUserData{
			FirstName: "N/A",
			LastName:  "N/A",
			Address1:  "N/A",
			City:      "N/A",
			State:     "N/A",
			Zip:       "00000",
			Country:   "US",
		},
		PlanID:          nmiPlanID,
		CustomerVaultID: customerVaultID,
		Amount:          float64(price.Amount) / 100.0,
		Currency:        price.Currency,
		OrderID:         newSubID.String(),
		PONumber:        newSubID.String(),
		CustomerID:      args.UserID,
		// Start immediately - user already paid proration
		StartDate: now.Format("20060102"),
	}

	resp, err := client.AddRecurringSubscription(params)
	if err != nil {
		return fmt.Errorf("create NMI subscription: %w", err)
	}

	logEntry.WithField("nmi_subscription_id", resp.SubscriptionID).
		Info("createSubscription: created NMI subscription")

	return w.createLocalSubscriptionRecord(ctx, args, price, product, resp.SubscriptionID, now,
		subscriptionSvc, entitlementSvc, notifSvc, paymentSvc)
}

// findNMISubscription searches NMI for a subscription matching our user and plan
func (w *FulfillPaymentWorker) findNMISubscription(ctx context.Context, client *nmi.NMIClient, planID, userID string) (string, error) {
	// Query NMI for subscriptions with this plan
	response, err := client.SearchSubscriptions(nmi.SubscriptionQueryFilter{
		PlanID: planID,
	})
	if err != nil {
		return "", fmt.Errorf("query NMI subscriptions: %w", err)
	}

	// Parse XML response to find matching subscription
	// NMI returns XML like: <nm_response><subscription>...</subscription></nm_response>
	// The customerid field we pass when creating maps to "customerid" in the response
	type nmiSubscription struct {
		SubscriptionID string `xml:"subscription_id"`
		CustomerID     string `xml:"customerid"` // Our user.ID passed when creating subscription
		PlanID         string `xml:"plan_id"`
		Status         string `xml:"status"`
	}
	type nmiResponse struct {
		Subscriptions []nmiSubscription `xml:"subscription"`
	}

	var parsed nmiResponse
	if err := xml.Unmarshal([]byte(response), &parsed); err != nil {
		return "", fmt.Errorf("parse NMI response: %w", err)
	}

	// Find subscription matching our user
	for _, sub := range parsed.Subscriptions {
		if sub.CustomerID == userID && strings.EqualFold(sub.Status, "active") {
			return sub.SubscriptionID, nil
		}
	}

	return "", nil
}

// createLocalSubscriptionRecord creates the local DB record for a subscription
func (w *FulfillPaymentWorker) createLocalSubscriptionRecord(
	ctx context.Context,
	args *jobs.FulfillPaymentArgs,
	price *models.Price,
	product *models.Product,
	nmiSubscriptionID string,
	now time.Time,
	subscriptionSvc *services.SubscriptionService,
	entitlementSvc *services.EntitlementService,
	notifSvc *services.NotificationService,
	paymentSvc *services.PaymentService,
) error {
	logEntry := log.WithContext(ctx).WithFields(log.Fields{
		"user_id":             args.UserID,
		"price_id":            args.PriceID,
		"nmi_subscription_id": nmiSubscriptionID,
	})

	// Calculate period dates
	billingCycleDays := 30
	if price.BillingCycleDays != nil {
		billingCycleDays = *price.BillingCycleDays
	}
	periodEnd := now.Add(time.Duration(billingCycleDays) * 24 * time.Hour)

	// Find payment method
	var paymentMethodID *uuid.UUID
	var pm models.PaymentMethod
	err := w.DB.GetDB().NewSelect().
		Model(&pm).
		Where("user_id = ?", args.UserID).
		Where("is_active = true").
		Where("vault_id IS NOT NULL AND vault_id != ''").
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)
	if err == nil {
		paymentMethodID = &pm.ID
	}

	// Create subscription
	newSubID := uuid.New()
	sub := &models.Subscription{
		ID:                      newSubID,
		UserID:                  args.UserID,
		ProductID:               product.ID,
		PriceID:                 args.PriceID,
		ProcessorSubscriptionID: nmiSubscriptionID,
		Processor:               models.ProcessorMobius,
		PaymentMethodID:         paymentMethodID,
		Status:                  models.StatusActive,
		CurrentPeriodStartsAt:   &now,
		CurrentPeriodEndsAt:     &periodEnd,
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	if err := subscriptionSvc.Create(ctx, sub); err != nil {
		return fmt.Errorf("create subscription record: %w", err)
	}

	logEntry.WithField("subscription_id", newSubID).Info("createSubscription: created local subscription record")

	// Grant entitlements
	lifecycle := services.NewSubscriptionLifecycleService(w.DB, nil, nil, entitlementSvc, notifSvc, paymentSvc, nil)
	if product.EntitlementsSpec != nil {
		for entitlementName, durationDays := range product.EntitlementsSpec {
			// Check if already granted
			existingCount, _ := w.DB.GetDB().NewSelect().
				TableExpr("billing.entitlements").
				Where("source_id = ?", newSubID).
				Where("source_type = ?", models.EntitlementSourceSubscription).
				Where("entitlement = ?", entitlementName).
				Where("deleted_at IS NULL").
				Count(ctx)
			if existingCount > 0 {
				continue
			}

			var endAt *time.Time
			if durationDays != nil && *durationDays > 0 {
				end := now.Add(time.Duration(*durationDays) * 24 * time.Hour)
				endAt = &end
			} else {
				endAt = &periodEnd
			}

			_, err := entitlementSvc.GrantWindow(
				ctx,
				args.UserID,
				entitlementName,
				now,
				endAt,
				models.EntitlementSourceSubscription,
				&newSubID,
			)
			if err != nil {
				logEntry.WithError(err).WithField("entitlement", entitlementName).
					Warn("createSubscription: failed to grant entitlement")
			}
		}
	}

	// Mark old subscription as cancelled (upgrade scenario)
	if args.SubscriptionID != nil {
		cancelType := models.CancelType("upgrade")
		_, err := w.DB.GetDB().NewUpdate().
			Table("billing.subscriptions").
			Set("status = ?", models.StatusCancelled).
			Set("cancelled_at = ?", now).
			Set("cancel_type = ?", cancelType).
			Set("updated_at = ?", now).
			Where("id = ?", *args.SubscriptionID).
			Where("status != ?", models.StatusCancelled). // Don't update if already cancelled
			Exec(ctx)
		if err != nil {
			logEntry.WithError(err).WithField("old_subscription_id", *args.SubscriptionID).
				Warn("createSubscription: failed to cancel old subscription")
		}
	}

	_ = lifecycle // silence unused warning if entitlements logic doesn't use it

	return nil
}
