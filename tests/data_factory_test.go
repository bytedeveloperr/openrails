package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUser represents a test user with different roles and permissions
type TestUser struct {
	ID       string
	Email    string
	Roles    []string
	IsAdmin  bool
	Metadata map[string]interface{}
}

// TestUserProfile defines different user profiles for testing
type TestUserProfile struct {
	EmailPrefix string
	Roles       []string
	IsAdmin     bool
	Metadata    map[string]interface{}
}

// Predefined test user profiles
var TestUserProfiles = map[string]TestUserProfile{
	"regular": {
		EmailPrefix: "regular-user",
		Roles:       []string{},
		IsAdmin:     false,
		Metadata:    map[string]interface{}{"type": "regular"},
	},
	"premium": {
		EmailPrefix: "premium-user",
		Roles:       []string{"premium"},
		IsAdmin:     false,
		Metadata:    map[string]interface{}{"type": "premium", "tier": "premium"},
	},
	"admin": {
		EmailPrefix: "admin-user",
		Roles:       []string{"admin"},
		IsAdmin:     true,
		Metadata:    map[string]interface{}{"type": "admin", "permissions": []string{"all"}},
	},
	"vip": {
		EmailPrefix: "vip-user",
		Roles:       []string{"premium", "vip"},
		IsAdmin:     false,
		Metadata:    map[string]interface{}{"type": "vip", "tier": "vip"},
	},
}

// SimpleTestDataFactory provides utilities for generating test data without database
type SimpleTestDataFactory struct {
	userCount int
}

// NewSimpleTestDataFactory creates a new simple test data factory
func NewSimpleTestDataFactory() *SimpleTestDataFactory {
	return &SimpleTestDataFactory{}
}

// CreateTestUser creates a test user with the specified profile
func (f *SimpleTestDataFactory) CreateTestUser(profile string) *TestUser {
	userProfile, exists := TestUserProfiles[profile]
	if !exists {
		return nil
	}

	f.userCount++
	userID := fmt.Sprintf("user-%d", f.userCount)
	email := fmt.Sprintf("%s-%d@example.com", userProfile.EmailPrefix, f.userCount)

	user := &TestUser{
		ID:       userID,
		Email:    email,
		Roles:    userProfile.Roles,
		IsAdmin:  userProfile.IsAdmin,
		Metadata: userProfile.Metadata,
	}

	return user
}

// TestDataFactory tests the test data factory functionality
func TestDataFactory(t *testing.T) {
	factory := NewSimpleTestDataFactory()

	t.Run("User Creation", func(t *testing.T) {
		user := factory.CreateTestUser("regular")
		require.NotNil(t, user, "Should create a test user")
		assert.NotEmpty(t, user.ID, "User should have an ID")
		assert.NotEmpty(t, user.Email, "User should have an email")
		assert.False(t, user.IsAdmin, "Regular user should not be admin")
		assert.Contains(t, user.Email, "regular-user", "Should have correct email prefix")
	})

	t.Run("Admin User Creation", func(t *testing.T) {
		adminUser := factory.CreateTestUser("admin")
		require.NotNil(t, adminUser, "Should create an admin user")
		assert.True(t, adminUser.IsAdmin, "Admin user should be admin")
		assert.Contains(t, adminUser.Roles, "admin", "Admin user should have admin role")
		assert.Contains(t, adminUser.Email, "admin-user", "Should have correct email prefix")
	})

	t.Run("Premium User Creation", func(t *testing.T) {
		premiumUser := factory.CreateTestUser("premium")
		require.NotNil(t, premiumUser, "Should create a premium user")
		assert.False(t, premiumUser.IsAdmin, "Premium user should not be admin")
		assert.Contains(t, premiumUser.Roles, "premium", "Premium user should have premium role")
		assert.Contains(t, premiumUser.Email, "premium-user", "Should have correct email prefix")
	})

	t.Run("VIP User Creation", func(t *testing.T) {
		vipUser := factory.CreateTestUser("vip")
		require.NotNil(t, vipUser, "Should create a VIP user")
		assert.False(t, vipUser.IsAdmin, "VIP user should not be admin")
		assert.Contains(t, vipUser.Roles, "premium", "VIP user should have premium role")
		assert.Contains(t, vipUser.Roles, "vip", "VIP user should have VIP role")
		assert.Contains(t, vipUser.Email, "vip-user", "Should have correct email prefix")
	})

	t.Run("Unknown Profile", func(t *testing.T) {
		user := factory.CreateTestUser("unknown")
		assert.Nil(t, user, "Should return nil for unknown profile")
	})

	t.Run("Multiple Users Have Different IDs", func(t *testing.T) {
		user1 := factory.CreateTestUser("regular")
		user2 := factory.CreateTestUser("regular")

		require.NotNil(t, user1, "Should create first user")
		require.NotNil(t, user2, "Should create second user")
		assert.NotEqual(t, user1.ID, user2.ID, "Users should have different IDs")
		assert.NotEqual(t, user1.Email, user2.Email, "Users should have different emails")
	})
}
