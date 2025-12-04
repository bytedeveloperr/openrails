package handlers

import (
	"time"

	"github.com/doujins-org/doujins-billing/internal/services"
)

type IRequest interface {
	Path() any
	Query() any
	Body() any
	SetDefaults()
}

// BaseRequest provides default nil implementations for Request interface methods.
type BaseRequest struct{}

// Path returns nil by default, indicating no URI parameters.
func (b *BaseRequest) Path() any {
	return nil
}

// Query returns nil by default, indicating no query parameters.
func (b *BaseRequest) Query() any {
	return nil
}

// Body returns nil by default, indicating no body parameters.
func (b *BaseRequest) Body() any {
	return nil
}

func (b *BaseRequest) SetDefaults() {
}

// PaginationParams provides common pagination parameters that can be embedded in request structs
// Uses limit/offset pattern (Stripe-style) for better SQL mapping and flexibility
type PaginationParams struct {
	Limit  int `form:"limit" default:"20" validate:"min=1,max=100"`
	Offset int `form:"offset" default:"0" validate:"min=0"`
}

// SetPaginationDefaults sets default values for pagination parameters
// This method accepts a custom default limit to allow flexibility across different endpoints
func (p *PaginationParams) SetPaginationDefaults(defaultLimit int) {
	p.Limit = defaultLimit
	p.Offset = 0
}

// CapLimit ensures Limit doesn't exceed maxLimit and returns true if it was capped
func (p *PaginationParams) CapLimit(maxLimit int) bool {
	if maxLimit < p.Limit {
		p.Limit = maxLimit
		return true
	}
	return false
}

// -------------------------------- GetSubscription Request --------------------------------

type GetSubscriptionRequest struct {
	BaseRequest
}

// -------------------------------- Subscribe Request --------------------------------

type SubscribeBodyParams struct {
	services.SubscribeData
}

type SubscribeRequest struct {
	BaseRequest
	SubscribeBodyParams
}

func (r *SubscribeRequest) Body() any {
	return &r.SubscribeBodyParams
}

// -------------------------------- GetProducts Request --------------------------------
// Query params follow Stripe's pattern: https://docs.stripe.com/api/products/list

type GetProductsQueryParams struct {
	// Stripe-style pagination using limit/offset
	PaginationParams
	// Only return products that are active or inactive. Defaults to true (only active).
	// Non-admins can only see active products; inactive filter is silently ignored for them.
	Active *bool `form:"active"`
}

type GetProductsRequest struct {
	BaseRequest
	GetProductsQueryParams
}

func (r *GetProductsRequest) Query() any {
	return &r.GetProductsQueryParams
}

func (r *GetProductsRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}

// -------------------------------- GetPrices Request --------------------------------
// Query params follow Stripe's pattern: https://docs.stripe.com/api/prices/list

type GetPricesQueryParams struct {
	PaginationParams
	// Only return prices that are active or inactive. Defaults to true (only active).
	// Non-admins can only see active prices; inactive filter is silently ignored for them.
	Active *bool `form:"active"`
	// Only return prices for the given currency (e.g., "usd")
	Currency string `form:"currency"`
	// Only return prices for the given product ID
	Product string `form:"product"`
	// Only return prices of type "recurring" or "one_time"
	Type string `form:"type" validate:"omitempty,oneof=recurring one_time"`
}

type GetPricesRequest struct {
	BaseRequest
	GetPricesQueryParams
}

func (r *GetPricesRequest) Query() any {
	return &r.GetPricesQueryParams
}

func (r *GetPricesRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}

// -------------------------------- CancelSubscription Request --------------------------------

type CancelSubscriptionBodyParams struct {
	Feedback string `json:"feedback" validate:"max=500"`
}

type CancelSubscriptionRequest struct {
	BaseRequest
	CancelSubscriptionBodyParams
}

func (r *CancelSubscriptionRequest) Body() any {
	return &r.CancelSubscriptionBodyParams
}

// -------------------------------- Solana Generate Payment Request --------------------------------

type GeneratePaymentBodyParams struct {
	PriceID    string `json:"price_id" validate:"required,uuid"`
	Token      string `json:"token" validate:"required"`       // e.g., SOL, USDC
	UserWallet string `json:"user_wallet" validate:"required"` // base58 address
}

type GeneratePaymentRequest struct {
	BaseRequest
	GeneratePaymentBodyParams
}

func (r *GeneratePaymentRequest) Body() any {
	return &r.GeneratePaymentBodyParams
}

// -------------------------------- Solana Submit Payment Request --------------------------------

type SubmitPaymentBodyParams struct {
	SignedTransaction string `json:"signed_transaction" validate:"required"`
	PriceID           string `json:"price_id" validate:"required,uuid"`
	Memo              string `json:"memo"`
	IntentID          string `json:"intent_id" validate:"required,uuid"`
}

