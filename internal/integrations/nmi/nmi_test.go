package nmi

import (
	"testing"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_EndpointSelection(t *testing.T) {
	baseCfg := &config.NMIProviderSettings{
		SecurityKey: "test-security-key",
	}

	t.Run("test mode uses sandbox endpoints", func(t *testing.T) {
		client, err := NewClient("mobius", baseCfg, true)
		require.NoError(t, err)
		assert.Equal(t, SandboxDirectPostURL, client.DirectPostURL, "should use sandbox direct post URL")
		assert.Equal(t, SandboxQueryAPIURL, client.QueryURL, "should use sandbox query URL")
		assert.True(t, client.TestMode)
	})

	t.Run("production mode uses default endpoints", func(t *testing.T) {
		client, err := NewClient("mobius", baseCfg, false)
		require.NoError(t, err)
		assert.Equal(t, DefaultDirectPostURL, client.DirectPostURL, "should use production direct post URL")
		assert.Equal(t, DefaultQueryAPIURL, client.QueryURL, "should use production query URL")
		assert.False(t, client.TestMode)
	})

	t.Run("test mode uses custom URLs if provided", func(t *testing.T) {
		customCfg := &config.NMIProviderSettings{
			SecurityKey:   "test-security-key",
			DirectPostURL: "https://custom.example.com/transact",
			QueryURL:      "https://custom.example.com/query",
		}
		client, err := NewClient("mobius", customCfg, true)
		require.NoError(t, err)
		assert.Equal(t, "https://custom.example.com/transact", client.DirectPostURL)
		assert.Equal(t, "https://custom.example.com/query", client.QueryURL)
	})

	t.Run("production mode uses custom URLs if provided", func(t *testing.T) {
		customCfg := &config.NMIProviderSettings{
			SecurityKey:   "test-security-key",
			DirectPostURL: "https://custom.example.com/transact",
			QueryURL:      "https://custom.example.com/query",
		}
		client, err := NewClient("mobius", customCfg, false)
		require.NoError(t, err)
		assert.Equal(t, "https://custom.example.com/transact", client.DirectPostURL)
		assert.Equal(t, "https://custom.example.com/query", client.QueryURL)
	})

	t.Run("production mode requires security key", func(t *testing.T) {
		emptyCfg := &config.NMIProviderSettings{}
		_, err := NewClient("mobius", emptyCfg, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "security key is required")
	})

	t.Run("test mode allows missing security key", func(t *testing.T) {
		emptyCfg := &config.NMIProviderSettings{}
		client, err := NewClient("mobius", emptyCfg, true)
		require.NoError(t, err)
		// Client created but SecurityKey is empty (API calls will fail but that's expected)
		assert.Empty(t, client.SecurityKey)
	})
}

func TestSandboxEndpointConstants(t *testing.T) {
	// Verify the sandbox endpoint constants are correctly defined
	assert.Contains(t, SandboxDirectPostURL, "sandbox.nmi.com")
	assert.Contains(t, SandboxQueryAPIURL, "sandbox.nmi.com")

	// Verify production endpoints don't contain sandbox
	assert.NotContains(t, DefaultDirectPostURL, "sandbox")
	assert.NotContains(t, DefaultQueryAPIURL, "sandbox")
}
