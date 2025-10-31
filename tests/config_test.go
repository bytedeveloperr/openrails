package tests

import (
	"os"
	"testing"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig provides configuration management for tests
type TestConfig struct {
	*config.Config
	TestTimeout       time.Duration
	EnableTestLogs    bool
	EnableMockServers bool
	DatabaseCleanup   bool
	WebhookTimeout    time.Duration
}

// NewTestConfig creates a new test configuration
func NewTestConfig() *TestConfig {
	baseConfig := &config.Config{
		Env:  "test",
		Host: "localhost",
		Port: 8080,
		DB: &config.DBConfig{
			URL:     "postgres://test:test@localhost:5432/test?sslmode=disable",
			Schema:  "billing",
			Dialect: "postgres",
		},
		Redis: &config.RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
		Auth: &config.AuthConfig{
			Issuer:   "doujins-test",
			Audience: "billing-app",
			BaseURL:  "doujins-test",
		},
	}

	return &TestConfig{
		Config:            baseConfig,
		TestTimeout:       5 * time.Minute,
		EnableTestLogs:    false,
		EnableMockServers: true,
		DatabaseCleanup:   true,
		WebhookTimeout:    30 * time.Second,
	}
}

// Clone creates a copy of the test configuration
func (tc *TestConfig) Clone() *TestConfig {
	clonedConfig := *tc.Config

	if tc.Config.DB != nil {
		dbConfig := *tc.Config.DB
		clonedConfig.DB = &dbConfig
	}

	if tc.Config.Redis != nil {
		redisConfig := *tc.Config.Redis
		clonedConfig.Redis = &redisConfig
	}

	if tc.Config.Auth != nil {
		authConfig := *tc.Config.Auth
		clonedConfig.Auth = &authConfig
	}

	return &TestConfig{
		Config:            &clonedConfig,
		TestTimeout:       tc.TestTimeout,
		EnableTestLogs:    tc.EnableTestLogs,
		EnableMockServers: tc.EnableMockServers,
		DatabaseCleanup:   tc.DatabaseCleanup,
		WebhookTimeout:    tc.WebhookTimeout,
	}
}

// TestEnvironment represents different test environments
type TestEnvironment string

const (
	TestEnvUnit        TestEnvironment = "unit"
	TestEnvIntegration TestEnvironment = "integration"
	TestEnvE2E         TestEnvironment = "e2e"
	TestEnvPerformance TestEnvironment = "performance"
)

// GetTestEnvironment returns the current test environment
func GetTestEnvironment() TestEnvironment {
	env := os.Getenv("TEST_ENVIRONMENT")
	if env == "" {
		env = string(TestEnvIntegration)
	}
	return TestEnvironment(env)
}

// IsIntegrationTest returns true if running integration tests
func IsIntegrationTest() bool {
	return GetTestEnvironment() == TestEnvIntegration
}

// TestBasicConfig tests the test configuration utilities
func TestBasicConfig(t *testing.T) {
	t.Run("Create Test Config", func(t *testing.T) {
		config := NewTestConfig()
		require.NotNil(t, config, "Should create test config")
		assert.Equal(t, "test", config.Env, "Should be test environment")
		assert.NotEmpty(t, config.Auth.Issuer, "Should have auth issuer")
		assert.Equal(t, "billing", config.DB.Schema, "Should have correct schema")
	})

	t.Run("Config Cloning", func(t *testing.T) {
		originalConfig := NewTestConfig()
		clonedConfig := originalConfig.Clone()

		assert.NotSame(t, originalConfig, clonedConfig, "Cloned config should be different instance")
		assert.Equal(t, originalConfig.Env, clonedConfig.Env, "Cloned config should have same values")

		// Modify clone to ensure independence
		clonedConfig.Env = "modified"
		assert.NotEqual(t, originalConfig.Env, clonedConfig.Env, "Configs should be independent")
	})

	t.Run("Environment Detection", func(t *testing.T) {
		env := GetTestEnvironment()
		assert.NotEmpty(t, env, "Should detect test environment")

		isIntegration := IsIntegrationTest()
		assert.True(t, isIntegration, "Should detect integration test")
	})
}
