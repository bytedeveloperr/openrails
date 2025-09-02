package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/doujins-org/doujins-billing/internal/db/models"
)

// SubscriptionEmailService handles subscription-related email notifications
// This service is called directly by other services when subscription events occur
type SubscriptionEmailService struct {
	emailService        *EmailService
	userServicesitory   *UserService // Unified repository
	subscriptionService *SubscriptionService
	productService      *ProductService
	priceService        *PriceService
}

// NewSubscriptionEmailService creates a new subscription email service
func NewSubscriptionEmailService(
	emailService *EmailService,
	userService *UserService,
	subscriptionService *SubscriptionService,
	productService *ProductService,
	priceService *PriceService,
) *SubscriptionEmailService {
	return &SubscriptionEmailService{
		emailService:        emailService,
		userServicesitory:   userService,
		subscriptionService: subscriptionService,
		productService:      productService,
		priceService:        priceService,
	}
}

// SendSubscriptionConfirmed sends a subscription confirmation email
func (s *SubscriptionEmailService) SendSubscriptionConfirmed(ctx context.Context, userID uuid.UUID) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping subscription confirmation email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendSubscriptionConfirmation(ctx, *emailData)
}

// SendSubscriptionRenewed sends a subscription renewal email
func (s *SubscriptionEmailService) SendSubscriptionRenewed(ctx context.Context, userID uuid.UUID) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping subscription renewal email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendSubscriptionRenewal(ctx, *emailData)
}

// SendSubscriptionCancelled sends a subscription cancellation email
func (s *SubscriptionEmailService) SendSubscriptionCancelled(ctx context.Context, userID uuid.UUID, subscriptionType, amount string) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping subscription cancellation email")
		return nil
	}

	// For cancelled subscriptions, we might not have active subscription data
	// so we accept the subscription details as parameters
	profile, email, err := s.getUserProfile(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user profile: %w", err)
	}

	// Parse amount string to float64
	var amountFloat float64
	if _, err := fmt.Sscanf(amount, "$%f", &amountFloat); err != nil {
		amountFloat = 0 // Default if parsing fails
	}

	emailData := SubscriptionEmailData{
		UserEmail:      email,
		Username:       profile.Username,
		SubscriptionID: uuid.Nil, // We don't have the subscription ID in this context
		Amount:         amountFloat,
		Currency:       "USD", // Default for cancellation emails without price context
		PeriodStart:    time.Now(),
		PeriodEnd:      time.Now(),
		PaymentMethod:  "Credit Card",
		TransactionID:  "",
	}

	return s.emailService.SendSubscriptionCancellation(ctx, emailData)
}

// SendPaymentFailed sends a payment failure email
func (s *SubscriptionEmailService) SendPaymentFailed(ctx context.Context, userID uuid.UUID) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping payment failure email")
		return nil
	}

	emailData, err := s.getEmailData(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get email data: %w", err)
	}

	return s.emailService.SendPaymentFailed(ctx, *emailData)
}

// SendRoleExpired sends a role expiration email
func (s *SubscriptionEmailService) SendRoleExpired(ctx context.Context, userID uuid.UUID, roleName string, expiresAt time.Time) error {
	if s.emailService == nil || !s.emailService.IsEnabled() {
		log.Println("Email service not available - skipping role expiration email")
		return nil
	}

	profile, email, err := s.getUserProfile(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user profile: %w", err)
	}

	return s.emailService.SendRoleExpiration(ctx, email, profile.Username, roleName, expiresAt)
}

// getEmailData fetches subscription data for email notifications
func (s *SubscriptionEmailService) getEmailData(ctx context.Context, userID uuid.UUID) (*SubscriptionEmailData, error) {
	profile, email, err := s.getUserProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get the user's active subscription
	subscription, err := s.subscriptionService.GetActiveSubscription(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active subscription: %w", err)
	}

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

	return &SubscriptionEmailData{
		UserEmail:      email,
		Username:       profile.Username,
		SubscriptionID: subscription.ID,
		Amount:         price.Amount,
		Currency:       price.Currency,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		PaymentMethod:  "Credit Card", // Default, could be enhanced to get actual payment method
		TransactionID:  "",            // Would come from payment processor
	}, nil
}

// getUserProfile gets user profile and validates email exists
func (s *SubscriptionEmailService) getUserProfile(ctx context.Context, userID uuid.UUID) (*models.Profile, string, error) {
	// Get user profile
	profile, err := s.userServicesitory.GetByUserID(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get profile for user %s: %w", userID, err)
	}

	// Get user data including email
	user, err := s.userServicesitory.GetByID(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get user data for %s: %w", userID, err)
	}

	if user.Email == nil || *user.Email == "" {
		return nil, "", fmt.Errorf("user %s has no email address", userID)
	}

	email := *user.Email
	return profile, email, nil
}
