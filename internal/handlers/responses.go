package handlers

import (
	"strconv"
	"strings"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/api"
	"github.com/doujins-org/ginapi/response"
)

type GetSubscriptionResponse = services.UserSubscriptionResponse

type SubscribeResponse = services.SubscribeResponse

// GetProductsResponse is now a Stripe-like list response
type GetProductsResponse = response.List[api.ProductObject]

// GetPricesResponse is a Stripe-like list response for prices
type GetPricesResponse = response.List[api.PriceObject]

// ProductToAPI converts a models.Product to api.ProductObject
func ProductToAPI(p *models.Product, prices []*models.Price) api.ProductObject {
	priceObjects := make([]api.PriceObject, len(prices))
	for i, price := range prices {
		priceObjects[i] = PriceToAPI(price)
	}

	return api.ProductObject{
		ID:          api.FormatProductID(p.ID),
		Object:      "product",
		Name:        p.DisplayName,
		Description: p.Description,
		Active:      p.IsActive,
		Livemode:    false,
		Metadata:    map[string]string{},
		Created:     api.ToUnix(p.CreatedAt),
		Updated:     api.ToUnix(p.UpdatedAt),
		Prices:      priceObjects,
	}
}

// PaymentToAPI converts a models.Payment to api.PaymentObject
// If refunds is provided, it calculates amount_refunded and includes the refunds list
func PaymentToAPI(p *models.Payment, refunds []*models.Payment) api.PaymentObject {
	var subID *string
	if p.SubscriptionID != nil {
		s := api.FormatSubscriptionID(*p.SubscriptionID)
		subID = &s
	}

	// Calculate refund totals
	var amountRefunded int64
	var refundObjects []api.PaymentObject
	if refunds != nil {
		for _, r := range refunds {
			// Refunds have negative amounts, so we negate to get positive refund amount
			if r.Amount < 0 {
				amountRefunded += -r.Amount
			} else {
				amountRefunded += r.Amount
			}
			refundObjects = append(refundObjects, PaymentToAPI(r, nil))
		}
	}

	status := "succeeded"
	refunded := amountRefunded >= p.Amount && p.Amount > 0
	if refunded {
		status = "refunded"
	} else if amountRefunded > 0 {
		status = "partially_refunded"
	}

	payment := api.PaymentObject{
		ID:             api.FormatPaymentID(p.ID),
		Object:         "charge",
		Status:         status,
		Amount:         p.Amount,
		AmountRefunded: amountRefunded,
		Currency:       p.Currency,
		User:           api.FormatUserID(p.UserID),
		Subscription:   subID,
		Processor:      string(p.Processor),
		TransactionID:  p.TransactionID,
		Refunded:       refunded,
		Captured:       true,
		Created:        api.ToUnix(p.CreatedAt),
	}

	// Include refunds list if provided (always include for single payment detail view)
	if refunds != nil {
		// Ensure Data is never nil (use empty slice if no refunds)
		if refundObjects == nil {
			refundObjects = []api.PaymentObject{}
		}
		payment.Refunds = &api.PaymentRefundsList{
			Object: "list",
			Data:   refundObjects,
		}
	}

	// Include expanded price if available
	if p.Price != nil {
		priceObj := PriceToAPI(p.Price)
		payment.Price = &priceObj
	}

	return payment
}

// PriceToAPI converts a models.Price to api.PriceObject
func PriceToAPI(p *models.Price) api.PriceObject {
	var recurring *api.RecurringInfo
	if p.BillingCycleDays != nil && *p.BillingCycleDays > 0 {
		interval, intervalCount := billingCycleDaysToInterval(*p.BillingCycleDays)
		recurring = &api.RecurringInfo{
			Interval:      interval,
			IntervalCount: intervalCount,
		}
	}

	priceType := "one_time"
	if recurring != nil {
		priceType = "recurring"
	}

	return api.PriceObject{
		ID:        api.FormatPriceID(p.ID),
		Object:    "price",
		Name:      p.DisplayName,
		Amount:    p.Amount,
		Currency:  p.Currency,
		Type:      priceType,
		Recurring: recurring,
		Product:   api.FormatProductID(p.ProductID),
		Active:    p.IsActive,
		Livemode:  false,
		Metadata:  map[string]string{},
		Created:   api.ToUnix(p.CreatedAt),
	}
}