type SubmitPaymentRequest struct {
	BaseRequest
	SubmitPaymentBodyParams
}

func (r *SubmitPaymentRequest) Body() any {
	return &r.SubmitPaymentBodyParams
}

// -------------------------------- Solana Check Payment Request --------------------------------

type CheckSolanaPaymentQueryParams struct {
	Reference string `form:"reference" validate:"required"`
	Memo      string `form:"memo"`
}

type CheckSolanaPaymentRequest struct {
	BaseRequest
	CheckSolanaPaymentQueryParams
}

func (r *CheckSolanaPaymentRequest) Query() any {
	return &r.CheckSolanaPaymentQueryParams
}

// -------------------------------- Solana Generate QR Request --------------------------------

type GenerateSolanaPayQRBodyParams struct {
	PriceID    string `json:"price_id" validate:"required,uuid"`
	Token      string `json:"token" validate:"required"`
	UserWallet string `json:"user_wallet" validate:"required"`
}

type GenerateSolanaPayQRRequest struct {
	BaseRequest
	GenerateSolanaPayQRBodyParams
}

func (r *GenerateSolanaPayQRRequest) Body() any {
	return &r.GenerateSolanaPayQRBodyParams
}

// -------------------------------- Payment Methods Requests --------------------------------

type ListPaymentMethodsQueryParams struct {
	PaginationParams
	IncludeInactive bool `form:"include_inactive"`
}

type ListPaymentMethodsRequest struct {
	BaseRequest
	ListPaymentMethodsQueryParams
}

func (r *ListPaymentMethodsRequest) Query() any {
	return &r.ListPaymentMethodsQueryParams
}

func (r *ListPaymentMethodsRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}

type DeletePaymentMethodPathParams struct {
	ID string `uri:"id" binding:"required,uuid"`
}

type DeletePaymentMethodRequest struct {
	BaseRequest
	DeletePaymentMethodPathParams
}

func (r *DeletePaymentMethodRequest) Path() any {
	return &r.DeletePaymentMethodPathParams
}

type ActivatePaymentMethodPathParams struct {
	ID string `uri:"id" binding:"required,uuid"`
}

type ActivatePaymentMethodRequest struct {
	BaseRequest
	ActivatePaymentMethodPathParams
}

func (r *ActivatePaymentMethodRequest) Path() any {
	return &r.ActivatePaymentMethodPathParams
}

type CreatePaymentMethodRequest struct {
	PaymentToken string `json:"payment_token" binding:"required"`
	FirstName    string `json:"first_name" binding:"required"`
	LastName     string `json:"last_name" binding:"required"`
	Address1     string `json:"address1" binding:"required"`
	City         string `json:"city" binding:"required"`
	State        string `json:"state" binding:"omitempty"`
	Zip          string `json:"zip" binding:"required"`
	Country      string `json:"country" binding:"required"`
	Phone        string `json:"phone" binding:"omitempty"`
	Email        string `json:"email" binding:"omitempty,email"`
	Company      string `json:"company" binding:"omitempty"`
	Address2     string `json:"address2" binding:"omitempty"`
	Provider     string `json:"provider" binding:"omitempty"`
}

type UpdatePaymentMethodPathParams struct {
	ID string `uri:"id" binding:"required,uuid"`
}

type UpdatePaymentMethodBodyParams struct {
	PaymentToken string  `json:"payment_token" binding:"required"`
	FirstName    *string `json:"first_name"`
	LastName     *string `json:"last_name"`
	Address1     *string `json:"address1"`
	City         *string `json:"city"`
	State        *string `json:"state"`
	Zip          *string `json:"zip"`
	Country      *string `json:"country"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email" binding:"omitempty,email"`
	Company      *string `json:"company"`
	Address2     *string `json:"address2"`
	Provider     *string `json:"provider"`
}

type UpdatePaymentMethodRequest struct {
	BaseRequest
	UpdatePaymentMethodPathParams
	UpdatePaymentMethodBodyParams
}

func (r *UpdatePaymentMethodRequest) Path() any {
	return &r.UpdatePaymentMethodPathParams
}

func (r *UpdatePaymentMethodRequest) Body() any {
	return &r.UpdatePaymentMethodBodyParams
}

// -------------------------------- GetBillingHistory Request --------------------------------

type GetBillingHistoryQueryParams struct {
	PaginationParams
	StartDate     *time.Time `form:"start_date" time_format:"2006-01-02"`
	EndDate       *time.Time `form:"end_date" time_format:"2006-01-02"`
	Processor     *string    `form:"processor" validate:"omitempty,oneof=ccbill nmi system"`
	MinAmount     *float64   `form:"min_amount" validate:"omitempty,min=0"`
	MaxAmount     *float64   `form:"max_amount" validate:"omitempty,min=0"`
	IncludeStats  bool       `form:"include_stats" default:"false"`
	IncludeEvents bool       `form:"include_events" default:"true"`
}

