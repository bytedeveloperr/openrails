package handlers

import (
	"time"

	"github.com/doujins-org/doujins-billing/internal/api/models/common"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// -------------------------------- Subscription Responses --------------------------------

type SubscribeResponse struct {
	SubscriptionID string `json:"subscription_id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
}

func NewSubscribeResponse(serviceResponse *services.SubscribeResponse) *SubscribeResponse {
	return &SubscribeResponse{
		SubscriptionID: serviceResponse.SubscriptionID,
		Status:         serviceResponse.Status,
		Message:        serviceResponse.Message,
	}
}

type ActiveSubscriptionResponse struct {
	ID                    string     `json:"id"`
	Status                string     `json:"status"`
	Processor             string     `json:"processor"`
	CurrentPeriodStartsAt *time.Time `json:"current_period_starts_at,omitempty"`
	CurrentPeriodEndsAt   *time.Time `json:"current_period_ends_at,omitempty"`
	PriceAmount           float64    `json:"price_amount"`
	Currency              string     `json:"currency"`
	BillingCycleDays      int        `json:"billing_cycle_days"`
}

func NewActiveSubscriptionResponse(subscription *services.UserSubscriptionResponse) *ActiveSubscriptionResponse {
	if subscription == nil {
		return nil
	}
	return &ActiveSubscriptionResponse{
		ID:                    subscription.ID,
		Status:                subscription.Status,
		Processor:             subscription.Processor,
		CurrentPeriodStartsAt: subscription.CurrentPeriodStartsAt,
		CurrentPeriodEndsAt:   subscription.CurrentPeriodEndsAt,
		PriceAmount:           subscription.PriceAmount,
		Currency:              subscription.Currency,
		BillingCycleDays:      subscription.BillingCycleDays,
	}
}

type SubscriptionHistoryItem struct {
	ID                    string     `json:"id"`
	Status                string     `json:"status"`
	Processor             string     `json:"processor"`
	StartedAt             time.Time  `json:"started_at"`
	EndedAt               *time.Time `json:"ended_at,omitempty"`
	CurrentPeriodStartsAt *time.Time `json:"current_period_starts_at,omitempty"`
	CurrentPeriodEndsAt   *time.Time `json:"current_period_ends_at,omitempty"`
	PriceAmount           float64    `json:"price_amount"`
	Currency              string     `json:"currency"`
	BillingCycleDays      int        `json:"billing_cycle_days"`
}

type GetSubscriptionHistoryResponse = query.PaginatedResponse[*SubscriptionHistoryItem]

func NewGetSubscriptionHistoryResponse(subscriptions []*services.UserSubscriptionResponse, page, pageSize int, totalItems int64) *GetSubscriptionHistoryResponse {
	items := make([]*SubscriptionHistoryItem, len(subscriptions))
	for i, sub := range subscriptions {
		items[i] = &SubscriptionHistoryItem{
			ID:                    sub.ID,
			Status:                sub.Status,
			Processor:             sub.Processor,
			StartedAt:             sub.StartedAt,
			EndedAt:               sub.EndedAt,
			CurrentPeriodStartsAt: sub.CurrentPeriodStartsAt,
			CurrentPeriodEndsAt:   sub.CurrentPeriodEndsAt,
			PriceAmount:           sub.PriceAmount,
			Currency:              sub.Currency,
			BillingCycleDays:      sub.BillingCycleDays,
		}
	}
	return common.NewPaginatedResponse(items, page, pageSize, totalItems)
}

type GenerateFlexFormURLResponse struct {
	IFrameURL  string `json:"iframe_url"`
	Width      string `json:"width"`
	Height     string `json:"height"`
	SuccessURL string `json:"success_url,omitempty"`
	DeclineURL string `json:"decline_url,omitempty"`
}

func NewGenerateFlexFormURLResponse(serviceResponse *services.FlexFormURLResponse) *GenerateFlexFormURLResponse {
	return &GenerateFlexFormURLResponse{
		IFrameURL:  serviceResponse.IFrameURL,
		Width:      serviceResponse.Width,
		Height:     serviceResponse.Height,
		SuccessURL: serviceResponse.SuccessURL,
		DeclineURL: serviceResponse.DeclineURL,
	}
}

// -------------------------------- Payment Method Responses --------------------------------

type PaymentMethodResponse struct {
	ID               string    `json:"id"`
	Processor        string    `json:"processor"`
	IsActive         bool      `json:"is_active"`
	CardLast4        string    `json:"card_last4,omitempty"`
	CardType         string    `json:"card_type,omitempty"`
	ExpirationMonth  int       `json:"expiration_month,omitempty"`
	ExpirationYear   int       `json:"expiration_year,omitempty"`
	BillingFirstName string    `json:"billing_first_name,omitempty"`
	BillingLastName  string    `json:"billing_last_name,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func NewPaymentMethodResponse(serviceResponse *services.PaymentMethodResponse) *PaymentMethodResponse {
	return &PaymentMethodResponse{
		ID:               serviceResponse.ID,
		Processor:        serviceResponse.Processor,
		IsActive:         serviceResponse.IsActive,
		CardLast4:        serviceResponse.CardLast4,
		CardType:         serviceResponse.CardType,
		ExpirationMonth:  serviceResponse.ExpirationMonth,
		ExpirationYear:   serviceResponse.ExpirationYear,
		BillingFirstName: serviceResponse.BillingFirstName,
		BillingLastName:  serviceResponse.BillingLastName,
		CreatedAt:        serviceResponse.CreatedAt,
		UpdatedAt:        serviceResponse.UpdatedAt,
	}
}

type CreatePaymentMethodResponse struct {
	PaymentMethod *PaymentMethodResponse `json:"payment_method"`
	Message       string                 `json:"message"`
}

func NewCreatePaymentMethodResponse(serviceResponse *services.PaymentMethodResponse, message string) *CreatePaymentMethodResponse {
	return &CreatePaymentMethodResponse{
		PaymentMethod: NewPaymentMethodResponse(serviceResponse),
		Message:       message,
	}
}

type ListPaymentMethodsResponse = query.PaginatedResponse[*PaymentMethodResponse]

func NewListPaymentMethodsResponse(paymentMethods []*services.PaymentMethodResponse, page, pageSize int, totalItems int64) *ListPaymentMethodsResponse {
	items := make([]*PaymentMethodResponse, len(paymentMethods))
	for i, pm := range paymentMethods {
		items[i] = NewPaymentMethodResponse(pm)
	}
	return common.NewPaginatedResponse(items, page, pageSize, totalItems)
}

type UpdatePaymentMethodResponse struct {
	PaymentMethod *PaymentMethodResponse `json:"payment_method"`
	Message       string                 `json:"message"`
}

func NewUpdatePaymentMethodResponse(serviceResponse *services.PaymentMethodResponse, message string) *UpdatePaymentMethodResponse {
	return &UpdatePaymentMethodResponse{
		PaymentMethod: NewPaymentMethodResponse(serviceResponse),
		Message:       message,
	}
}

type DeletePaymentMethodResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func NewDeletePaymentMethodResponse(success bool, message string) *DeletePaymentMethodResponse {
	return &DeletePaymentMethodResponse{
		Success: success,
		Message: message,
	}
}

type ActivatePaymentMethodResponse struct {
	PaymentMethod *PaymentMethodResponse `json:"payment_method"`
	Message       string                 `json:"message"`
}

func NewActivatePaymentMethodResponse(serviceResponse *services.PaymentMethodResponse, message string) *ActivatePaymentMethodResponse {
	return &ActivatePaymentMethodResponse{
		PaymentMethod: NewPaymentMethodResponse(serviceResponse),
		Message:       message,
	}
}

// -------------------------------- Solana Responses --------------------------------

type SupportedTokenResponse struct {
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	Decimals     int    `json:"decimals"`
	MintAddress  string `json:"mint_address"`
	LogoURI      string `json:"logo_uri,omitempty"`
	IsStablecoin bool   `json:"is_stablecoin"`
}

type GetSupportedTokensResponse struct {
	Tokens []SupportedTokenResponse `json:"tokens"`
}

func NewGetSupportedTokensResponse(tokens []services.SupportedToken) *GetSupportedTokensResponse {
	items := make([]SupportedTokenResponse, len(tokens))
	for i, token := range tokens {
		items[i] = SupportedTokenResponse{
			Symbol:       token.Symbol,
			Name:         token.Name,
			Decimals:     token.Decimals,
			MintAddress:  token.MintAddress,
			LogoURI:      token.LogoURI,
			IsStablecoin: token.IsStablecoin,
		}
	}
	return &GetSupportedTokensResponse{
		Tokens: items,
	}
}

type GenerateQRResponse struct {
	URL         string  `json:"url"`
	Amount      float64 `json:"amount"`
	TokenAmount string  `json:"token_amount"`
	TokenSymbol string  `json:"token_symbol"`
	Label       string  `json:"label"`
	Message     string  `json:"message"`
	ExpiresAt   int64   `json:"expires_at"`
}

func NewGenerateQRResponse(serviceResponse *services.QRCodeResponse) *GenerateQRResponse {
	return &GenerateQRResponse{
		URL:         serviceResponse.URL,
		Amount:      serviceResponse.Amount,
		TokenAmount: serviceResponse.TokenAmount,
		TokenSymbol: serviceResponse.TokenSymbol,
		Label:       serviceResponse.Label,
		Message:     serviceResponse.Message,
		ExpiresAt:   serviceResponse.ExpiresAt,
	}
}

type GenerateTransactionResponse struct {
	Transaction string `json:"transaction"`
	Message     string `json:"message"`
}

func NewGenerateTransactionResponse(serviceResponse *services.TransactionResponse) *GenerateTransactionResponse {
	return &GenerateTransactionResponse{
		Transaction: serviceResponse.Transaction,
		Message:     serviceResponse.Message,
	}
}

type SubmitTransactionResponse struct {
	PurchaseID    string `json:"purchase_id"`
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

func NewSubmitTransactionResponse(serviceResponse *services.TransactionSubmissionResponse) *SubmitTransactionResponse {
	return &SubmitTransactionResponse{
		PurchaseID:    serviceResponse.PurchaseID,
		TransactionID: serviceResponse.TransactionID,
		Status:        serviceResponse.Status,
		Message:       serviceResponse.Message,
	}
}

// -------------------------------- Webhook Responses --------------------------------

type ProcessWebhookResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	EventID string `json:"event_id,omitempty"`
}

func NewProcessWebhookResponse(serviceResponse *services.WebhookProcessResult) *ProcessWebhookResponse {
	return &ProcessWebhookResponse{
		Success: serviceResponse.Success,
		Message: serviceResponse.Message,
		EventID: serviceResponse.EventID,
	}
}

// -------------------------------- Admin Responses --------------------------------

type ExtendSubscriptionResponse struct {
	SubscriptionID string     `json:"subscription_id"`
	ExtendedUntil  *time.Time `json:"extended_until"`
	Message        string     `json:"message"`
}

func NewExtendSubscriptionResponse(subscriptionID string, extendedUntil *time.Time, message string) *ExtendSubscriptionResponse {
	return &ExtendSubscriptionResponse{
		SubscriptionID: subscriptionID,
		ExtendedUntil:  extendedUntil,
		Message:        message,
	}
}

type CancelSubscriptionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func NewCancelSubscriptionResponse(success bool, message string) *CancelSubscriptionResponse {
	return &CancelSubscriptionResponse{
		Success: success,
		Message: message,
	}
}

type GetSubscriptionDetailsResponse struct {
	Subscription  *ActiveSubscriptionResponse `json:"subscription"`
	PaymentMethod *PaymentMethodResponse      `json:"payment_method,omitempty"`
}

func NewGetSubscriptionDetailsResponse(subscription *services.UserSubscriptionResponse, paymentMethod *services.PaymentMethodResponse) *GetSubscriptionDetailsResponse {
	var pmResponse *PaymentMethodResponse
	if paymentMethod != nil {
		pmResponse = NewPaymentMethodResponse(paymentMethod)
	}
	return &GetSubscriptionDetailsResponse{
		Subscription:  NewActiveSubscriptionResponse(subscription),
		PaymentMethod: pmResponse,
	}
}

type ProcessRefundResponse struct {
	RefundID string  `json:"refund_id"`
	Amount   float64 `json:"amount"`
	Status   string  `json:"status"`
	Message  string  `json:"message"`
}

func NewProcessRefundResponse(refundID string, amount float64, status, message string) *ProcessRefundResponse {
	return &ProcessRefundResponse{
		RefundID: refundID,
		Amount:   amount,
		Status:   status,
		Message:  message,
	}
}
