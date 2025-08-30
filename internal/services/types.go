package services

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Subscription helper functions (moved from database model to service layer)

// IsExpired checks if a subscription has expired
func IsExpired(s *models.Subscription) bool {
	return s.CurrentPeriodEndsAt != nil && s.CurrentPeriodEndsAt.Before(time.Now())
}

// -------------------------------- Mobius Webhook Types --------------------------------

type MobiusWebhookEvent struct {
	EventID   string                 `json:"event_id" validate:"required"`
	EventType MobiusWebhookEventType `json:"event_type" validate:"required"`
	EventBody MobiusEventBody        `json:"event_body" validate:"required"`
}

type MobiusEventBody struct {
	SubscriptionID    string                `json:"subscription_id" validate:"required"`
	AttemptedPayments int                   `json:"attempted_payments"`
	CompletedPayments int                   `json:"completed_payments"`
	BillingAddress    *MobiusBillingAddress `json:"billing_address"`
	Card              *MobiusCard           `json:"card"`
	Features          *MobiusFeatures       `json:"features"`
	Merchant          *MobiusMerchant       `json:"merchant"`
	NextChargeDate    string                `json:"next_charge_date"`
	OrderDescription  string                `json:"order_description"`
	OrderID           string                `json:"order_id"`
	Plan              MobiusPlan            `json:"plan"`
	PONumber          string                `json:"ponumber"`
	ProcessorID       string                `json:"processor_id"`
	RemainingPayments string                `json:"remaining_payments"`
	Shipping          string                `json:"shipping"`
	SubscriptionType  string                `json:"subscription_type"`
	Tax               string                `json:"tax"`
	Website           string                `json:"website"`

	// ACU-specific fields for payment method updates
	VaultID       string             `json:"vault_id"`       // Vault ID for ACU events
	PaymentMethod *MobiusPaymentInfo `json:"payment_method"` // Updated payment method info for ACU
}

// MobiusPaymentInfo represents updated payment method information from ACU
type MobiusPaymentInfo struct {
	LastFour   string `json:"last_four"`   // Last 4 digits of updated card
	CardType   string `json:"card_type"`   // Updated card type
	ExpiryDate string `json:"expiry_date"` // Updated expiry in MM/YY format
}

