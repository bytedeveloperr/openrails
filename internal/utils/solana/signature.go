package solana

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrInvalidSignature = errors.New("invalid Solana signature")
	ErrInvalidAddress   = errors.New("invalid Solana address")
)

// Base58 regex patterns for Solana
var (
	// Solana addresses are 32-44 characters in base58
	addressRegex = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
	// Solana signatures are typically 87-88 characters in base58
	signatureRegex = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{87,88}$`)
)

// ValidateAddress checks if a string is a valid Solana wallet address
func ValidateAddress(address string) error {
	if address == "" {
		return fmt.Errorf("%w: address cannot be empty", ErrInvalidAddress)
	}
	if !addressRegex.MatchString(address) {
		return fmt.Errorf("%w: address format invalid", ErrInvalidAddress)
	}
	return nil
}

// ValidateSignature checks if a string is a valid Solana transaction signature format
func ValidateSignature(signature string) error {
	if signature == "" {
		return fmt.Errorf("%w: signature cannot be empty", ErrInvalidSignature)
	}
	if !signatureRegex.MatchString(signature) {
		return fmt.Errorf("%w: signature format invalid", ErrInvalidSignature)
	}
	return nil
}

// IsValidAddress returns true if the address format is valid
func IsValidAddress(address string) bool {
	return ValidateAddress(address) == nil
}

// IsValidSignature returns true if the signature format is valid
func IsValidSignature(signature string) bool {
	return ValidateSignature(signature) == nil
}