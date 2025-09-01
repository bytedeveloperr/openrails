package handlers

import (
	"time"
)

// -------------------------------- Subscription Requests --------------------------------

type SubscribeBodyParams struct {
	PriceID      string `json:"price_id" validate:"required,uuid"`
	Processor    string `json:"processor" validate:"required,oneof=mobius ccbill"`
	PaymentToken string `json:"payment_token,omitempty"`
	FirstName    string `json:"first_name" validate:"required"`
	LastName     string `json:"last_name" validate:"required"`
	Email        string `json:"email" validate:"required,email"`
	Address1     string `json:"address1" validate:"required"`
	City         string `json:"city" validate:"required"`
	State        string `json:"state" validate:"required"`
	Zip          string `json:"zip" validate:"required"`
	Country      string `json:"country" validate:"required"`
}

type SubscribeRequest struct {
	SubscribeBodyParams
}

func (r *SubscribeRequest) Body() any {
	return &r.SubscribeBodyParams
}

type GenerateFlexFormURLBodyParams struct {
	PriceID   string `json:"price_id" validate:"required,uuid"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100"`
	Address1  string `json:"address1" validate:"required,min=1,max=200"`
	City      string `json:"city" validate:"required,min=1,max=100"`
	State     string `json:"state" validate:"required,min=1,max=50"`
	ZipCode   string `json:"zip_code" validate:"required,min=1,max=20"`
	Country   string `json:"country" validate:"required,min=2,max=2"`
}

type GenerateFlexFormURLRequest struct {
	GenerateFlexFormURLBodyParams
}

func (r *GenerateFlexFormURLRequest) Body() any {
	return &r.GenerateFlexFormURLBodyParams
}

type GetSubscriptionHistoryQueryParams struct {
	Page     int    `form:"page" json:"page"`
	PageSize int    `form:"page_size" json:"page_size"`
	Sort     string `form:"sort" json:"sort"`
	Order    string `form:"order" json:"order"`
	StartDate *time.Time `form:"start_date" time_format:"2006-01-02"`
	EndDate   *time.Time `form:"end_date" time_format:"2006-01-02"`
}

type GetSubscriptionHistoryRequest struct {
	GetSubscriptionHistoryQueryParams
}

func (r *GetSubscriptionHistoryRequest) Query() any {
	return &r.GetSubscriptionHistoryQueryParams
}

func (r *GetSubscriptionHistoryRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}

// -------------------------------- Payment Method Requests --------------------------------

type CreatePaymentMethodBodyParams struct {
	PaymentToken string `json:"payment_token" validate:"required"`
	FirstName    string `json:"first_name" validate:"required"`
	LastName     string `json:"last_name" validate:"required"`
	Address1     string `json:"address1" validate:"required"`
	City         string `json:"city" validate:"required"`
	State        string `json:"state" validate:"required"`
	Zip          string `json:"zip" validate:"required"`
	Country      string `json:"country" validate:"required"`
	Email        string `json:"email" validate:"required,email"`
}

type CreatePaymentMethodRequest struct {
	CreatePaymentMethodBodyParams
}

func (r *CreatePaymentMethodRequest) Body() any {
	return &r.CreatePaymentMethodBodyParams
}

type ListPaymentMethodsQueryParams struct {
	IncludeInactive bool `form:"include_inactive" default:"false"`
}

type ListPaymentMethodsRequest struct {
	ListPaymentMethodsQueryParams
}

func (r *ListPaymentMethodsRequest) Query() any {
	return &r.ListPaymentMethodsQueryParams
}

type UpdatePaymentMethodPathParams struct {
	PaymentMethodID string `uri:"id" binding:"required,uuid"`
}

type UpdatePaymentMethodBodyParams struct {
	CCNumber  string `json:"cc_number,omitempty"`
	CCExp     string `json:"cc_exp,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Address1  string `json:"address1,omitempty"`
	City      string `json:"city,omitempty"`
	State     string `json:"state,omitempty"`
	Zip       string `json:"zip,omitempty"`
	Country   string `json:"country,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	Company   string `json:"company,omitempty"`
	Address2  string `json:"address2,omitempty"`
}

type UpdatePaymentMethodRequest struct {
	UpdatePaymentMethodPathParams
	UpdatePaymentMethodBodyParams
}

func (r *UpdatePaymentMethodRequest) Path() any {
	return &r.UpdatePaymentMethodPathParams
}

