package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	email "github.com/doujins-org/doujins-email"
	log "github.com/sirupsen/logrus"
)

type EmailService struct {
	svc *email.Service
}

// OneOffPurchaseEmailData contains data for one-off purchase receipts
type OneOffPurchaseEmailData struct {
	UserEmail     string
	Amount        float64
	Currency      string
	ProductName   string
	PaymentMethod string
	IsPremium     bool
}

// NewEmailService wires the shared email package into the billing domain service.
func NewEmailService(cfg *email.Config) (*EmailService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("email configuration not provided")
	}

	svc, err := email.NewService(*cfg, email.WithLogger(logrusAdapter{}))
	if err != nil {
		return nil, err
	}

	return &EmailService{svc: svc}, nil
}

// IsEnabled returns true when delivery is possible.
func (s *EmailService) IsEnabled() bool {
	return s != nil && s.svc != nil && s.svc.IsEnabled()
}

// SendEmail sends a basic email using the configured provider.
func (s *EmailService) SendEmail(ctx context.Context, to, subject, htmlContent, plainContent string) error {
	if !s.IsEnabled() {
		log.WithContext(ctx).WithFields(log.Fields{
			"to":      to,
			"subject": subject,
		}).Debug("email service disabled - skipping send")
		return nil
	}

	msg := email.Message{
		To:       []email.Recipient{{Address: to}},
		Subject:  subject,
		HTMLBody: htmlContent,
		TextBody: plainContent,
	}

	return s.send(ctx, msg, to)
}

// SendTemplatedEmail sends a template-based email using the configured provider.
func (s *EmailService) SendTemplatedEmail(ctx context.Context, to, templateID string, templateData map[string]any) error {
	if !s.IsEnabled() {
		log.WithContext(ctx).WithFields(log.Fields{
			"to":          to,
			"template_id": templateID,
		}).Debug("email service disabled - skipping templated send")
		return nil
	}

	msg := email.Message{
		To:           []email.Recipient{{Address: to}},
		TemplateID:   templateID,
		TemplateData: templateData,
	}

	return s.send(ctx, msg, to)
}

// SendOneOffPurchaseReceipt sends a receipt for a one-off purchase (e.g., Solana payment).
func (s *EmailService) SendOneOffPurchaseReceipt(ctx context.Context, data OneOffPurchaseEmailData) error {
	if !s.IsEnabled() {
		log.WithContext(ctx).WithField("user_email", data.UserEmail).Debug("email service disabled - skipping one-off receipt send")
		return nil
	}

	productName := data.ProductName
	if productName == "" {
		productName = "Doujins premium content"
	}

	amountLine := fmt.Sprintf("%.2f %s", data.Amount, data.Currency)
	if data.Currency == "USD" {
		amountLine = fmt.Sprintf("$%.2f %s", data.Amount, data.Currency)
	}

	issuedAt := time.Now().Format("Jan 2, 2006 15:04 MST")

	paymentMethod := strings.ToLower(data.PaymentMethod)
	isSolana := paymentMethod == "solana"

	subject := "Thanks for supporting Doujins!"
	if isSolana {
		subject = "Your Solana premium purchase is confirmed"
	}

	messageIntro := "Thanks for completing your purchase!"
	if data.IsPremium {
		messageIntro = "Thanks for unlocking Doujins Premium!"
	}

	if isSolana {
		htmlContent := fmt.Sprintf(`
			<h2>Solana Payment Received</h2>
			<p>Hi there,</p>
			<p>%s This one-time Solana transaction instantly extended your premium access.</p>
			<ul>
				<li><strong>Product:</strong> %s</li>
				<li><strong>Amount:</strong> %s</li>
				<li><strong>Date:</strong> %s</li>
			</ul>
			<p>Enjoy your premium benefits—no rebill will occur automatically.</p>
			<p>The Doujins Team</p>
		`, messageIntro, productName, amountLine, issuedAt)

		plainContent := fmt.Sprintf(`
		Solana Payment Received
		
		%s This one-time Solana transaction instantly extended your premium access.
		Product: %s
		Amount: %s
		Date: %s
		
		Enjoy your premium benefits—there won't be an automatic rebill.
		The Doujins Team
		`, messageIntro, productName, amountLine, issuedAt)

		return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
	}

	htmlContent := fmt.Sprintf(`
		<h2>Payment Received</h2>
		<p>Hi there,</p>
		<p>%s</p>
		<ul>
			<li><strong>Product:</strong> %s</li>
			<li><strong>Amount:</strong> %s</li>
			<li><strong>Date:</strong> %s</li>
		</ul>
		<p>Your access has been updated instantly. Enjoy!</p>
		<p>The Doujins Team</p>
	`, messageIntro, productName, amountLine, issuedAt)

	plainContent := fmt.Sprintf(`
		Payment Received
		
		%s
		Product: %s
		Amount: %s
		Date: %s
		
		Your access has been updated instantly. Enjoy!
		The Doujins Team
	`, messageIntro, productName, amountLine, issuedAt)

	return s.SendEmail(ctx, data.UserEmail, subject, htmlContent, plainContent)
}

func (s *EmailService) send(ctx context.Context, msg email.Message, to string) error {
	res, err := s.svc.Send(ctx, msg)
	if err != nil {
		return fmt.Errorf("email send failed: %w", err)
	}

	if res != nil {
		log.WithContext(ctx).WithFields(log.Fields{
			"to":        to,
			"provider":  res.Provider,
			"status":    res.StatusCode,
			"messageID": res.MessageID,
		}).Debug("email sent successfully")
	}

	return nil
}

type logrusAdapter struct{}

func (logrusAdapter) Debugf(format string, args ...any) { log.Debugf(format, args...) }
func (logrusAdapter) Infof(format string, args ...any)  { log.Infof(format, args...) }
func (logrusAdapter) Warnf(format string, args ...any)  { log.Warnf(format, args...) }
func (logrusAdapter) Errorf(format string, args ...any) { log.Errorf(format, args...) }
