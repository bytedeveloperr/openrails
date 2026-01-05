package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/doujins-org/doujins-billing/pkg/api"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

const (
	checkoutSessionIdempotencyOp = "checkout_session_create"
	defaultCheckoutSessionTTL    = 15 * time.Minute
)

var (
	ErrCheckoutSessionValidation = errors.New("checkout session validation failed")
	ErrCheckoutSessionNotFound   = errors.New("checkout session not found")
	ErrCheckoutSessionForbidden  = errors.New("checkout session access denied")
	ErrCheckoutSessionExpired    = errors.New("checkout session expired")
	ErrCheckoutSessionPending    = errors.New("checkout session request already pending")
	ErrCheckoutSessionConflict   = errors.New("checkout session conflict")
)

type CheckoutSessionPaymentRequest struct {
	Processor       string
	PaymentMethodID string
	PaymentToken    string

	TokenSymbol string
	Flow        string
	Wallet      string

	Email     string
	FirstName string
	LastName  string
	Address1  string
	City      string
	State     string
	Zip       string
	Country   string
}

type CheckoutSessionCreateRequest struct {
	PriceID        string
	Mode           string
	Payment        CheckoutSessionPaymentRequest
	Metadata       map[string]string
	IdempotencyKey string
}

type CheckoutSessionConfirmPayment struct {
	Processor string
	Signature string
	Wallet    string
}

type CheckoutSessionConfirmRequest struct {
	Payment CheckoutSessionConfirmPayment
}

type CheckoutSessionRedirectToURL struct {
	URL       string `json:"url,omitempty"`
	ReturnURL string `json:"return_url,omitempty"`
}

type CheckoutSessionNextAction struct {
	Type          string                        `json:"type"`
	RedirectToURL *CheckoutSessionRedirectToURL `json:"redirect_to_url,omitempty"`
}

type CheckoutSessionPaymentResponse struct {
	Processor       string `json:"processor"`
	Reference       string `json:"reference,omitempty"`
	TransactionURL  string `json:"transaction_url,omitempty"`
	TransactionData string `json:"transaction_data,omitempty"`
	RedirectURL     string `json:"redirect_url,omitempty"`
	TransactionID   string `json:"transaction_id,omitempty"`
}

type CheckoutSessionResponse struct {
	Object         string                         `json:"object"`
	ID             string                         `json:"id"`
	Status         string                         `json:"status"`
	Mode           string                         `json:"mode"`
	PriceID        string                         `json:"price_id"`
	Payment        CheckoutSessionPaymentResponse `json:"payment"`
	PaymentID      *string                        `json:"payment_id,omitempty"`
	SubscriptionID *string                        `json:"subscription_id,omitempty"`
	ExpiresAt      *time.Time                     `json:"expires_at,omitempty"`
	NextAction     *CheckoutSessionNextAction     `json:"next_action,omitempty"`
	Message        string                         `json:"message,omitempty"`
}

type CheckoutSessionService struct {
	db                   *db.DB
	repo                 *repo.CheckoutSessionRepo
	priceService         *PriceService
	productService       *ProductService
	paymentMethodService *PaymentMethodService
	idempotencyService   *IdempotencyService
	Clock                clockwork.Clock
}

func NewCheckoutSessionService(
	db *db.DB,
	priceService *PriceService,
	productService *ProductService,
	paymentMethodService *PaymentMethodService,
	idempotencyService *IdempotencyService,
) *CheckoutSessionService {
	return &CheckoutSessionService{
		db:                   db,
		repo:                 repo.NewCheckoutSessionRepo(db),
		priceService:         priceService,
		productService:       productService,
		paymentMethodService: paymentMethodService,
		idempotencyService:   idempotencyService,
	}
}

func (s *CheckoutSessionService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now()
}