type GetBillingHistoryRequest struct {
	BaseRequest
	GetBillingHistoryQueryParams
}

func (r *GetBillingHistoryRequest) Query() any {
	return &r.GetBillingHistoryQueryParams
}

func (r *GetBillingHistoryRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}

// -------------------------------- GetUserBillingHistory Request (Admin) --------------------------------

type GetUserBillingHistoryPathParams struct {
	UserID string `uri:"user_id" binding:"required"`
}

type GetUserBillingHistoryQueryParams struct {
	PaginationParams
	StartDate     *time.Time `form:"start_date" time_format:"2006-01-02"`
	EndDate       *time.Time `form:"end_date" time_format:"2006-01-02"`
	Processor     *string    `form:"processor" validate:"omitempty,oneof=ccbill nmi system"`
	MinAmount     *float64   `form:"min_amount" validate:"omitempty,min=0"`
	MaxAmount     *float64   `form:"max_amount" validate:"omitempty,min=0"`
	IncludeStats  bool       `form:"include_stats" default:"false"`
	IncludeEvents bool       `form:"include_events" default:"true"`
}

type GetUserBillingHistoryRequest struct {
	BaseRequest
	GetUserBillingHistoryPathParams
	GetUserBillingHistoryQueryParams
}

func (r *GetUserBillingHistoryRequest) Path() any {
	return &r.GetUserBillingHistoryPathParams
}

func (r *GetUserBillingHistoryRequest) Query() any {
	return &r.GetUserBillingHistoryQueryParams
}

func (r *GetUserBillingHistoryRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}

// -------------------------------- Payment Intent Requests --------------------------------

// CreatePaymentIntentBodyParams for POST /v1/payment-intents (direct wallet flow)
type CreatePaymentIntentBodyParams struct {
	PriceID string `json:"price_id" validate:"required,uuid"`
	Token   string `json:"token" validate:"required"`  // e.g., SOL, USDC
	Wallet  string `json:"wallet" validate:"required"` // User's linked wallet address
}

type CreatePaymentIntentRequest struct {
	BaseRequest
	CreatePaymentIntentBodyParams
}

func (r *CreatePaymentIntentRequest) Body() any {
	return &r.CreatePaymentIntentBodyParams
}

// CreatePaymentIntentQRBodyParams for POST /v1/payment-intents/qr (Solana Pay QR flow)
type CreatePaymentIntentQRBodyParams struct {
	PriceID string `json:"price_id" validate:"required,uuid"`
	Token   string `json:"token" validate:"required"` // e.g., SOL, USDC
	Wallet  string `json:"wallet"`                    // Optional: user's wallet for reference
}

type CreatePaymentIntentQRRequest struct {
	BaseRequest
	CreatePaymentIntentQRBodyParams
}

func (r *CreatePaymentIntentQRRequest) Body() any {
	return &r.CreatePaymentIntentQRBodyParams
}

// ConfirmPaymentIntentBodyParams for POST /v1/payment-intents/:id/confirm
type ConfirmPaymentIntentBodyParams struct {
	SignedTransaction string `json:"signed_transaction" validate:"required"`
}

type ConfirmPaymentIntentRequest struct {
	BaseRequest
	ConfirmPaymentIntentBodyParams
}

func (r *ConfirmPaymentIntentRequest) Body() any {
	return &r.ConfirmPaymentIntentBodyParams
}

// -------------------------------- Unified Checkout Request --------------------------------

// CheckoutBodyParams for POST /v1/me/checkout
type CheckoutBodyParams struct {
	PriceID         string `json:"price_id" validate:"required,uuid"`
	Processor       string `json:"processor" validate:"required,oneof=mobius ccbill solana"`
	PaymentMethodID string `json:"payment_method_id,omitempty" validate:"omitempty,uuid"`
	PaymentToken    string `json:"payment_token,omitempty"`

	// Optional billing info (used when creating vault from payment token)
	Email     string `json:"email,omitempty" validate:"omitempty,email"`
	FirstName string `json:"first_name,omitempty" validate:"omitempty,max=100"`
	LastName  string `json:"last_name,omitempty" validate:"omitempty,max=100"`
	Address1  string `json:"address1,omitempty" validate:"omitempty,max=200"`
	City      string `json:"city,omitempty" validate:"omitempty,max=100"`
	State     string `json:"state,omitempty" validate:"omitempty,max=50"`
	Zip       string `json:"zip,omitempty" validate:"omitempty,max=20"`
	Country   string `json:"country,omitempty" validate:"omitempty,max=2"`
}

type CheckoutRequest struct {
	BaseRequest
	CheckoutBodyParams
}

func (r *CheckoutRequest) Body() any {
	return &r.CheckoutBodyParams
}