func (r *UpdatePaymentMethodRequest) Body() any {
	return &r.UpdatePaymentMethodBodyParams
}

type DeletePaymentMethodPathParams struct {
	PaymentMethodID string `uri:"id" binding:"required,uuid"`
}

type DeletePaymentMethodRequest struct {
	DeletePaymentMethodPathParams
}

func (r *DeletePaymentMethodRequest) Path() any {
	return &r.DeletePaymentMethodPathParams
}

type ActivatePaymentMethodPathParams struct {
	PaymentMethodID string `uri:"id" binding:"required,uuid"`
}

type ActivatePaymentMethodRequest struct {
	ActivatePaymentMethodPathParams
}

func (r *ActivatePaymentMethodRequest) Path() any {
	return &r.ActivatePaymentMethodPathParams
}

// -------------------------------- Solana Requests --------------------------------

type GenerateQRBodyParams struct {
	PriceID     string `json:"price_id" validate:"required,uuid"`
	TokenSymbol string `json:"token" validate:"required,oneof=SOL USDC PYUSD"`
	UserWallet  string `json:"user_wallet" validate:"required"`
}

type GenerateQRRequest struct {
	GenerateQRBodyParams
}

func (r *GenerateQRRequest) Body() any {
	return &r.GenerateQRBodyParams
}

type GenerateTransactionBodyParams struct {
	PriceID     string `json:"price_id" validate:"required,uuid"`
	TokenSymbol string `json:"token" validate:"required,oneof=SOL USDC PYUSD"`
	Account     string `json:"account" validate:"required"`
}

type GenerateTransactionRequest struct {
	GenerateTransactionBodyParams
}

func (r *GenerateTransactionRequest) Body() any {
	return &r.GenerateTransactionBodyParams
}

type SubmitTransactionBodyParams struct {
	Signature string `json:"signature" validate:"required"`
}

type SubmitTransactionRequest struct {
	SubmitTransactionBodyParams
}

func (r *SubmitTransactionRequest) Body() any {
	return &r.SubmitTransactionBodyParams
}

// -------------------------------- Admin Requests --------------------------------

type ExtendSubscriptionPathParams struct {
	UserID string `uri:"user_id" binding:"required,uuid"`
}

type ExtendSubscriptionBodyParams struct {
	Days   int    `json:"days" validate:"required,min=1,max=365"`
	Reason string `json:"reason" validate:"required"`
}

type ExtendSubscriptionRequest struct {
	ExtendSubscriptionPathParams
	ExtendSubscriptionBodyParams
}

func (r *ExtendSubscriptionRequest) Path() any {
	return &r.ExtendSubscriptionPathParams
}

func (r *ExtendSubscriptionRequest) Body() any {
	return &r.ExtendSubscriptionBodyParams
}

type CancelSubscriptionPathParams struct {
	UserID string `uri:"user_id" binding:"required,uuid"`
}

type CancelSubscriptionBodyParams struct {
	Reason string `json:"reason" validate:"required"`
}

type CancelSubscriptionRequest struct {
	CancelSubscriptionPathParams
	CancelSubscriptionBodyParams
}

func (r *CancelSubscriptionRequest) Path() any {
	return &r.CancelSubscriptionPathParams
}

func (r *CancelSubscriptionRequest) Body() any {
	return &r.CancelSubscriptionBodyParams
}

type GetSubscriptionDetailsPathParams struct {
	UserID string `uri:"user_id" binding:"required,uuid"`
}

type GetSubscriptionDetailsRequest struct {
	GetSubscriptionDetailsPathParams
}

func (r *GetSubscriptionDetailsRequest) Path() any {
	return &r.GetSubscriptionDetailsPathParams
}

type ProcessRefundPathParams struct {
	UserID string `uri:"user_id" binding:"required,uuid"`
}

type ProcessRefundBodyParams struct {
	TransactionID string  `json:"transaction_id" validate:"required"`
	Amount        float64 `json:"amount" validate:"required,min=0"`
	Reason        string  `json:"reason" validate:"required"`
}

type ProcessRefundRequest struct {
	ProcessRefundPathParams
	ProcessRefundBodyParams
}

func (r *ProcessRefundRequest) Path() any {
	return &r.ProcessRefundPathParams
}

func (r *ProcessRefundRequest) Body() any {
	return &r.ProcessRefundBodyParams
}
