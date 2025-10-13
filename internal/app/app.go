package app

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/auth"
	"github.com/doujins-org/doujins-billing/internal/state"
	"github.com/doujins-org/doujins-billing/pkg/cache"
)

// App encapsulates the long-lived dependencies shared across transports.
type App struct {
	Config       *config.Config
	State        *state.State
	Cache        cache.Cache
	RedisClient  *redis.Client
	AuthVerifier auth.Verifier

	stopRedisMonitor context.CancelFunc
}

// Bootstrap initialises core services, caches, and auth verifier.
func Bootstrap(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	verifier, err := auth.NewVerifier(cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("build auth verifier: %w", err)
	}

	st, err := state.NewState(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialise state: %w", err)
	}

	memoryCache := cache.NewMemoryCache()
	switchable := cache.NewSwitchableCache(memoryCache)

	var stop context.CancelFunc
	if st.RedisClient != nil {
		stop = monitorRedis(st.RedisClient, switchable, memoryCache)
	} else {
		log.Warn("redis not configured; cache operating in-memory only")
	}

	return &App{
		Config:           cfg,
		State:            st,
		Cache:            switchable,
		RedisClient:      st.RedisClient,
		AuthVerifier:     verifier,
		stopRedisMonitor: stop,
	}, nil
}

// Close releases all resources owned by the application.
func (a *App) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if a.stopRedisMonitor != nil {
		a.stopRedisMonitor()
	}
	var errs []error
	if a.Cache != nil {
		if err := a.Cache.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close cache: %w", err))
		}
	}
	if a.State != nil {
		if err := a.State.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("shutdown errors: %v", errs)
}

func monitorRedis(client *redis.Client, switchable *cache.SwitchableCache, fallback cache.Cache) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	redisCache := cache.NewRedisCache(client)

	// Initial probe
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	usingRedis := false
	if _, err := client.Ping(probeCtx).Result(); err == nil {
		switchable.SetBackend(redisCache)
		log.Info("redis available: using redis-backed cache")
		usingRedis = true
	} else {
		log.WithError(err).Warn("redis unavailable at startup; using in-memory cache")
	}
	probeCancel()

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, err := client.Ping(pingCtx).Result()
				pingCancel()
				if err == nil {
					if !usingRedis {
						switchable.SetBackend(redisCache)
						usingRedis = true
						log.Info("redis became available; switched cache backend")
					}
					continue
				}
				if usingRedis {
					switchable.SetBackend(fallback)
					usingRedis = false
					log.WithError(err).Warn("redis lost; reverting cache to memory")
				}
			}
		}
	}()

	return cancel
}
