package server

import (
	"context"
	"fmt"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/doujins-org/doujins-billing/internal/auth"
)

func (s *Server) initAdminAuthKit() error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}
	if s.cfg == nil || s.cfg.Auth == nil {
		return fmt.Errorf("auth config is required for admin routes")
	}
	if s.cfg.DB == nil {
		return fmt.Errorf("database config is required for admin routes")
	}

	accept, err := auth.BuildAcceptConfig(s.cfg.Auth)
	if err != nil {
		return fmt.Errorf("build auth accept config: %w", err)
	}
	s.adminAuth = authgin.MiddlewareFromConfig(accept)

	dbURL := s.cfg.DB.GetConnectionString()
	if dbURL == "" {
		return fmt.Errorf("database connection string is required for admin routes")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return fmt.Errorf("build admin auth pool: %w", err)
	}
	s.adminAuthPool = pool
	return nil
}
