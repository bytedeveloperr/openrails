package handlers

import (
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/api"
)

type GetSubscriptionResponse = services.UserSubscriptionResponse

type SubscribeResponse = services.SubscribeResponse

// GetProductsResponse is now a Stripe-like list response
type GetProductsResponse = api.ListResponse[api.ProductObject]

// GetPricesResponse is a Stripe-like list response for prices
type GetPricesResponse = api.ListResponse[api.PriceObject]

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

	payment := api.PaymentObject{
		ID:             api.FormatPaymentID(p.ID),
		Object:         "payment",
		Amount:         p.Amount,
		AmountRefunded: amountRefunded,
		Currency:       p.Currency,
		User:           api.FormatUserID(p.UserID),
		Subscription:   subID,
		Processor:      string(p.Processor),
		TransactionID:  p.TransactionID,
		Refunded:       amountRefunded >= p.Amount && p.Amount > 0,
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

	return api.PriceObject{
		ID:        api.FormatPriceID(p.ID),
		Object:    "price",
		Name:      p.DisplayName,
		Amount:    p.Amount,
		Currency:  p.Currency,
		Recurring: recurring,
		Product:   api.FormatProductID(p.ProductID),
		Active:    p.IsActive,
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

// GeneratePaymentResponse represents a generated on-chain transaction with helper info
type GeneratePaymentResponse struct {
	Transaction  string `json:"transaction"`            // base64-encoded tx (placeholder if not available)
	Amount       int64  `json:"amount"`                 // fiat price amount in cents
	Currency     string `json:"currency"`               // fiat currency (e.g., usd)
	TokenAmount  uint64 `json:"token_amount"`           // smallest unit amount
	TokenSymbol  string `json:"token_symbol"`           // SOL/USDC/etc.
	ExpiresAt    int64  `json:"expires_at"`             // unix epoch expiry
	Instructions string `json:"instructions,omitempty"` // human-readable instructions
	IntentID     string `json:"intent_id"`
}

// SubmitPaymentResponse represents the result of submitting a signed transaction
type SubmitPaymentResponse struct {
	PaymentID     string `json:"payment_id"`
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	Amount        int64  `json:"amount"` // Amount in cents
	Currency      string `json:"currency"`
	ProcessedAt   int64  `json:"processed_at"` // Unix epoch seconds
	Message       string `json:"message"`
	IntentID      string `json:"intent_id"`
}

// PaymentStatusResponse represents the status of a payment
type PaymentStatusResponse struct {
	PaymentID     string `json:"payment_id"`
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	Amount        int64  `json:"amount"` // Amount in cents
	Currency      string `json:"currency"`
	CreatedAt     int64  `json:"created_at"`             // Unix epoch seconds
	ConfirmedAt   *int64 `json:"confirmed_at,omitempty"` // Unix epoch seconds
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// SolanaPayQRResponse contains the Solana Pay URL metadata
type SolanaPayQRResponse struct {
	URL         string `json:"url"`          // Solana Pay URL for QR code
	Amount      int64  `json:"amount"`       // Amount in cents
	TokenAmount string `json:"token_amount"` // human-readable token amount
	TokenSymbol string `json:"token_symbol"` // SOL, USDC, etc.
	Label       string `json:"label"`        // Merchant label
	Message     string `json:"message"`      // Payment message
	ExpiresAt   int64  `json:"expires_at"`   // Unix timestamp when QR expires
	Reference   string `json:"reference"`
	IntentID    string `json:"intent_id"`
}

// CheckSolanaPaymentResponse contains the payment check result
type CheckSolanaPaymentResponse struct {
	Status       string `json:"status"`               // pending, confirmed, failed
	PaymentID    string `json:"payment_id,omitempty"` // Payment ID if confirmed
	IntentID     string `json:"intent_id"`
	Transaction  string `json:"transaction,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
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
	IFrameURL  string `json:"iframe_url" binding:"required"`
	Width      string `json:"width" binding:"required"`
	Height     string `json:"height" binding:"required"`
	SuccessURL string `json:"success_url,omitempty"`
	DeclineURL string `json:"decline_url,omitempty"`
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

// PaymentMethodResponse represents a NMI payment method response (Stripe-compatible)
// Payment methods only support NMI card vaults
type PaymentMethodResponse struct {
	ID        string `json:"id"`        // Prefixed with pm_
	Object    string `json:"object"`    // Always "payment_method"
	Type      string `json:"type"`      // Always "card" for NMI
	Processor string `json:"processor"` // nmi, mobius, etc.
	IsActive  bool   `json:"is_active"`
	Created   int64  `json:"created"` // Unix epoch seconds

	// Card-specific fields (NMI only) - Stripe-compatible names
	Last4      *string `json:"last4,omitempty"`
	Brand      *string `json:"brand,omitempty"`       // visa, mastercard, amex, etc.
	ExpiryDate *string `json:"expiry_date,omitempty"` // MM/YY format

	// Additional fields
	DisplayName   string  `json:"display_name"`
	FailureReason *string `json:"failure_reason,omitempty"`
}

// PaymentMethodToAPI converts a models.PaymentMethod to a Stripe-compatible PaymentMethodResponse
func PaymentMethodToAPI(pm *models.PaymentMethod) PaymentMethodResponse {
	displayName := "Card"
	if pm.CardType != nil && pm.LastFour != nil {
		displayName = *pm.CardType + " •••• " + *pm.LastFour
	} else if pm.LastFour != nil {
		displayName = "Card •••• " + *pm.LastFour
	}

	return PaymentMethodResponse{
		ID:            api.FormatPaymentMethodID(pm.ID),
		Object:        "payment_method",
		Type:          "card",
		Processor:     string(pm.Processor),
		IsActive:      pm.IsActive,
		Created:       api.ToUnix(pm.CreatedAt),
		Last4:         pm.LastFour,
		Brand:         pm.CardType,
		ExpiryDate:    pm.ExpiryDate,
		DisplayName:   displayName,
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
