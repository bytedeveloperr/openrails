package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTestMode(t *testing.T) {
	t.Run("defaults to true when nil", func(t *testing.T) {
		cfg := &Config{TestMode: nil}
		assert.True(t, cfg.IsTestMode(), "should default to test mode when not explicitly set")
	})

	t.Run("returns true when explicitly true", func(t *testing.T) {
		trueBool := true
		cfg := &Config{TestMode: &trueBool}
		assert.True(t, cfg.IsTestMode())
	})

	t.Run("returns false when explicitly false", func(t *testing.T) {
		falseBool := false
		cfg := &Config{TestMode: &falseBool}
		assert.False(t, cfg.IsTestMode())
	})
}

func TestIsDev(t *testing.T) {
	t.Run("returns true for empty env", func(t *testing.T) {
		cfg := &Config{Env: ""}
		assert.True(t, cfg.IsDev())
	})

	t.Run("returns true for dev", func(t *testing.T) {
		cfg := &Config{Env: "dev"}
		assert.True(t, cfg.IsDev())
	})

	t.Run("returns true for development", func(t *testing.T) {
		cfg := &Config{Env: "development"}
		assert.True(t, cfg.IsDev())
	})

	t.Run("returns false for prod", func(t *testing.T) {
		cfg := &Config{Env: "prod"}
		assert.False(t, cfg.IsDev())
	})

	t.Run("returns false for production", func(t *testing.T) {
		cfg := &Config{Env: "production"}
		assert.False(t, cfg.IsDev())
	})

	t.Run("is case sensitive (expects lowercase)", func(t *testing.T) {
		// IsDev expects lowercase env values
		cfg := &Config{Env: "DEV"}
		assert.False(t, cfg.IsDev(), "uppercase DEV is not recognized as dev")
	})
}

func TestTestModeOrthogonality(t *testing.T) {
	// Test that env and test_mode are independent settings
	t.Run("prod env can have test_mode true", func(t *testing.T) {
		trueBool := true
		cfg := &Config{
			Env:      "prod",
			TestMode: &trueBool,
		}
		assert.False(t, cfg.IsDev(), "env=prod should not be dev")
		assert.True(t, cfg.IsTestMode(), "test_mode=true should be in test mode")
	})

	t.Run("dev env can have test_mode false", func(t *testing.T) {
		falseBool := false
		cfg := &Config{
			Env:      "dev",
			TestMode: &falseBool,
		}
		assert.True(t, cfg.IsDev(), "env=dev should be dev")
		assert.False(t, cfg.IsTestMode(), "test_mode=false should not be in test mode")
	})
}

func TestStripeKeyModeCompatibility(t *testing.T) {
	// These tests verify the documented Stripe key validation behavior
	// Note: Actual validation happens in Load(), these test the rules

	t.Run("test key sk_test_ prefix identified correctly", func(t *testing.T) {
		key := "sk_test_abc123"
		isTestKey := len(key) >= 8 && key[:8] == "sk_test_"
		assert.True(t, isTestKey)
	})

	t.Run("live key sk_live_ prefix identified correctly", func(t *testing.T) {
		key := "sk_live_abc123"
		isLiveKey := len(key) >= 8 && key[:8] == "sk_live_"
		assert.True(t, isLiveKey)
	})
}