func (s *CheckoutSessionService) CreateSession(ctx context.Context, req *CheckoutSessionCreateRequest, user *UserIdentity) (*CheckoutSessionResponse, error) {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return nil, fmt.Errorf("%w: user is required", ErrCheckoutSessionValidation)
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrCheckoutSessionValidation)
	}

	claimed := false
	if s.idempotencyService != nil && strings.TrimSpace(req.IdempotencyKey) != "" {
		rec, exists, err := s.idempotencyService.Begin(ctx, checkoutSessionIdempotencyOp, req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if exists {
			switch rec.Status {
			case IdempotencyStatusSuccess:
				var cached CheckoutSessionResponse
				if err := json.Unmarshal(rec.Result, &cached); err != nil {
					return nil, fmt.Errorf("failed to decode cached response: %w", err)
				}
				return &cached, nil
			case IdempotencyStatusPending:
				return nil, ErrCheckoutSessionPending
			case IdempotencyStatusFailed:
				return nil, fmt.Errorf("%w: previous request failed: %s", ErrCheckoutSessionConflict, rec.Error)
			}
		}
		claimed = true
	}

	resp, err := s.createSessionWithValidation(ctx, req, user)
	if err != nil {
		if claimed && s.idempotencyService != nil && strings.TrimSpace(req.IdempotencyKey) != "" {
			_ = s.idempotencyService.Fail(ctx, checkoutSessionIdempotencyOp, req.IdempotencyKey, err)
		}
		return nil, err
	}

	if claimed && s.idempotencyService != nil && strings.TrimSpace(req.IdempotencyKey) != "" {
		payload, _ := json.Marshal(resp)
		_ = s.idempotencyService.Complete(ctx, checkoutSessionIdempotencyOp, req.IdempotencyKey, payload)
	}

	return resp, nil
}

func (s *CheckoutSessionService) createSessionWithValidation(ctx context.Context, req *CheckoutSessionCreateRequest, user *UserIdentity) (*CheckoutSessionResponse, error) {
	if strings.TrimSpace(req.PriceID) == "" {
		return nil, fmt.Errorf("%w: price_id is required", ErrCheckoutSessionValidation)
	}
	if strings.TrimSpace(req.Payment.Processor) == "" {
		return nil, fmt.Errorf("%w: payment.processor is required", ErrCheckoutSessionValidation)
	}

	priceID, err := api.ParsePriceID(req.PriceID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid price_id", ErrCheckoutSessionValidation)
	}
	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("%w: price not found", ErrCheckoutSessionValidation)
	}
	if !price.IsActive {
		return nil, fmt.Errorf("%w: price is not active", ErrCheckoutSessionValidation)
	}
	product, err := s.productService.GetByID(ctx, price.ProductID)
	if err != nil {
		return nil, fmt.Errorf("%w: product not found", ErrCheckoutSessionValidation)
	}
	if !product.IsActive {
		return nil, fmt.Errorf("%w: product is not active", ErrCheckoutSessionValidation)
	}

	processor := strings.ToLower(strings.TrimSpace(req.Payment.Processor))
	mode, err := s.resolveMode(req.Mode, processor, price)
	if err != nil {
		return nil, err
	}

	if err := s.validatePayment(ctx, processor, &req.Payment, user); err != nil {
		return nil, err
	}

	now := s.now()
	session := &models.CheckoutSession{
		ID:              uuid.New(),
		UserID:          user.ID,
		PriceID:         price.ID,
		Mode:            mode,
		Processor:       models.Processor(processor),
		Status:          models.CheckoutSessionStatusCreated,
		Amount:          price.Amount,
		Currency:        price.Currency,
		ExpiresAt:       timePtr(now.Add(defaultCheckoutSessionTTL)),
		ProcessorFields: s.buildProcessorFields(processor, &req.Payment),
		ProcessorState:  map[string]any{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if strings.TrimSpace(req.IdempotencyKey) != "" {
		session.IdempotencyKey = stringPtr(strings.TrimSpace(req.IdempotencyKey))
	}

	if err := s.repo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create checkout session: %w", err)
	}

	return s.sessionToResponse(session), nil
}

