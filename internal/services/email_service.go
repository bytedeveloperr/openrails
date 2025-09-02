package services

import (
	"context"
	"fmt"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
)

type EmailService struct {
	config   *config.SendGridConfig
	client   *sendgrid.Client
	fromMail *mail.Email
}

func NewEmailService(cfg *config.SendGridConfig) (*EmailService, error) {
	if cfg == nil || cfg.APIKey == "" {
		return nil, fmt.Errorf("SendGrid API key not configured - email service unavailable")
	}

	client := sendgrid.NewSendClient(cfg.APIKey)
	fromMail := mail.NewEmail(cfg.FromName, cfg.FromEmail)

	return &EmailService{
		config:   cfg,
		client:   client,
		fromMail: fromMail,
	}, nil
}

// IsEnabled returns true if the email service is properly configured
func (s *EmailService) IsEnabled() bool {
	return s.config != nil && s.config.APIKey != "" && s.client != nil
}

// SendEmail sends a basic email using SendGrid
func (s *EmailService) SendEmail(ctx context.Context, to, subject, htmlContent, plainContent string) error {
	if !s.IsEnabled() {
		log.Printf("Email service disabled - would send email to %s: %s", to, subject)
		return nil
	}

	toMail := mail.NewEmail("", to)
	message := mail.NewSingleEmail(s.fromMail, subject, toMail, plainContent, htmlContent)

	response, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send email via SendGrid: %w", err)
	}

	if response.StatusCode >= 400 {
		return fmt.Errorf("SendGrid API error: status %d, body: %s", response.StatusCode, response.Body)
	}

	log.Printf("Email sent successfully to %s (status: %d)", to, response.StatusCode)
	return nil
}

// SendTemplatedEmail sends an email using a SendGrid template
func (s *EmailService) SendTemplatedEmail(ctx context.Context, to, templateID string, templateData map[string]any) error {
	if !s.IsEnabled() {
		log.Printf("Email service disabled - would send templated email to %s with template %s", to, templateID)
		return nil
	}

	toMail := mail.NewEmail("", to)
	message := mail.NewV3Mail()
	message.SetFrom(s.fromMail)
	message.SetTemplateID(templateID)

	personalization := mail.NewPersonalization()
	personalization.AddTos(toMail)

	// Add template data as dynamic template data
	for key, value := range templateData {
		personalization.SetDynamicTemplateData(key, value)
	}

	message.AddPersonalizations(personalization)

	response, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send templated email via SendGrid: %w", err)
	}

	if response.StatusCode >= 400 {
		return fmt.Errorf("SendGrid API error: status %d, body: %s", response.StatusCode, response.Body)
	}

	log.Printf("Templated email sent successfully to %s (status: %d)", to, response.StatusCode)
	return nil
}
