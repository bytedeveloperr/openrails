package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SubscriptionEmailData contains data for subscription-related emails
type SubscriptionEmailData struct {
	UserEmail      string
	Username       string
	SubscriptionID uuid.UUID
	Amount         float64
	Currency       string
	PeriodStart    time.Time
	PeriodEnd      time.Time
	PaymentMethod  string
	TransactionID  string
}

// SendSubscriptionConfirmation sends a confirmation email when subscription is created
func (s *EmailService) SendSubscriptionConfirmation(ctx context.Context, data SubscriptionEmailData) error {
	subject := "Welcome to Doujins Premium! Your subscription is confirmed"

	htmlContent := fmt.Sprintf(`
		<h2>Welcome to Doujins Premium!</h2>
		<p>Hi %s,</p>
		<p>Your premium subscription has been successfully activated. Thank you for supporting Doujins!</p>
		
		<h3>Subscription Details:</h3>
		<ul>
			<li><strong>Subscription ID:</strong> %s</li>
			<li><strong>Amount:</strong> $%.2f %s</li>
			<li><strong>Current Period:</strong> %s to %s</li>
			<li><strong>Payment Method:</strong> %s</li>
		</ul>
		
		<p>You now have access to:</p>
		<ul>
			<li>Exclusive premium content</li>
			<li>High-definition streaming</li>
			<li>Priority support</li>
			<li>Ad-free browsing experience</li>
		</ul>
		
		<p>Enjoy your premium experience!</p>
		<p>The Doujins Team</p>
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency,
		data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"), data.PaymentMethod)

	plainContent := fmt.Sprintf(`
		Welcome to Doujins Premium!
		
		Hi %s,
		
		Your premium subscription has been successfully activated. Thank you for supporting Doujins!
		
		Subscription Details:
		- Subscription ID: %s
		- Amount: $%.2f %s
		- Current Period: %s to %s
		- Payment Method: %s
		
		You now have access to exclusive premium content, HD streaming, priority support, and ad-free browsing.
		
		Enjoy your premium experience!
		The Doujins Team
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency,
		data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"), data.PaymentMethod)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

// SendSubscriptionRenewal sends notification when subscription is renewed
func (s *EmailService) SendSubscriptionRenewal(ctx context.Context, data SubscriptionEmailData) error {
	subject := "Your Doujins Premium subscription has been renewed"

	htmlContent := fmt.Sprintf(`
		<h2>Subscription Renewed Successfully</h2>
		<p>Hi %s,</p>
		<p>Your Doujins Premium subscription has been automatically renewed. Thank you for your continued support!</p>
		
		<h3>Renewal Details:</h3>
		<ul>
			<li><strong>Subscription ID:</strong> %s</li>
			<li><strong>Amount Charged:</strong> $%.2f %s</li>
			<li><strong>New Period:</strong> %s to %s</li>
			<li><strong>Transaction ID:</strong> %s</li>
		</ul>
		
		<p>Your premium access continues uninterrupted. Enjoy exclusive content and features!</p>
		<p>The Doujins Team</p>
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency,
		data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"), data.TransactionID)

	plainContent := fmt.Sprintf(`
		Subscription Renewed Successfully
		
		Hi %s,
		
		Your Doujins Premium subscription has been automatically renewed. Thank you for continued support!
		
		Renewal Details:
		- Subscription ID: %s
		- Amount Charged: $%.2f %s
		- New Period: %s to %s
		- Transaction ID: %s
		
		Your premium access continues uninterrupted.
		
		The Doujins Team
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency,
		data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"), data.TransactionID)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

// SendSubscriptionCancellation sends notification when subscription is cancelled
func (s *EmailService) SendSubscriptionCancellation(ctx context.Context, data SubscriptionEmailData) error {
	subject := "Your Doujins Premium subscription has been cancelled"

	htmlContent := fmt.Sprintf(`
		<h2>Subscription Cancelled</h2>
		<p>Hi %s,</p>
		<p>We're sorry to see you go! Your Doujins Premium subscription has been cancelled as requested.</p>
		
		<h3>Cancellation Details:</h3>
		<ul>
			<li><strong>Subscription ID:</strong> %s</li>
			<li><strong>Access Until:</strong> %s</li>
		</ul>
		
		<p>You'll continue to have premium access until %s. After that, your account will revert to the free tier.</p>
		<p>You can resubscribe at any time to regain premium access. We'd love to have you back!</p>
		<p>The Doujins Team</p>
	`, data.Username, data.SubscriptionID, data.PeriodEnd.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"))

	plainContent := fmt.Sprintf(`
		Subscription Cancelled
		
		Hi %s,
		
		Your Doujins Premium subscription has been cancelled as requested.
		
		Cancellation Details:
		- Subscription ID: %s
		- Access Until: %s
		
		You'll continue to have premium access until %s, then revert to the free tier.
		You can resubscribe at any time to regain premium access.
		
		The Doujins Team
	`, data.Username, data.SubscriptionID, data.PeriodEnd.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006"))

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

// SendPaymentFailed sends notification when subscription payment fails
func (s *EmailService) SendPaymentFailed(ctx context.Context, data SubscriptionEmailData) error {
	subject := "Action Required: Payment failed for your Doujins Premium subscription"

	htmlContent := fmt.Sprintf(`
		<h2>Payment Failed - Action Required</h2>
		<p>Hi %s,</p>
		<p>We were unable to process the payment for your Doujins Premium subscription. Your subscription is now past due.</p>
		
		<h3>Subscription Details:</h3>
		<ul>
			<li><strong>Subscription ID:</strong> %s</li>
			<li><strong>Amount Due:</strong> $%.2f %s</li>
			<li><strong>Payment Method:</strong> %s</li>
		</ul>
		
		<p><strong>What happens next:</strong></p>
		<ul>
			<li>Your premium access will continue for a grace period</li>
			<li>We'll attempt to retry payment automatically</li>
			<li>If payment continues to fail, your subscription will be cancelled</li>
		</ul>
		
		<p>To resolve this issue, please update your payment method in your account settings or contact support.</p>
		<p>The Doujins Team</p>
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency, data.PaymentMethod)

	plainContent := fmt.Sprintf(`
		Payment Failed - Action Required
		
		Hi %s,
		
		We were unable to process payment for your Doujins Premium subscription. Your subscription is now past due.
		
		Subscription Details:
		- Subscription ID: %s
		- Amount Due: $%.2f %s
		- Payment Method: %s
		
		What happens next:
		- Your premium access continues for a grace period
		- We'll retry payment automatically
		- If payment continues to fail, subscription will be cancelled
		
		Please update your payment method in account settings or contact support.
		
		The Doujins Team
	`, data.Username, data.SubscriptionID, data.Amount, data.Currency, data.PaymentMethod)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

// SendRoleExpiration sends notification when user role is expiring soon
func (s *EmailService) SendRoleExpiration(ctx context.Context, userEmail, username, roleName string, expiresAt time.Time) error {
	subject := fmt.Sprintf("Your %s access expires soon", roleName)

	daysUntilExpiry := int(time.Until(expiresAt).Hours() / 24)

	htmlContent := fmt.Sprintf(`
		<h2>Access Expiring Soon</h2>
		<p>Hi %s,</p>
		<p>This is a reminder that your <strong>%s</strong> access will expire in %d days on <strong>%s</strong>.</p>
		
		<p>To continue enjoying premium features, please renew your subscription before the expiration date.</p>
		<p>Thank you for being a valued member of Doujins!</p>
		<p>The Doujins Team</p>
	`, username, roleName, daysUntilExpiry, expiresAt.Format("January 2, 2006"))

	plainContent := fmt.Sprintf(`
		Access Expiring Soon
		
		Hi %s,
		
		Your %s access will expire in %d days on %s.
		
		To continue enjoying premium features, please renew your subscription before the expiration date.
		
		Thank you for being a valued member of Doujins!
		The Doujins Team
	`, username, roleName, daysUntilExpiry, expiresAt.Format("January 2, 2006"))

	return s.SendEmail(ctx, userEmail, subject, htmlContent, plainContent)
}
