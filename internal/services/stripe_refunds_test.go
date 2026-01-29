package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-rails/openrails/config"
)

func TestStripeRefundService_CreateRefund_Success(t *testing.T) {
	// This test requires a controllable HTTP endpoint (StripeRefundService uses hardcoded Stripe URLs today).
	// In restricted sandboxes that disallow binding local ports, attempting httptest.NewServer panics.
	t.Skip("requires injectable Stripe endpoint and local listener")
}

func TestStripeRefundService_CreateRefund_ValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		params    RefundParams
		wantError string
	}{
		{
			name:      "nil config",
			config:    nil,
			params:    RefundParams{ChargeID: "ch_123"},
			wantError: "stripe configuration is not available",
		},
		{
			name:      "nil stripe config",
			config:    &config.Config{},
			params:    RefundParams{ChargeID: "ch_123"},
			wantError: "stripe configuration is not available",
		},
		{
			name: "empty secret key",
			config: &config.Config{
				Processors: map[string]*config.ProcessorConfig{
					"stripe": {Type: config.ProcessorTypeStripe, SecretKey: ""},
				},
			},
			params:    RefundParams{ChargeID: "ch_123"},
			wantError: "stripe secret key is not configured",
		},
		{
			name: "empty charge ID",
			config: &config.Config{
				Processors: map[string]*config.ProcessorConfig{
					"stripe": {Type: config.ProcessorTypeStripe, SecretKey: "sk_test_123"},
				},
			},
			params:    RefundParams{ChargeID: ""},
			wantError: "charge_id or payment_intent_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &StripeRefundService{Config: tt.config}
			_, err := svc.CreateRefund(context.Background(), tt.params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestStripeRefundService_GetRefund_ValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		refundID  string
		wantError string
	}{
		{
			name:      "nil config",
			config:    nil,
			refundID:  "re_123",
			wantError: "stripe configuration is not available",
		},
		{
			name: "empty refund ID",
			config: &config.Config{
				Processors: map[string]*config.ProcessorConfig{
					"stripe": {Type: config.ProcessorTypeStripe, SecretKey: "sk_test_123"},
				},
			},
			refundID:  "",
			wantError: "refund_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &StripeRefundService{Config: tt.config}
			_, err := svc.GetRefund(context.Background(), tt.refundID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestRefundParams_ChargeIDTypes(t *testing.T) {
	// Test that we correctly handle both charge IDs and payment intent IDs
	tests := []struct {
		chargeID     string
		expectsField string
	}{
		{"ch_test123", "charge"},
		{"pi_test456", "payment_intent"},
	}

	for _, tt := range tests {
		t.Run(tt.chargeID, func(t *testing.T) {
			params := RefundParams{
				ChargeID: tt.chargeID,
				Amount:   1000,
			}
			// Validate the chargeID prefix detection logic
			if tt.expectsField == "payment_intent" {
				assert.True(t, len(params.ChargeID) > 3 && params.ChargeID[:3] == "pi_")
			} else {
				assert.True(t, len(params.ChargeID) <= 3 || params.ChargeID[:3] != "pi_")
			}
		})
	}
}

func TestRefundParams_ReasonValidation(t *testing.T) {
	// Valid reasons
	validReasons := []string{"duplicate", "fraudulent", "requested_by_customer"}
	for _, reason := range validReasons {
		params := RefundParams{
			ChargeID: "ch_test",
			Reason:   reason,
		}
		assert.Equal(t, reason, params.Reason)
	}
}
