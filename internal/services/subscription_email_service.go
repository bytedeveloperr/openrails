package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	repo "github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/google/uuid"
)

var errUserEmailUnavailable = errors.New("user email unavailable")

// SubscriptionEmailService handles subscription-related email notifications
// This service is called directly by other services when subscription events occur
type SubscriptionEmailService struct {
	emailService        *EmailService
	subscriptionService *SubscriptionService
	productService      *ProductService
	priceService        *PriceService
	profiles            *repo.ProfileRepo
}

// NewSubscriptionEmailService creates a new subscription email service
func NewSubscriptionEmailService(
	emailService *EmailService,
	subscriptionService *SubscriptionService,
	productService *ProductService,
	priceService *PriceService,
	profiles *repo.ProfileRepo,
) *SubscriptionEmailService {
	return &SubscriptionEmailService{
		emailService:        emailService,
		subscriptionService: subscriptionService,
		productService:      productService,
		priceService:        priceService,
		profiles:            profiles,
	}
}

// SendSubscriptionConfirmed sends a subscription confirmation email
func (s *SubscriptionEmailService) SendSubscriptionConfirmed(ctx context.Context, userID string) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping subscription confirmation email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping subscription confirmation email", userID)
			return nil
		}
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendSubscriptionConfirmation(ctx, *emailData)
}

// SendSubscriptionRenewed sends a subscription renewal email
func (s *SubscriptionEmailService) SendSubscriptionRenewed(ctx context.Context, userID string) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping subscription renewal email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping subscription renewal email", userID)
			return nil
		}
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendSubscriptionRenewal(ctx, *emailData)
}

// SendPremiumEnded sends the appropriate email when a premium entitlement ends.
func (s *SubscriptionEmailService) SendPremiumEnded(ctx context.Context, userID string, reason PremiumEndReason) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping premium-ended email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping premium-ended email", userID)
			return nil
		}
		return fmt.Errorf("failed to get email data: %w", err)
	}

	switch reason {
	case PremiumEndReasonExpired:
		return s.emailService.SendSubscriptionExpired(ctx, *emailData)
	case PremiumEndReasonChargeback, PremiumEndReasonRefund, PremiumEndReasonAdmin, PremiumEndReasonProcessor:
		return s.emailService.SendSubscriptionCancellation(ctx, *emailData, reason)
	case PremiumEndReasonUserCancel:
		fallthrough
	case PremiumEndReasonUnknown:
		return s.emailService.SendSubscriptionCancellation(ctx, *emailData, PremiumEndReasonUserCancel)
	default:
		return s.emailService.SendSubscriptionCancellation(ctx, *emailData, reason)
	}
}

// SendPaymentFailed sends a payment failure email
func (s *SubscriptionEmailService) SendPaymentFailed(ctx context.Context, userID string) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping payment failure email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping payment failure email", userID)
			return nil
		}
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendPaymentFailed(ctx, *emailData)
}

// SendEntitlementExpired sends an entitlement expiration email
func (s *SubscriptionEmailService) SendEntitlementExpired(ctx context.Context, userID string, entitlementName string, expiresAt time.Time) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping entitlement expiration email")
		return nil
	}

	username, email, err := s.getUserEmail(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserEmailUnavailable) {
			log.Printf("Email unavailable for user %s - skipping entitlement expiration email", userID)
			return nil
		}
		return fmt.Errorf("failed to get user profile: %w", err)
	}

	return s.emailService.SendEntitlementExpiration(ctx, email, username, entitlementName, expiresAt)
}

// getEmailData fetches subscription data for email notifications
func (s *SubscriptionEmailService) getEmailData(ctx context.Context, userID string) (*SubscriptionEmailData, error) {
	username, email, err := s.getUserEmail(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get the user's active subscription or last known subscription as fallback
	subscription, err := s.subscriptionService.GetActiveSubscription(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			subscription, err = s.subscriptionService.GetByUserID(ctx, userID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, errUserEmailUnavailable
				}
				return nil, fmt.Errorf("failed to get subscription for user %s: %w", userID, err)
			}
		} else {
			return nil, fmt.Errorf("failed to get active subscription: %w", err)
		}
	}

	// Email address comes from the user directory; subscription no longer caches it.

	// Get the price details
	price, err := s.priceService.GetByID(ctx, subscription.PriceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}

	// Calculate billing period based on subscription and price interval
	periodStart := time.Now()
	periodEnd := time.Now()
	if subscription.CurrentPeriodStartsAt != nil {
		periodStart = *subscription.CurrentPeriodStartsAt
		if price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
			periodEnd = periodStart.AddDate(0, 0, *price.BillingCycleDays)
		} else {
			periodEnd = periodStart.AddDate(0, 1, 0) // Default to monthly for one-time purchases
		}
		if subscription.CurrentPeriodEndsAt != nil {
			periodEnd = *subscription.CurrentPeriodEndsAt
		}
	}

	paymentMethod := describePaymentMethod(subscription)
	if paymentMethod == "" {
		paymentMethod = processorDisplayName(subscription.Processor)
		if paymentMethod == "" {
			paymentMethod = "Credit Card"
		}
	}

	return &SubscriptionEmailData{
		UserEmail:      email,
		Username:       username,
		SubscriptionID: subscription.ID,
		Amount:         price.Amount,
		Currency:       price.Currency,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		PaymentMethod:  paymentMethod,
		TransactionID:  "", // Would come from payment processor
	}, nil
}

// getUserProfile gets user profile and validates email exists
func (s *SubscriptionEmailService) getUserEmail(ctx context.Context, userID string) (username string, email string, err error) {
	if s.profiles == nil {
		return "", "", errUserEmailUnavailable
	}
	uid, perr := uuid.Parse(userID)
	if perr != nil {
		return "", "", errUserEmailUnavailable
	}
	uname, mail, verified, active, qerr := s.profiles.GetUserEmail(ctx, uid)
	if qerr != nil || mail == "" || !active {
		return "", "", errUserEmailUnavailable
	}
	_ = verified // reserved for future policy checks
	return uname, mail, nil
}

func describePaymentMethod(subscription *models.Subscription) string {
	if subscription == nil || subscription.PaymentMethod == nil {
		return ""
	}

	pm := subscription.PaymentMethod
	cardType := ""
	if pm.CardType != nil {
		cardType = strings.TrimSpace(*pm.CardType)
	}
	lastFour := ""
	if pm.LastFour != nil {
		lastFour = strings.TrimSpace(*pm.LastFour)
	}

	parts := make([]string, 0, 2)
	if cardType != "" {
		parts = append(parts, cardType)
	} else {
		friendly := processorDisplayName(pm.Processor)
		if friendly != "" {
			parts = append(parts, friendly)
		}
	}

	if lastFour != "" {
		parts = append(parts, fmt.Sprintf("••••%s", lastFour))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " ")
}

func processorDisplayName(processor models.Processor) string {
	switch processor {
	case models.ProcessorNMI, models.ProcessorCCBill:
		return "Credit Card"
	case models.ProcessorPayPal:
		return "PayPal"
	case models.ProcessorSolana:
		return "Solana"
	default:
		clean := strings.TrimSpace(string(processor))
		if clean == "" {
			return ""
		}
		return strings.ToUpper(clean)
	}
}