type MobiusBillingAddress struct {
	Address1   string `json:"address_1"`
	Address2   string `json:"address_2"`
	CellPhone  string `json:"cell_phone"`
	City       string `json:"city"`
	Company    string `json:"company"`
	Country    string `json:"country"`
	Email      string `json:"email" validate:"required,email"`
	Fax        string `json:"fax"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Phone      string `json:"phone"`
	PostalCode string `json:"postal_code"`
	State      string `json:"state"`
}

type MobiusCard struct {
	AVSResponse          string `json:"avs_response"`
	CardAvailableBalance string `json:"card_available_balance"`
	CardBalance          string `json:"card_balance"`
	CardholderAuth       string `json:"cardholder_auth"`
	CAVV                 string `json:"cavv"`
	CAVVResult           string `json:"cavv_result"`
	CCBin                string `json:"cc_bin"`
	CCExp                string `json:"cc_exp"`
	CCIssueNumber        string `json:"cc_issue_number"`
	CCNumber             string `json:"cc_number"`
	CCStartDate          string `json:"cc_start_date"`
	CCType               string `json:"cc_type"`
	CSCResponse          string `json:"csc_response"`
	ECI                  string `json:"eci"`
	EntryMode            string `json:"entry_mode"`
	XID                  string `json:"xid"`
}

type MobiusMerchant struct {
	ID   string `json:"id" validate:"required"`
	Name string `json:"name" validate:"required"`
}

type MobiusPlan struct {
	Amount         string `json:"amount"`
	DayFrequency   string `json:"day_frequency"`
	DayOfMonth     string `json:"day_of_month"`
	ID             string `json:"id" validate:"required"`
	MonthFrequency string `json:"month_frequency"`
	Name           string `json:"name"`
	Payments       string `json:"payments"`
}

type MobiusFeatures struct {
	IsTestMode bool `json:"is_test_mode"`
}

// -------------------------------- CCBill Webhook Types --------------------------------

type CCBillWebhookEvent struct {
	EventType CCBillWebhookEventType
	EventBody []byte
	Version   string // Detected or provided webhook version
}

// CCBillWebhookVersion represents supported CCBill webhook payload versions
type CCBillWebhookVersion string

const (
	CCBillVersionV2 CCBillWebhookVersion = "v2"
	CCBillVersionV4 CCBillWebhookVersion = "v4"
	CCBillVersionV5 CCBillWebhookVersion = "v5"
	CCBillVersionV7 CCBillWebhookVersion = "v7"
	CCBillVersionV8 CCBillWebhookVersion = "v8"
)

// CCBillVersionedPayload represents a versioned webhook payload that can be parsed
type CCBillVersionedPayload interface {
	GetVersion() CCBillWebhookVersion
	GetTransactionID() string
	GetSubscriptionID() string
	GetClientAccnum() string
	GetClientSubacc() string
	GetTimestamp() string
}

type CCBillCommonFields struct {
	ClientAccnum   int    `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`
}

// CCBillNewSaleSuccessEvent represents official CCBill NewSaleSuccess webhook v8 (April 2025)
type CCBillNewSaleSuccessEvent struct {
	// Core transaction fields
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	TransactionID  string `json:"transactionId" validate:"required"`
	ClientAccnum   string `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`

	// Customer information
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Address1    string `json:"address1"`
	City        string `json:"city"`
	State       string `json:"state"`
	Country     string `json:"country"`
	PostalCode  string `json:"postalCode"`
	Email       string `json:"email" validate:"required,email"`
	PhoneNumber string `json:"phoneNumber"`
	IPAddress   string `json:"ipAddress"`
	Username    string `json:"username"`
	Password    string `json:"password"`

	// Product information
	FormName                  string `json:"formName"`
	FlexID                    string `json:"flexId"`
	ProductDesc               string `json:"productDesc"`
	PriceDescription          string `json:"priceDescription"`
	RecurringPriceDescription string `json:"recurringPriceDescription"`

	// Pricing information
	BilledInitialPrice         string `json:"billedInitialPrice"`
	BilledRecurringPrice       string `json:"billedRecurringPrice"`
	BilledCurrencyCode         int    `json:"billedCurrencyCode"`
	SubscriptionInitialPrice   string `json:"subscriptionInitialPrice"`
	SubscriptionRecurringPrice string `json:"subscriptionRecurringPrice"`
	SubscriptionCurrencyCode   int    `json:"subscriptionCurrencyCode"`
	AccountingInitialPrice     string `json:"accountingInitialPrice"`
	AccountingRecurringPrice   string `json:"accountingRecurringPrice"`
	AccountingCurrencyCode     int    `json:"accountingCurrencyCode"`

	// Subscription details
	InitialPeriod      int    `json:"initialPeriod"`
	RecurringPeriod    int    `json:"recurringPeriod"`
	Rebills            int    `json:"rebills"`
	NextRenewalDate    string `json:"nextRenewalDate"`
	SubscriptionTypeID string `json:"subscriptionTypeId"`

	// Payment information
	PaymentType    string `json:"paymentType"`
	CardType       string `json:"cardType"`
	Bin            string `json:"bin"`
	PrePaid        string `json:"prePaid"`
	Last4          string `json:"last4"`
	ExpDate        string `json:"expDate"`
	AVSResponse    string `json:"avsResponse"`
	CVV2Response   string `json:"cvv2Response"`
	PaymentAccount string `json:"paymentAccount"`
	ThreeDSecure   string `json:"threeDSecure"`
	CardSubType    string `json:"cardSubType"`

	// Additional fields
	ReservationID                  string `json:"reservationId"`
	DynamicPricingValidationDigest string `json:"dynamicPricingValidationDigest"`
	AffiliateSystem                string `json:"affiliateSystem"`
	ReferringURL                   string `json:"referringUrl"`
	LifeTimeSubscription           int    `json:"lifeTimeSubscription"`
	LifeTimePrice                  string `json:"lifeTimePrice"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillNewSaleSuccessEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV8 }
func (e CCBillNewSaleSuccessEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillNewSaleSuccessEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillNewSaleSuccessEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillNewSaleSuccessEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillNewSaleSuccessEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillUpgradeSuccessEvent represents official CCBill UpgradeSuccess webhook v5 (April 2025)
// Contains all NewSaleSuccess v8 fields plus upgrade-specific fields
type CCBillUpgradeSuccessEvent struct {
	// Core transaction fields
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	TransactionID  string `json:"transactionId" validate:"required"`
	ClientAccnum   string `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`

	// Customer information
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Address1    string `json:"address1"`
	City        string `json:"city"`
	State       string `json:"state"`
	Country     string `json:"country"`
	PostalCode  string `json:"postalCode"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
	IPAddress   string `json:"ipAddress"`

	// Account and form information
	ReservationID string `json:"reservationId"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	FormName      string `json:"formName"`
	FlexID        string `json:"flexId"`

	// Product information
	ProductDesc               string `json:"productDesc"`
	PriceDescription          string `json:"priceDescription"`
	RecurringPriceDescription string `json:"recurringPriceDescription"`

	// Billing information
	BilledInitialPrice   string `json:"billedInitialPrice"`
	BilledRecurringPrice string `json:"billedRecurringPrice"`
	BilledCurrencyCode   int    `json:"billedCurrencyCode"`

	// Subscription information
	SubscriptionInitialPrice   string `json:"subscriptionInitialPrice"`
	SubscriptionRecurringPrice string `json:"subscriptionRecurringPrice"`
	SubscriptionCurrencyCode   int    `json:"subscriptionCurrencyCode"`

	// Accounting information
	AccountingInitialPrice   string `json:"accountingInitialPrice"`
	AccountingRecurringPrice string `json:"accountingRecurringPrice"`
	AccountingCurrencyCode   int    `json:"accountingCurrencyCode"`

	// Subscription terms
	InitialPeriod      int    `json:"initialPeriod"`
	RecurringPeriod    int    `json:"recurringPeriod"`
	Rebills            int    `json:"rebills"`
	NextRenewalDate    string `json:"nextRenewalDate"`
	SubscriptionTypeID string `json:"subscriptionTypeId"`

	// Security and validation
	DynamicPricingValidationDigest string `json:"dynamicPricingValidationDigest"`

	// Payment information
	PaymentType    string `json:"paymentType"`
	CardType       string `json:"cardType"`
	Bin            string `json:"bin"`
	PrePaid        string `json:"prePaid"`
	Last4          string `json:"last4"`
	ExpDate        string `json:"expDate"`
	AVSResponse    string `json:"avsResponse"`
	CVV2Response   string `json:"cvv2Response"`
	PaymentAccount string `json:"paymentAccount"`
	ThreeDSecure   string `json:"threeDSecure"`
	CardSubType    string `json:"cardSubType"`

	// Additional information
	AffiliateSystem      string `json:"affiliateSystem"`
	ReferringURL         string `json:"referringUrl"`
	LifeTimeSubscription int    `json:"lifeTimeSubscription"`
	LifeTimePrice        string `json:"lifeTimePrice"`

	// -------- UpgradeSuccess-only fields (v5) --------
	OriginalSubscriptionID string `json:"originalSubscriptionId"`
	OriginalClientAccnum   string `json:"originalClientAccnum"`
	OriginalClientSubacc   string `json:"originalClientSubacc"`
	Source                 string `json:"source"`            // FORM | API | PHONE
	SCAResponseStatus      string `json:"scaResponseStatus"` // E | Y | N | A | U | R

	// Convenience field for unified amount access (derived from billedInitialPrice for upgrades)
	Amount float64 `json:"-"` // Not marshaled, populated during processing
}

// Implement CCBillVersionedPayload interface
func (e CCBillUpgradeSuccessEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV5 }
func (e CCBillUpgradeSuccessEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillUpgradeSuccessEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillUpgradeSuccessEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillUpgradeSuccessEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillUpgradeSuccessEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillUpgradeFailureEvent represents official CCBill UpgradeFailure webhook v4 (April 2025)
// Contains all NewSaleFailure fields plus upgrade-specific fields
type CCBillUpgradeFailureEvent struct {
	// Core transaction fields
	TransactionID string `json:"transactionId" validate:"required"`
	ClientAccnum  string `json:"clientAccnum" validate:"required"`
	ClientSubacc  string `json:"clientSubacc" validate:"required"`
	Timestamp     string `json:"timestamp" validate:"required"`

	// Customer information
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Address1    string `json:"address1"`
	City        string `json:"city"`
	State       string `json:"state"`
	Country     string `json:"country"`
	PostalCode  string `json:"postalCode"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
	IPAddress   string `json:"ipAddress"`

	// Account and form information
	ReservationID string `json:"reservationId"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	FormName      string `json:"formName"`
	FlexID        string `json:"flexId"`

	// Product information
	PriceDescription          string `json:"priceDescription"`
	RecurringPriceDescription string `json:"recurringPriceDescription"`

	// Billing information
	BilledInitialPrice   string `json:"billedInitialPrice"`
	BilledRecurringPrice string `json:"billedRecurringPrice"`
	BilledCurrencyCode   int    `json:"billedCurrencyCode"`

	// Subscription information
	SubscriptionInitialPrice   string `json:"subscriptionInitialPrice"`
	SubscriptionRecurringPrice string `json:"subscriptionRecurringPrice"`
	SubscriptionCurrencyCode   int    `json:"subscriptionCurrencyCode"`

	// Accounting information
	AccountingInitialPrice   string `json:"accountingInitialPrice"`
	AccountingRecurringPrice string `json:"accountingRecurringPrice"`
	AccountingCurrencyCode   int    `json:"accountingCurrencyCode"`

	// Subscription terms
	InitialPeriod      int    `json:"initialPeriod"`
	RecurringPeriod    int    `json:"recurringPeriod"`
	Rebills            int    `json:"rebills"`
	SubscriptionTypeID string `json:"subscriptionTypeId"`

	// Security and validation
	DynamicPricingValidationDigest string `json:"dynamicPricingValidationDigest"`

	// Payment information
	PaymentType    string `json:"paymentType"`
	CardType       string `json:"cardType"`
	PrePaid        string `json:"prePaid"`
	AVSResponse    string `json:"avsResponse"`
	CVV2Response   string `json:"cvv2Response"`
	PaymentAccount string `json:"paymentAccount"`
	Last4          string `json:"last4"`
	ExpDate        string `json:"expDate"`
	ThreeDSecure   string `json:"threeDSecure"`

	// Additional information
	AffiliateSystem      string `json:"affiliateSystem"`
	ReferringURL         string `json:"referringUrl"`
	LifeTimeSubscription int    `json:"lifeTimeSubscription"`
	LifeTimePrice        string `json:"lifeTimePrice"`

	// Failure information
	FailureReason string `json:"failureReason"`
	FailureCode   string `json:"failureCode"`

	// -------- UpgradeFailure-specific fields (v4) --------
	OriginalSubscriptionID string                 `json:"originalSubscriptionId"`
	OriginalClientAccnum   int                    `json:"originalClientAccnum"`
	OriginalClientSubacc   string                 `json:"originalClientSubacc"`
	Source                 string                 `json:"source"` // FORM | API | PHONE
	Bin                    int                    `json:"bin"`
	SCAResponseStatus      string                 `json:"scaResponseStatus"` // E | Y | N | A | U | R
	CardSubType            string                 `json:"cardSubType"`       // v4 addition
	PassThrough            map[string]interface{} `json:"passThrough"`       // Custom pass-through data
}

// Implement CCBillVersionedPayload interface
func (e CCBillUpgradeFailureEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV4 }
func (e CCBillUpgradeFailureEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillUpgradeFailureEvent) GetSubscriptionID() string        { return "" } // UpgradeFailure may not have subscriptionId
func (e CCBillUpgradeFailureEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillUpgradeFailureEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillUpgradeFailureEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillBillingDateChangeEvent represents official CCBill BillingDateChange webhook v2 (Feb 2025)
type CCBillBillingDateChangeEvent struct {
	// Core fields
	SubscriptionID  string `json:"subscriptionId" validate:"required"`
	ClientAccnum    string `json:"clientAccnum" validate:"required"`
	ClientSubacc    string `json:"clientSubacc" validate:"required"`
	Timestamp       string `json:"timestamp" validate:"required"`
	NextRenewalDate string `json:"nextRenewalDate" validate:"required"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillBillingDateChangeEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV2 }
func (e CCBillBillingDateChangeEvent) GetTransactionID() string         { return "" }
func (e CCBillBillingDateChangeEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillBillingDateChangeEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillBillingDateChangeEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillBillingDateChangeEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillCustomerDataUpdateEvent represents official CCBill CustomerDataUpdate webhook v5 (Feb 2025)
type CCBillCustomerDataUpdateEvent struct {
	// Core fields
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	ClientAccnum   string `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`

	// Customer information
	FirstName      string `json:"firstName"`
	LastName       string `json:"lastName"`
	PaymentAccount string `json:"paymentAccount"`
	Address1       string `json:"address1"`
	City           string `json:"city"`
	State          string `json:"state"`
	Country        string `json:"country"`
	PostalCode     string `json:"postalCode"`
	Email          string `json:"email"`
	PhoneNumber    string `json:"phoneNumber"`
	IPAddress      string `json:"ipAddress"`
	ReservationID  string `json:"reservationId"`
	Username       string `json:"username"`
	Password       string `json:"password"`

	// Payment information
	PaymentType string `json:"paymentType"`
	CardType    string `json:"cardType"`
	Bin         int    `json:"bin"`
	ExpDate     string `json:"expDate"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillCustomerDataUpdateEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV5 }
func (e CCBillCustomerDataUpdateEvent) GetTransactionID() string         { return "" }
func (e CCBillCustomerDataUpdateEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillCustomerDataUpdateEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillCustomerDataUpdateEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillCustomerDataUpdateEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillUserReactivationEvent represents official CCBill UserReactivation webhook v2 (Feb 2025)
type CCBillUserReactivationEvent struct {
	// Core fields
	SubscriptionID  string `json:"subscriptionId" validate:"required"`
	TransactionID   string `json:"transactionId" validate:"required"`
	Price           string `json:"price" validate:"required"`
	ClientAccnum    string `json:"clientAccnum" validate:"required"`
	ClientSubacc    string `json:"clientSubacc" validate:"required"`
	Email           string `json:"email" validate:"required"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	NextRenewalDate string `json:"nextRenewalDate"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillUserReactivationEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV2 }
func (e CCBillUserReactivationEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillUserReactivationEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillUserReactivationEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillUserReactivationEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillUserReactivationEvent) GetTimestamp() string             { return "" } // UserReactivation doesn't have timestamp field

// CCBillRenewalSuccessEvent represents official CCBill RenewalSuccess webhook v7
type CCBillRenewalSuccessEvent struct {
	// Core transaction fields
	TransactionID  string `json:"transactionId" validate:"required"`
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	ClientAccnum   string `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`

	// Billing information
	BilledAmount       string `json:"billedAmount"`
	BilledCurrency     string `json:"billedCurrency"`
	BilledCurrencyCode int    `json:"billedCurrencyCode"`

	// Accounting information
	AccountingAmount       string `json:"accountingAmount"`
	AccountingCurrency     string `json:"accountingCurrency"`
	AccountingCurrencyCode int    `json:"accountingCurrencyCode"`

	// Renewal information
	NextRenewalDate string `json:"nextRenewalDate"`
	RenewalDate     string `json:"renewalDate"`

	// Payment information
	CardType       string `json:"cardType"`
	PaymentAccount string `json:"paymentAccount"`
	PaymentType    string `json:"paymentType"`
	Last4          string `json:"last4"`
	ExpDate        string `json:"expDate"`
	CardSubType    string `json:"cardSubType"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillRenewalSuccessEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV7 }
func (e CCBillRenewalSuccessEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillRenewalSuccessEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillRenewalSuccessEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillRenewalSuccessEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillRenewalSuccessEvent) GetTimestamp() string             { return e.Timestamp }

type CCBillCancellationEvent struct {
	CCBillCommonFields

	Reason string `json:"reason"`
	Source string `json:"source"`
}

type CCBillExpirationEvent struct {
	CCBillCommonFields
}

// CCBillRenewalFailureEvent represents official CCBill RenewalFailure webhook v5
type CCBillRenewalFailureEvent struct {
	// Core transaction fields
	TransactionID  string `json:"transactionId" validate:"required"`
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	ClientAccnum   string `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`

	// Failure information
	FailureReason string `json:"failureReason"`
	FailureCode   string `json:"failureCode"`
	NextRetryDate string `json:"nextRetryDate"`
	RenewalDate   string `json:"renewalDate"`

	// Payment information
	CardType    string `json:"cardType"`
	PaymentType string `json:"paymentType"`
	CardSubType string `json:"cardSubType"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillRenewalFailureEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV5 }
func (e CCBillRenewalFailureEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillRenewalFailureEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillRenewalFailureEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillRenewalFailureEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillRenewalFailureEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillNewSaleFailureEvent represents official CCBill NewSaleFailure webhook v5
type CCBillNewSaleFailureEvent struct {
	// Core transaction fields
	TransactionID string `json:"transactionId" validate:"required"`
	ClientAccnum  string `json:"clientAccnum" validate:"required"`
	ClientSubacc  string `json:"clientSubacc" validate:"required"`
	Timestamp     string `json:"timestamp" validate:"required"`

	// Customer information
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Address1    string `json:"address1"`
	City        string `json:"city"`
	State       string `json:"state"`
	Country     string `json:"country"`
	PostalCode  string `json:"postalCode"`
	Email       string `json:"email" validate:"required,email"`
	PhoneNumber string `json:"phoneNumber"`
	IPAddress   string `json:"ipAddress"`
	Username    string `json:"username"`
	Password    string `json:"password"`

	// Product information
	FormName                  string `json:"formName"`
	FlexID                    string `json:"flexId"`
	PriceDescription          string `json:"priceDescription"`
	RecurringPriceDescription string `json:"recurringPriceDescription"`

	// Pricing information
	BilledInitialPrice         string `json:"billedInitialPrice"`
	BilledRecurringPrice       string `json:"billedRecurringPrice"`
	BilledCurrencyCode         int    `json:"billedCurrencyCode"`
	SubscriptionInitialPrice   string `json:"subscriptionInitialPrice"`
	SubscriptionRecurringPrice string `json:"subscriptionRecurringPrice"`
	SubscriptionCurrencyCode   int    `json:"subscriptionCurrencyCode"`
	AccountingInitialPrice     string `json:"accountingInitialPrice"`
	AccountingRecurringPrice   string `json:"accountingRecurringPrice"`
	AccountingCurrencyCode     int    `json:"accountingCurrencyCode"`

	// Subscription details
	InitialPeriod      int    `json:"initialPeriod"`
	RecurringPeriod    int    `json:"recurringPeriod"`
	Rebills            int    `json:"rebills"`
	SubscriptionTypeID string `json:"subscriptionTypeId"`

	// Payment information
	PaymentType    string `json:"paymentType"`
	CardType       string `json:"cardType"`
	PrePaid        string `json:"prePaid"`
	AVSResponse    string `json:"avsResponse"`
	CVV2Response   string `json:"cvv2Response"`
	PaymentAccount string `json:"paymentAccount"`
	ThreeDSecure   string `json:"threeDSecure"`
	CardSubType    string `json:"cardSubType"`

	// Failure information
	FailureReason string `json:"failureReason"`
	FailureCode   string `json:"failureCode"`

	// Additional fields
	ReservationID                  string `json:"reservationId"`
	DynamicPricingValidationDigest string `json:"dynamicPricingValidationDigest"`
	AffiliateSystem                string `json:"affiliateSystem"`
	ReferringURL                   string `json:"referringUrl"`
	LifeTimeSubscription           int    `json:"lifeTimeSubscription"`
	LifeTimePrice                  string `json:"lifeTimePrice"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillNewSaleFailureEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV5 }
func (e CCBillNewSaleFailureEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillNewSaleFailureEvent) GetSubscriptionID() string        { return "" } // No subscription for failures
func (e CCBillNewSaleFailureEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillNewSaleFailureEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillNewSaleFailureEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillRefundEvent represents official CCBill Refund webhook v5
type CCBillRefundEvent struct {
	// Core transaction fields
	TransactionID  string `json:"transactionId" validate:"required"`
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	ClientAccnum   string `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`

	// Refund information
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	CurrencyCode int    `json:"currencyCode"`
	Reason       string `json:"reason"`

	// Accounting information
	AccountingAmount       string `json:"accountingAmount"`
	AccountingCurrency     string `json:"accountingCurrency"`
	AccountingCurrencyCode int    `json:"accountingCurrencyCode"`

	// Payment information
	CardType       string `json:"cardType"`
	PaymentAccount string `json:"paymentAccount"`
	PaymentType    string `json:"paymentType"`
	Last4          string `json:"last4"`
	ExpDate        string `json:"expDate"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillRefundEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV5 }
func (e CCBillRefundEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillRefundEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillRefundEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillRefundEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillRefundEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillChargebackEvent represents official CCBill Chargeback webhook v5
type CCBillChargebackEvent struct {
	// Core transaction fields
	TransactionID  string `json:"transactionId" validate:"required"`
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	ClientAccnum   string `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`

	// Chargeback information
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	CurrencyCode int    `json:"currencyCode"`
	Reason       string `json:"reason"`

	// Accounting information
	AccountingAmount       string `json:"accountingAmount"`
	AccountingCurrency     string `json:"accountingCurrency"`
	AccountingCurrencyCode int    `json:"accountingCurrencyCode"`

	// Payment information
	CardType       string `json:"cardType"`
	PaymentAccount string `json:"paymentAccount"`
	PaymentType    string `json:"paymentType"`
	Bin            string `json:"bin"`
	Last4          string `json:"last4"`
	ExpDate        string `json:"expDate"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillChargebackEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV5 }
func (e CCBillChargebackEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillChargebackEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillChargebackEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillChargebackEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillChargebackEvent) GetTimestamp() string             { return e.Timestamp }

// CCBillVoidEvent represents official CCBill Void webhook v5
type CCBillVoidEvent struct {
	// Core transaction fields
	TransactionID  string `json:"transactionId" validate:"required"`
	SubscriptionID string `json:"subscriptionId" validate:"required"`
	ClientAccnum   string `json:"clientAccnum" validate:"required"`
	ClientSubacc   string `json:"clientSubacc" validate:"required"`
	Timestamp      string `json:"timestamp" validate:"required"`

	// Void information
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	CurrencyCode int    `json:"currencyCode"`
	Reason       string `json:"reason"`

	// Accounting information
	AccountingAmount       string `json:"accountingAmount"`
	AccountingCurrency     string `json:"accountingCurrency"`
	AccountingCurrencyCode int    `json:"accountingCurrencyCode"`

	// Payment information
	CardType       string `json:"cardType"`
	PaymentAccount string `json:"paymentAccount"`
	PaymentType    string `json:"paymentType"`
	Last4          string `json:"last4"`
	ExpDate        string `json:"expDate"`
}

// Implement CCBillVersionedPayload interface
func (e CCBillVoidEvent) GetVersion() CCBillWebhookVersion { return CCBillVersionV5 }
func (e CCBillVoidEvent) GetTransactionID() string         { return e.TransactionID }
func (e CCBillVoidEvent) GetSubscriptionID() string        { return e.SubscriptionID }
func (e CCBillVoidEvent) GetClientAccnum() string          { return e.ClientAccnum }
func (e CCBillVoidEvent) GetClientSubacc() string          { return e.ClientSubacc }
func (e CCBillVoidEvent) GetTimestamp() string             { return e.Timestamp }

type GrantRoleForSubscriptionParams struct {
	userID         uuid.UUID
	subscriptionID uuid.UUID
	price          *models.Price
	product        *models.Product
	processor      models.ProcessorType
}

func newGrantRoleParams(userID, subscriptionID uuid.UUID, processor models.ProcessorType, price *models.Price, product *models.Product, db *db.DB) GrantRoleForSubscriptionParams {
	return GrantRoleForSubscriptionParams{
		price:          price,
		userID:         userID,
		product:        product,
		processor:      processor,
		subscriptionID: subscriptionID,
		purchaseRepo:   repo.NewPurchaseRepo(db),
		roleGrantRepo:  repo.NewUserRoleGrantRepo(db),
	}
}

func grantRole(ctx context.Context, params GrantRoleForSubscriptionParams) error {
	price := params.price
	userID := params.userID
	product := params.product
	purchaseRepo := params.purchaseRepo
	roleGrantRepo := params.roleGrantRepo
	subscriptionID := params.subscriptionID

	if product.RoleID == nil {
		log.WithContext(ctx).Info("Product has no role associated, skipping role grant")
		return nil
	}

	var extensionDays int
	if product.RoleDurationDays != nil && *product.RoleDurationDays > 0 {
		extensionDays = *product.RoleDurationDays
	} else if price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
		extensionDays = *price.BillingCycleDays
	} else {
		extensionDays = 30 // Default fallback
	}

	// Create Purchase event for this subscription payment
	purchase := &models.Purchase{
		ID:            uuid.New(),
		UserID:        userID,
		PriceID:       price.ID,
		Amount:        price.Amount,
		Currency:      price.Currency,
		ExtensionDays: &extensionDays,
		Processor:     params.processor,
		TransactionID: subscriptionID.String(),
		PurchasedAt:   time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	grant, _, err := roleGrantRepo.ExtendRoleExpiration(ctx, userID, *product.RoleID, extensionDays)
	if err != nil {
		return fmt.Errorf("failed to extend role expiration: %w", err)
	}

	purchase.UserRoleGrantID = &grant.ID
	if err := purchaseRepo.Create(ctx, purchase); err != nil {
		return fmt.Errorf("failed to create purchase event: %w", err)
	}

	log.WithContext(ctx).WithFields(log.Fields{
		"userID":         userID,
		"subscriptionID": subscriptionID,
		"roleID":         *product.RoleID,
	}).Info("Granted role for subscription")

	return nil
}

func validateSubscription(sub *models.Subscription, newStatus models.SubscriptionStatus, amount float64) error {
	if sub.CurrentPeriodEndsAt != nil && sub.CurrentPeriodEndsAt.Before(time.Now()) {
		if newStatus == models.StatusActive {
			return fmt.Errorf("cannot activate expired subscription without proper renewal")
		}
	}

	if newStatus == models.StatusActive && amount <= 0 {
		return fmt.Errorf("cannot activate subscription with invalid amount: %.2f", amount)
	}

	if newStatus == models.StatusPastDue {
		if sub.RetryAttempts != nil && *sub.RetryAttempts >= 3 {
			return fmt.Errorf("subscription has exceeded maximum retry attempts, should be cancelled")
		}
	}

	return nil
}