// billingCycleDaysToInterval converts billing cycle days to Stripe-style interval
func billingCycleDaysToInterval(days int) (string, int) {
	switch {
	case days == 1:
		return "day", 1
	case days == 7:
		return "week", 1
	case days >= 28 && days <= 31:
		return "month", 1
	case days >= 365 && days <= 366:
		return "year", 1
	case days%365 == 0:
		return "year", days / 365
	case days%30 == 0:
		return "month", days / 30
	case days%7 == 0:
		return "week", days / 7
	default:
		return "day", days
	}
}

// -------------------------------- Solana / Payments Responses --------------------------------

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// SupportedTokensResponse lists available Solana tokens from config
type SupportedTokensResponse struct {
	Tokens []TokenInfo `json:"tokens"`
}

type TokenInfo struct {
	Symbol   string  `json:"symbol"`
	Name     string  `json:"name"`
	Mint     string  `json:"mint"`
	Decimals int     `json:"decimals"`
	Price    float64 `json:"price"`
}

type PublicPriceResponse struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Amount    int64              `json:"amount"` // Amount in cents
	Currency  string             `json:"currency"`
	Recurring *api.RecurringInfo `json:"recurring,omitempty"`
}

// (Removed) NMI setup response and nonce: no longer needed because
// the Collect.js tokenization key is injected into the frontend template.

type CancelSubscriptionResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type GenerateFlexFormURLResponse struct {
	RedirectURL string `json:"redirect_url" binding:"required"`
}

// -------------------------------- Enhanced Billing History Responses --------------------------------

// SubscriptionHistoryItem represents a subscription record with price details
type SubscriptionHistoryItem struct {
	ID                      string                 `json:"id"`
	Status                  string                 `json:"status"`
	Processor               string                 `json:"processor"`
	ProcessorSubscriptionID string                 `json:"processor_subscription_id"`
	StartedAt               int64                  `json:"started_at"`                         // Unix epoch seconds
	EndedAt                 *int64                 `json:"ended_at,omitempty"`                 // Unix epoch seconds
	CurrentPeriodStartsAt   *int64                 `json:"current_period_starts_at,omitempty"` // Unix epoch seconds
	CurrentPeriodEndsAt     *int64                 `json:"current_period_ends_at,omitempty"`   // Unix epoch seconds
	CancelledAt             *int64                 `json:"cancelled_at,omitempty"`             // Unix epoch seconds
	CancelType              *string                `json:"cancel_type,omitempty"`
	CancelFeedback          *string                `json:"cancel_feedback,omitempty"`
	CreatedAt               int64                  `json:"created_at"` // Unix epoch seconds
	UpdatedAt               int64                  `json:"updated_at"` // Unix epoch seconds
	Price                   *PriceInfo             `json:"price,omitempty"`
	PaymentMethod           *PaymentMethodInfo     `json:"payment_method,omitempty"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
}

// PriceInfo represents price information for billing history
type PriceInfo struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Amount    int64              `json:"amount"` // Amount in cents
	Currency  string             `json:"currency"`
	Recurring *api.RecurringInfo `json:"recurring,omitempty"`
}

// PaymentMethodInfo represents payment method information for billing history
type PaymentMethodInfo struct {
	ID        string `json:"id"`
	Processor string `json:"processor"`
	IsActive  bool   `json:"is_active"`
}

// PaymentItem represents a canonical payment record from Postgres
type PaymentItem struct {
	ID             string     `json:"id"`
	SubscriptionID *string    `json:"subscription_id,omitempty"`
	Processor      string     `json:"processor"`
	TransactionID  string     `json:"transaction_id"`
	Amount         int64      `json:"amount"` // Amount in cents
	Currency       string     `json:"currency"`
	Price          *PriceInfo `json:"price,omitempty"`
	PurchasedAt    int64      `json:"purchased_at"` // Unix epoch seconds
}

// PaymentEventItem represents a payment transaction event
type PaymentEventItem struct {
	EventID                string                 `json:"event_id"`
	SubscriptionID         *string                `json:"subscription_id,omitempty"`
	EventType              string                 `json:"event_type"`
	Processor              string                 `json:"processor"`
	ProcessorTransactionID *string                `json:"processor_transaction_id,omitempty"`
	Amount                 *int64                 `json:"amount,omitempty"` // Amount in cents
	Currency               string                 `json:"currency"`
	BillingInfo            map[string]interface{} `json:"billing_info,omitempty"`
	WebhookSource          *string                `json:"webhook_source,omitempty"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	Timestamp              int64                  `json:"timestamp"`  // Unix epoch seconds
	CreatedAt              int64                  `json:"created_at"` // Unix epoch seconds
}

