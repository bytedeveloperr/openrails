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
type PaginationParams struct {
	Page     int `form:"page" default:"1" validate:"min=1,max=1000"`
	PageSize int `form:"page_size" default:"20" validate:"min=1,max=500"`
}

// SetPaginationDefaults sets default values for pagination parameters
// This method accepts a custom default page size to allow flexibility across different endpoints
func (p *PaginationParams) SetPaginationDefaults(defaultPageSize int) {
	p.Page = 1
	p.PageSize = defaultPageSize
}

// This method makes PageSize=min(PageSize,maxPageSize) and return true is PageSize have been changed
func (p *PaginationParams) CapPageSize(maxPageSize int) bool {
	if maxPageSize < p.PageSize {
		p.PageSize = maxPageSize
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
	Data services.SubscribeData `json:"data" validate:"required"`
}

type SubscribeRequest struct {
	BaseRequest
	SubscribeBodyParams
}

func (r *SubscribeRequest) Body() any {
	return &r.SubscribeBodyParams
}

// -------------------------------- GetProducts Request --------------------------------

type GetProductsRequest struct {
	BaseRequest
}

// -------------------------------- GetSubscribePageData Request --------------------------------

type GetSubscribePageDataRequest struct {
    BaseRequest
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

// -------------------------------- GenerateFlexFormURL Request --------------------------------

type GenerateFlexFormURLBodyParams struct {
	PriceID   string `json:"price_id" validate:"required,uuid"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100"`
	Address1  string `json:"address1" validate:"required,min=1,max=200"`
	City      string `json:"city" validate:"required,min=1,max=100"`
	State     string `json:"state" validate:"required,min=1,max=50"`
	ZipCode   string `json:"zip_code" validate:"required,min=1,max=20"`
	Country   string `json:"country" validate:"required,min=2,max=2"` // ISO 2-letter country code
}

type GenerateFlexFormURLRequest struct {
	BaseRequest
	GenerateFlexFormURLBodyParams
}

func (r *GenerateFlexFormURLRequest) Body() any {
    return &r.GenerateFlexFormURLBodyParams
}

// -------------------------------- Solana Generate Payment Request --------------------------------

type GeneratePaymentBodyParams struct {
    PriceID     string `json:"price_id" validate:"required,uuid"`
    Token       string `json:"token" validate:"required"`      // e.g., SOL, USDC
    UserWallet  string `json:"user_wallet" validate:"required"` // base58 address
}

type GeneratePaymentRequest struct {
    BaseRequest
    GeneratePaymentBodyParams
}

func (r *GeneratePaymentRequest) Body() any {
    return &r.GeneratePaymentBodyParams
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

// -------------------------------- GetBillingHistory Request --------------------------------

type GetBillingHistoryQueryParams struct {
	PaginationParams
	StartDate     *time.Time `form:"start_date" time_format:"2006-01-02"`
	EndDate       *time.Time `form:"end_date" time_format:"2006-01-02"`
	Processor     *string    `form:"processor" validate:"omitempty,oneof=ccbill mobius system"`
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
	UserID string `uri:"user_id" binding:"required,uuid"`
}

type GetUserBillingHistoryQueryParams struct {
	PaginationParams
	StartDate     *time.Time `form:"start_date" time_format:"2006-01-02"`
	EndDate       *time.Time `form:"end_date" time_format:"2006-01-02"`
	Processor     *string    `form:"processor" validate:"omitempty,oneof=ccbill mobius system"`
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
