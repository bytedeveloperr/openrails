package auth

import (
	"context"
	"github.com/google/uuid"
)

type TestUserOptions struct {
	Email    string
	Roles    []string
	Metadata map[string]interface{}
}

type TestUser struct {
	ID    string
	Email string
}

type CreateResult struct {
	User TestUser
}

type UserManager struct{}

func GetUserManager(_ interface{}, _ interface{}) *UserManager { return &UserManager{} }

func (m *UserManager) CreateTestUsers(_ context.Context, opts []TestUserOptions) ([]CreateResult, error) {
	out := make([]CreateResult, len(opts))
	for i, o := range opts {
		out[i] = CreateResult{User: TestUser{ID: uuid.New().String(), Email: o.Email}}
	}
	return out, nil
}