func (s *CheckoutSessionService) GetSession(ctx context.Context, sessionID uuid.UUID, user *UserIdentity) (*CheckoutSessionResponse, error) {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, ErrCheckoutSessionNotFound
	}
	if user == nil || strings.TrimSpace(user.ID) == "" || session.UserID != user.ID {
		return nil, ErrCheckoutSessionForbidden
	}

	if s.isExpired(session) && !s.isTerminal(session.Status) {
		session.Status = models.CheckoutSessionStatusExpired
		session.UpdatedAt = s.now()
		if updateErr := s.repo.Update(ctx, session); updateErr != nil {
			return nil, fmt.Errorf("failed to update expired session: %w", updateErr)
		}
	}

	return s.sessionToResponse(session), nil
}

func (s *CheckoutSessionService) ConfirmSession(ctx context.Context, sessionID uuid.UUID, req *CheckoutSessionConfirmRequest, user *UserIdentity) (*CheckoutSessionResponse, error) {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, ErrCheckoutSessionNotFound
	}
	if user == nil || strings.TrimSpace(user.ID) == "" || session.UserID != user.ID {
		return nil, ErrCheckoutSessionForbidden
	}

	if s.isExpired(session) {
		return nil, ErrCheckoutSessionExpired
	}
	if s.isTerminal(session.Status) {
		return nil, ErrCheckoutSessionConflict
	}

	processor := strings.ToLower(strings.TrimSpace(req.Payment.Processor))
	if processor == "" {
		return nil, fmt.Errorf("%w: payment.processor is required", ErrCheckoutSessionValidation)
	}
	if processor != strings.ToLower(string(session.Processor)) {
		return nil, fmt.Errorf("%w: processor mismatch", ErrCheckoutSessionValidation)
	}

	return nil, fmt.Errorf("%w: confirmation not implemented for processor %s", ErrCheckoutSessionConflict, processor)
}

func (s *CheckoutSessionService) resolveMode(mode string, processor string, price *models.Price) (models.CheckoutSessionMode, error) {
	if processor == "" {
		return "", fmt.Errorf("%w: processor is required", ErrCheckoutSessionValidation)
	}

	trimmedMode := strings.TrimSpace(mode)
	if processor == "solana" {
		if trimmedMode == string(models.CheckoutSessionModeSubscription) {
			return "", fmt.Errorf("%w: solana does not support subscription mode", ErrCheckoutSessionValidation)
		}
		return models.CheckoutSessionModeOneOff, nil
	}

	expected := models.CheckoutSessionModeOneOff
	if price.BillingCycleDays != nil {
		expected = models.CheckoutSessionModeSubscription
	}
	if trimmedMode == "" {
		return expected, nil
	}
	if trimmedMode != string(expected) {
		return "", fmt.Errorf("%w: mode does not match price configuration", ErrCheckoutSessionValidation)
	}
	return models.CheckoutSessionMode(trimmedMode), nil
}

func (s *CheckoutSessionService) validatePayment(ctx context.Context, processor string, payment *CheckoutSessionPaymentRequest, user *UserIdentity) error {
	switch processor {
	case "mobius", "stripe":
		hasToken := strings.TrimSpace(payment.PaymentToken) != ""
		hasMethod := strings.TrimSpace(payment.PaymentMethodID) != ""
		if hasToken == hasMethod {
			return fmt.Errorf("%w: provide either payment_token or payment_method_id", ErrCheckoutSessionValidation)
		}
		if hasMethod {
			pmID, err := api.ParsePaymentMethodID(payment.PaymentMethodID)
			if err != nil {
				return fmt.Errorf("%w: invalid payment_method_id", ErrCheckoutSessionValidation)
			}
			if s.paymentMethodService == nil {
				return fmt.Errorf("%w: payment method service unavailable", ErrCheckoutSessionValidation)
			}
			if err := s.paymentMethodService.ValidateOwnership(ctx, pmID, user.ID); err != nil {
				return fmt.Errorf("%w: payment method not authorized", ErrCheckoutSessionValidation)
			}
		}
	case "solana":
		if strings.TrimSpace(payment.TokenSymbol) == "" {
			return fmt.Errorf("%w: token_symbol is required", ErrCheckoutSessionValidation)
		}
	case "ccbill":
		if err := requireBillingFields(payment); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: unsupported processor", ErrCheckoutSessionValidation)
	}
	return nil
}

