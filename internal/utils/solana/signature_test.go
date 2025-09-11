package solana

import (
	"testing"
)

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		expectErr bool
	}{
		{
			name:      "valid address",
			address:   "11111111111111111111111111111112",
			expectErr: false,
		},
		{
			name:      "valid longer address",
			address:   "So11111111111111111111111111111111111111112",
			expectErr: false,
		},
		{
			name:      "empty address",
			address:   "",
			expectErr: true,
		},
		{
			name:      "too short address",
			address:   "111111111111111111111111111111",
			expectErr: true,
		},
		{
			name:      "too long address",
			address:   "111111111111111111111111111111111111111111111",
			expectErr: true,
		},
		{
			name:      "invalid characters",
			address:   "1111111111111111111111111111111O", // O is not in base58
			expectErr: true,
		},
		{
			name:      "invalid character 0",
			address:   "1111111111111111111111111111111110", // 0 is not in base58
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAddress(tt.address)
			if tt.expectErr && err == nil {
				t.Errorf("ValidateAddress() expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("ValidateAddress() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateSignature(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		expectErr bool
	}{
		{
			name:      "valid signature 87 chars",
			signature: "3Mw3Z9q8q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q",
			expectErr: false,
		},
		{
			name:      "valid signature 88 chars",
			signature: "3Mw3Z9q8q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1",
			expectErr: false,
		},
		{
			name:      "empty signature",
			signature: "",
			expectErr: true,
		},
		{
			name:      "too short signature",
			signature: "3Mw3Z9q8q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q",
			expectErr: true,
		},
		{
			name:      "too long signature",
			signature: "3Mw3Z9q8q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q",
			expectErr: true,
		},
		{
			name:      "invalid character 0",
			signature: "3Mw3Z9q8q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q10",
			expectErr: true,
		},
		{
			name:      "invalid character O",
			signature: "3Mw3Z9q8q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1O",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSignature(tt.signature)
			if tt.expectErr && err == nil {
				t.Errorf("ValidateSignature() expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("ValidateSignature() unexpected error: %v", err)
			}
		})
	}
}

func TestIsValidAddress(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		expected bool
	}{
		{
			name:     "valid address",
			address:  "11111111111111111111111111111112",
			expected: true,
		},
		{
			name:     "invalid address",
			address:  "invalid",
			expected: false,
		},
		{
			name:     "empty address",
			address:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidAddress(tt.address)
			if result != tt.expected {
				t.Errorf("IsValidAddress() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestIsValidSignature(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		expected  bool
	}{
		{
			name:      "valid signature",
			signature: "3Mw3Z9q8q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q1q",
			expected:  true,
		},
		{
			name:      "invalid signature",
			signature: "invalid",
			expected:  false,
		},
		{
			name:      "empty signature",
			signature: "",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidSignature(tt.signature)
			if result != tt.expected {
				t.Errorf("IsValidSignature() = %v, expected %v", result, tt.expected)
			}
		})
	}
}