// SubscriptionEventItem represents a subscription lifecycle event
type SubscriptionEventItem struct {
	EventID                 string                 `json:"event_id"`
	SubscriptionID          string                 `json:"subscription_id"`
	EventType               string                 `json:"event_type"`
	Processor               string                 `json:"processor"`
	ProcessorSubscriptionID *string                `json:"processor_subscription_id,omitempty"`
	ProcessorTransactionID  *string                `json:"processor_transaction_id,omitempty"`
	Amount                  *int64                 `json:"amount,omitempty"` // Amount in cents
	Currency                string                 `json:"currency"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
	Timestamp               int64                  `json:"timestamp"`  // Unix epoch seconds
	CreatedAt               int64                  `json:"created_at"` // Unix epoch seconds
}

// BillingHistoryStats represents aggregated billing statistics
type BillingHistoryStats struct {
	TotalCharged       int64                         `json:"total_charged"` // Total in cents
	TotalCharges       int                           `json:"total_charges"`
	SuccessfulCharges  int                           `json:"successful_charges"`
	FailedCharges      int                           `json:"failed_charges"`
	Refunds            int                           `json:"refunds"`
	TotalRefunded      int64                         `json:"total_refunded"`              // Total in cents
	FirstChargeDate    *int64                        `json:"first_charge_date,omitempty"` // Unix epoch seconds
	LastChargeDate     *int64                        `json:"last_charge_date,omitempty"`  // Unix epoch seconds
	ProcessorBreakdown map[string]ProcessorStatsInfo `json:"processor_breakdown"`
}

// ProcessorStatsInfo represents statistics for a specific payment processor
type ProcessorStatsInfo struct {
	TotalCharged      int64 `json:"total_charged"` // Total in cents
	TotalCharges      int   `json:"total_charges"`
	SuccessfulCharges int   `json:"successful_charges"`
	FailedCharges     int   `json:"failed_charges"`
}

// GetBillingHistoryResponse represents the enhanced billing history response
type GetBillingHistoryResponse struct {
	Subscriptions      []SubscriptionHistoryItem `json:"subscriptions"`
	Payments           []PaymentItem             `json:"payments"`
	PaymentEvents      []PaymentEventItem        `json:"payment_events,omitempty"`
	SubscriptionEvents []SubscriptionEventItem   `json:"subscription_events,omitempty"`
	Stats              *BillingHistoryStats      `json:"stats,omitempty"`
	Page               int                       `json:"page"`
	PageSize           int                       `json:"page_size"`
	TotalItems         int64                     `json:"total_items"`
	TotalPages         int                       `json:"total_pages"`
}

// NewGetBillingHistoryResponse creates a new billing history response
// func NewGetBillingHistoryResponse(
// 	subscriptions []models.Subscription,
// 	payments []*models.Payment,
// 	paymentEvents []services.BillingEvent,
// 	subscriptionEvents []services.SubscriptionEvent,
// 	stats *services.BillingHistoryStats,
// 	page, pageSize int,
// 	totalItems int64,
// ) *GetBillingHistoryResponse {
// 	// Calculate pagination
// 	totalPages, hasMore := common.CalculatePagination(page, pageSize, totalItems)

// 	// Convert subscriptions
// 	subscriptionItems := make([]SubscriptionHistoryItem, len(subscriptions))
// 	for i, sub := range subscriptions {
// 		item := SubscriptionHistoryItem{
// 			ID:                      sub.ID.String(),
// 			Status:                  string(sub.Status),
// 			Processor:               string(sub.Processor),
// 			ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
// 			StartedAt:               sub.StartedAt,
// 			EndedAt:                 sub.EndedAt,
// 			CurrentPeriodStartsAt:   sub.CurrentPeriodStartsAt,
// 			CurrentPeriodEndsAt:     sub.CurrentPeriodEndsAt,
// 			CancelledAt:             sub.CancelledAt,
// 			CreatedAt:               sub.CreatedAt,
// 			UpdatedAt:               sub.UpdatedAt,
// 		}

// 		if sub.CancelType != nil {
// 			cancelType := string(*sub.CancelType)
// 			item.CancelType = &cancelType
// 		}
// 		if sub.CancelFeedback != nil {
// 			item.CancelFeedback = sub.CancelFeedback
// 		}

// 		// Include price information if available
// 		if sub.Price != nil {
// 			billingCycleDays := 0
// 			if sub.Price.BillingCycleDays != nil {
// 				billingCycleDays = *sub.Price.BillingCycleDays
// 			}
// 			item.Price = &PriceInfo{
// 				ID:               sub.Price.ID.String(),
// 				Name:             sub.Price.DisplayName,
// 				Amount:           sub.Price.Amount,
// 				Currency:         sub.Price.Currency,
// 				BillingCycleDays: billingCycleDays,
// 			}
// 		}

// 		// Include payment method information if available
// 		if sub.PaymentMethod != nil {
// 			item.PaymentMethod = &PaymentMethodInfo{
// 				ID:        sub.PaymentMethod.ID.String(),
// 				Processor: string(sub.PaymentMethod.Processor),
// 				IsActive:  sub.PaymentMethod.IsActive,
// 			}
// 		}

// 		// Parse metadata if available
// 		if len(sub.Metadata) > 0 {
// 			// Metadata is already a json.RawMessage, try to parse it
// 			var metadata map[string]interface{}
// 			if err := json.Unmarshal(sub.Metadata, &metadata); err == nil {
// 				item.Metadata = metadata
// 			}
// 		}

// 		subscriptionItems[i] = item
// 	}

// 	// Convert payments (Postgres canonical)
// 	paymentItems := make([]PaymentItem, 0, len(payments))
// 	for _, p := range payments {
// 		if p == nil {
// 			continue
// 		}
// 		item := PaymentItem{
// 			ID:             p.ID.String(),
// 			SubscriptionID: nil,
// 			Processor:      string(p.Processor),
// 			TransactionID:  p.TransactionID,
// 			Amount:         p.Amount,
// 			Currency:       p.Currency,
// 			PurchasedAt:    p.PurchasedAt,
// 		}
// 		if p.SubscriptionID != nil {
// 			sid := p.SubscriptionID.String()
// 			item.SubscriptionID = &sid
// 		}
// 		if p.ExtensionDays != nil {
// 			item.ExtensionDays = p.ExtensionDays
// 		}
// 		if p.UserRoleGrantID != nil {
// 			uid := p.UserRoleGrantID.String()
// 			item.UserRoleGrantID = &uid
// 		}
// 		if p.Price != nil {
// 			billingCycleDays := 0
// 			if p.Price.BillingCycleDays != nil {
// 				billingCycleDays = *p.Price.BillingCycleDays
// 			}
// 			item.Price = &PriceInfo{
// 				ID:               p.Price.ID.String(),
// 				Name:             p.Price.DisplayName,
// 				Amount:           p.Price.Amount,
// 				Currency:         p.Price.Currency,
// 				BillingCycleDays: billingCycleDays,
// 			}
// 		}
// 		paymentItems = append(paymentItems, item)
// 	}

// 	// Convert payment events
// 	var paymentEventItems []PaymentEventItem
// 	if paymentEvents != nil {
// 		paymentEventItems = make([]PaymentEventItem, len(paymentEvents))
// 		for i, event := range paymentEvents {
// 			paymentEventItems[i] = PaymentEventItem{
// 				EventID:                event.EventID,
// 				SubscriptionID:         event.SubscriptionID,
// 				EventType:              event.EventType,
// 				Processor:              event.Processor,
// 				ProcessorTransactionID: event.ProcessorTransactionID,
// 				Amount:                 event.Amount,
// 				Currency:               event.Currency,
// 				BillingInfo:            event.BillingInfo,
// 				WebhookSource:          event.WebhookSource,
// 				Metadata:               event.Metadata,
// 				Timestamp:              event.Timestamp,
// 				CreatedAt:              event.CreatedAt,
// 			}
// 		}
// 	}

// 	// Convert subscription events
// 	var subscriptionEventItems []SubscriptionEventItem
// 	if subscriptionEvents != nil {
// 		subscriptionEventItems = make([]SubscriptionEventItem, len(subscriptionEvents))
// 		for i, event := range subscriptionEvents {
// 			subscriptionEventItems[i] = SubscriptionEventItem{
// 				EventID:                 event.EventID,
// 				SubscriptionID:          event.SubscriptionID,
// 				EventType:               event.EventType,
// 				Processor:               event.Processor,
// 				ProcessorSubscriptionID: event.ProcessorSubscriptionID,
// 				ProcessorTransactionID:  event.ProcessorTransactionID,
// 				Amount:                  event.Amount,
// 				Currency:                event.Currency,
// 				Metadata:                event.Metadata,
// 				Timestamp:               event.Timestamp,
// 				CreatedAt:               event.CreatedAt,
// 			}
// 		}
// 	}

// 	// Convert stats
// 	var statsResponse *BillingHistoryStats
// 	if stats != nil {
// 		processorBreakdown := make(map[string]ProcessorStatsInfo)
// 		for processor, processorStats := range stats.ProcessorBreakdown {
// 			processorBreakdown[processor] = ProcessorStatsInfo{
// 				TotalCharged:      processorStats.TotalCharged,
// 				TotalCharges:      processorStats.TotalCharges,
// 				SuccessfulCharges: processorStats.SuccessfulCharges,
// 				FailedCharges:     processorStats.FailedCharges,
// 			}
// 		}

// 		statsResponse = &BillingHistoryStats{
// 			TotalCharged:       stats.TotalCharged,
// 			TotalCharges:       stats.TotalCharges,
// 			SuccessfulCharges:  stats.SuccessfulCharges,
// 			FailedCharges:      stats.FailedCharges,
// 			Refunds:            stats.Refunds,
// 			TotalRefunded:      stats.TotalRefunded,
// 			FirstChargeDate:    stats.FirstChargeDate,
// 			LastChargeDate:     stats.LastChargeDate,
// 			ProcessorBreakdown: processorBreakdown,
// 		}
// 	}

// 	return &GetBillingHistoryResponse{
// 		Subscriptions:      subscriptionItems,
// 		Payments:           paymentItems,
// 		PaymentEvents:      paymentEventItems,
// 		SubscriptionEvents: subscriptionEventItems,
// 		Stats:              statsResponse,
// 		Page:               page,
// 		PageSize:           pageSize,
// 		TotalItems:         totalItems,
// 		TotalPages:         totalPages,
// 		HasMore:            hasMore,
// 	}
// }

// Type alias for admin endpoint (same structure)
type GetUserBillingHistoryResponse = GetBillingHistoryResponse

// -------------------------------- Payment Method Responses --------------------------------

// PaymentMethodResponse represents a Stripe-style payment method (card)
type PaymentMethodResponse struct {
	ID             string                       `json:"id"`                 // pm_xxx
	Object         string                       `json:"object"`             // "payment_method"
	Type           string                       `json:"type"`               // "card"
	Processor      string                       `json:"processor"`          // nmi, mobius, etc.
	Customer       *string                      `json:"customer,omitempty"` // usr_ prefix if available
	BillingDetails *PaymentMethodBillingDetails `json:"billing_details,omitempty"`
	Card           *PaymentMethodCardDetails    `json:"card,omitempty"`
	Metadata       map[string]string            `json:"metadata,omitempty"`
	Livemode       bool                         `json:"livemode"`
	IsActive       bool                         `json:"is_active"` // legacy field; mirrors livemode/active
	Created        int64                        `json:"created"`   // Unix epoch seconds
	FailureReason  *string                      `json:"failure_reason,omitempty"`
}

type PaymentMethodBillingDetails struct {
	Name    *string               `json:"name,omitempty"`
	Email   *string               `json:"email,omitempty"`
	Phone   *string               `json:"phone,omitempty"`
	Address *PaymentMethodAddress `json:"address,omitempty"`
}

type PaymentMethodAddress struct {
	Line1      *string `json:"line1,omitempty"`
	Line2      *string `json:"line2,omitempty"`
	City       *string `json:"city,omitempty"`
	State      *string `json:"state,omitempty"`
	PostalCode *string `json:"postal_code,omitempty"`
	Country    *string `json:"country,omitempty"`
}

type PaymentMethodCardDetails struct {
	Brand    *string `json:"brand,omitempty"` // visa, mastercard, amex
	Last4    *string `json:"last4,omitempty"`
	ExpMonth *int    `json:"exp_month,omitempty"`
	ExpYear  *int    `json:"exp_year,omitempty"`
}

// PaymentMethodToAPI converts a models.PaymentMethod to a Stripe-compatible PaymentMethodResponse
func PaymentMethodToAPI(pm *models.PaymentMethod) PaymentMethodResponse {
	card := &PaymentMethodCardDetails{
		Brand: pm.CardType,
		Last4: pm.LastFour,
	}
	if pm.ExpiryDate != nil {
		if month, year, ok := parseExpiry(*pm.ExpiryDate); ok {
			card.ExpMonth = &month
			card.ExpYear = &year
		}
	}

	return PaymentMethodResponse{
		ID:            api.FormatPaymentMethodID(pm.ID),
		Object:        "payment_method",
		Type:          "card",
		Processor:     string(pm.Processor),
		Card:          card,
		IsActive:      pm.IsActive,
		Created:       api.ToUnix(pm.CreatedAt),
		Metadata:      map[string]string{},
		FailureReason: pm.FailureReason,
	}
}

// PaymentMethodsToAPI converts a slice of models.PaymentMethod to PaymentMethodResponse slice
func PaymentMethodsToAPI(methods []*models.PaymentMethod) []PaymentMethodResponse {
	result := make([]PaymentMethodResponse, len(methods))
	for i, pm := range methods {
		result[i] = PaymentMethodToAPI(pm)
	}
	return result
}

// parseExpiry converts "MM/YY" or "MM-YY" to month/year integers.
func parseExpiry(exp string) (int, int, bool) {
	exp = strings.TrimSpace(exp)
	if exp == "" {
		return 0, 0, false
	}
	sep := "/"
	if strings.Contains(exp, "-") {
		sep = "-"
	}
	parts := strings.Split(exp, sep)
	if len(parts) != 2 {
		return 0, 0, false
	}
	month, err1 := strconv.Atoi(parts[0])
	year, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if year < 100 {
		year += 2000
	}
	return month, year, true
}