func requireBillingFields(payment *CheckoutSessionPaymentRequest) error {
	if strings.TrimSpace(payment.Email) == "" ||
		strings.TrimSpace(payment.FirstName) == "" ||
		strings.TrimSpace(payment.LastName) == "" ||
		strings.TrimSpace(payment.Address1) == "" ||
		strings.TrimSpace(payment.City) == "" ||
		strings.TrimSpace(payment.Zip) == "" ||
		strings.TrimSpace(payment.Country) == "" {
		return fmt.Errorf("%w: billing fields are required", ErrCheckoutSessionValidation)
	}
	return nil
}

func (s *CheckoutSessionService) buildProcessorFields(processor string, payment *CheckoutSessionPaymentRequest) map[string]any {
	fields := map[string]any{
		"processor": processor,
	}

	addField(fields, "payment_method_id", payment.PaymentMethodID)
	addField(fields, "token_symbol", payment.TokenSymbol)
	addField(fields, "flow", payment.Flow)
	addField(fields, "wallet", payment.Wallet)
	addField(fields, "email", payment.Email)
	addField(fields, "first_name", payment.FirstName)
	addField(fields, "last_name", payment.LastName)
	addField(fields, "address1", payment.Address1)
	addField(fields, "city", payment.City)
	addField(fields, "state", payment.State)
	addField(fields, "zip", payment.Zip)
	addField(fields, "country", payment.Country)

	return fields
}

func addField(fields map[string]any, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fields[key] = strings.TrimSpace(value)
}

func (s *CheckoutSessionService) sessionToResponse(session *models.CheckoutSession) *CheckoutSessionResponse {
	resp := &CheckoutSessionResponse{
		Object:  "checkout_session",
		ID:      api.FormatCheckoutSessionID(session.ID),
		Status:  string(session.Status),
		Mode:    string(session.Mode),
		PriceID: api.FormatPriceID(session.PriceID),
		Payment: CheckoutSessionPaymentResponse{
			Processor: string(session.Processor),
		},
		ExpiresAt: session.ExpiresAt,
	}

	if session.Reference != nil {
		resp.Payment.Reference = *session.Reference
	}
	if session.TransactionID != nil {
		resp.Payment.TransactionID = *session.TransactionID
	}

	if session.PaymentID != nil {
		paymentID := api.FormatPaymentID(*session.PaymentID)
		resp.PaymentID = &paymentID
	}
	if session.SubscriptionID != nil {
		subID := api.FormatSubscriptionID(*session.SubscriptionID)
		resp.SubscriptionID = &subID
	}

	if session.ProcessorState != nil {
		if val, ok := session.ProcessorState["transaction_url"].(string); ok && strings.TrimSpace(val) != "" {
			resp.Payment.TransactionURL = val
		}
		if val, ok := session.ProcessorState["transaction_data"].(string); ok && strings.TrimSpace(val) != "" {
			resp.Payment.TransactionData = val
		}
		if val, ok := session.ProcessorState["redirect_url"].(string); ok && strings.TrimSpace(val) != "" {
			resp.Payment.RedirectURL = val
		}
	}

	return resp
}

func (s *CheckoutSessionService) isTerminal(status models.CheckoutSessionStatus) bool {
	switch status {
	case models.CheckoutSessionStatusSucceeded,
		models.CheckoutSessionStatusFailed,
		models.CheckoutSessionStatusExpired,
		models.CheckoutSessionStatusCanceled:
		return true
	default:
		return false
	}
}

func (s *CheckoutSessionService) isExpired(session *models.CheckoutSession) bool {
	if session.ExpiresAt == nil || session.ExpiresAt.IsZero() {
		return false
	}
	return session.ExpiresAt.Before(s.now())
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	val := strings.TrimSpace(value)
	return &val
}
