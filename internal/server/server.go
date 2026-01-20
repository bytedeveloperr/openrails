package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/app"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/pkg/authprovider"
	"github.com/doujins-org/doujins-billing/pkg/cache"
)

type Dependencies struct {
	Config       *config.Config
	Cache        cache.Cache
	Runtime      *app.Runtime
	Redis        *redis.Client
	AuthProvider authprovider.Provider
}

type Server struct {
	cfg          *config.Config
	cache        cache.Cache
	runtime      *app.Runtime
	rdb          *redis.Client
	authProvider authprovider.Provider

	// publicHandler is the standalone "full surface" HTTP handler.
	// It includes user + admin + webhook routes.
	publicHandler *gin.Engine

	// The split handlers below are primarily intended for embedded hosts that want to mount only
	// the minimal required surface area.
	userHandler    *gin.Engine // user/public billing APIs (and health)
	adminHandler   *gin.Engine // admin-only APIs (JWT + admin role required)
	webhookHandler *gin.Engine // processor webhooks (e.g. Stripe)

	privateHandler *gin.Engine // service API (X-API-KEY auth)
}

func New(deps Dependencies) (*Server, error) {
	if deps.Config == nil {
		return nil, fmt.Errorf("server config is required")
	}
	if deps.Runtime == nil {
		return nil, fmt.Errorf("server runtime is required")
	}
	if deps.Cache == nil {
		return nil, fmt.Errorf("server cache is required")
	}
	if deps.AuthProvider == nil {
		return nil, fmt.Errorf("auth provider is required")
	}

	s := &Server{
		cfg:          deps.Config,
		cache:        deps.Cache,
		runtime:      deps.Runtime,
		rdb:          deps.Redis,
		authProvider: deps.AuthProvider,
	}

	s.setupHandlers()
	s.registerPublicRoutes()
	s.registerAdminRoutes()
	s.registerServiceRoutes()

	log.Info("Billing service initialized successfully")
	return s, nil
}

func (s *Server) setupHandlers() {
	newPublicEngine := func() *gin.Engine {
		e := gin.New()
		e.Use(gin.Recovery())
		e.Use(gin.LoggerWithConfig(gin.LoggerConfig{
			SkipPaths: []string{"/health/live", "/health/ready", "/healthz", "/readyz", "/health"},
		}))
		e.Use(middleware.CORS(s.cfg.CorsOrigins))
		e.Use(middleware.RateLimit(s.cfg.RateLimits, s.rdb))
		return e
	}

	// Standalone handler (full surface).
	s.publicHandler = newPublicEngine()

	// Embedded-friendly split handlers.
	s.userHandler = newPublicEngine()
	s.adminHandler = newPublicEngine()
	s.webhookHandler = newPublicEngine()

	// Private handler for service-to-service API (X-API-KEY auth)
	// This runs on a separate port and should only be accessible within the Docker network
	s.privateHandler = gin.New()
	s.privateHandler.Use(gin.Recovery())
	s.privateHandler.Use(gin.Logger())
	// No CORS needed for internal service-to-service calls
	// Rate limiting could be added if needed
}

func (s *Server) wrap(fn func(r *handlers.Request)) func(c *gin.Context) {
	return func(c *gin.Context) {
		fn(handlers.NewRequest(c, s.runtime))
	}
}

func (s *Server) Handler() http.Handler        { return s.publicHandler }
func (s *Server) PrivateHandler() http.Handler { return s.privateHandler }

func (s *Server) UserHandler() http.Handler    { return s.userHandler }
func (s *Server) AdminHandler() http.Handler   { return s.adminHandler }
func (s *Server) WebhookHandler() http.Handler { return s.webhookHandler }

// ServiceHandler returns the internal service-to-service HTTP API (X-API-KEY protected).
// Embedded hosts should typically NOT mount this; use the in-process Go API (`pkg/service`) instead.
func (s *Server) ServiceHandler() http.Handler { return s.privateHandler }

// Close currently does not own underlying resources; callers should close the App.
func (s *Server) Close(_ context.Context) error {
	log.Info("Billing HTTP server shut down")
	return nil
}

func (s *Server) Cfg() *config.Config {
	return s.cfg
}
