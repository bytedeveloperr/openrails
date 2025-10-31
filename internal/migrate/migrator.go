package migrate

import (
	"context"
	"fmt"
	"time"

	authkitmigrations "github.com/doujins-org/authkit/migrations/postgres"
	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	postgresmigrations "github.com/doujins-org/doujins-billing/migrations/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

// RunAuthKit applies only AuthKit migrations to the profiles schema.
// Run this once before running Run() on all services.
func RunAuthKit(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.DB == nil {
		return fmt.Errorf("missing database config")
	}

	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = database.Close() }()

	bunDB, ok := database.GetDB().(*bun.DB)
	if !ok {
		return fmt.Errorf("unexpected db type for bun migrator")
	}

	// AuthKit migration tracking lives in the profiles schema itself
	authkitOpts := []migrate.MigratorOption{
		migrate.WithTableName("profiles.bun_migrations"),
		migrate.WithLocksTableName("profiles.bun_migration_locks"),
		migrate.WithMarkAppliedOnSuccess(true),
	}

	log.Info("Running Authkit migrations (profiles schema)...")
	if err := runAuthkitMigrations(ctx, bunDB, authkitOpts); err != nil {
		return fmt.Errorf("authkit migrations failed: %w", err)
	}

	log.Info("✓ AuthKit migrations completed successfully")
	return nil
}

// Run applies service-specific migrations in the correct order:
// 1. River (billing schema) - via rivermigrate
// 2. Billing (billing schema) - via Bun
// 3. ClickHouse - via per-file tracking
//
// NOTE: AuthKit migrations must be run first using RunAuthKit() or 'migrate authkit' command.
// Billing migrations use their own tracking tables in billing schema.
// River uses its own migration table (billing.river_migration).
func Run(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.DB == nil {
		return fmt.Errorf("missing database config")
	}

	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = database.Close() }()

	bunDB, ok := database.GetDB().(*bun.DB)
	if !ok {
		return fmt.Errorf("unexpected db type for bun migrator")
	}

	schema := cfg.DB.Schema
	if schema == "" {
		schema = "billing"
	}

	// Billing migration tracking in billing schema
	billingOpts := []migrate.MigratorOption{
		migrate.WithTableName(schema + ".bun_migrations"),
		migrate.WithLocksTableName(schema + ".bun_migration_locks"),
		migrate.WithMarkAppliedOnSuccess(true),
	}

	// ---------- 1. River Migrations (billing schema) ----------
	log.Info("Running River migrations (billing schema)...")
	if err := runRiverMigrations(ctx, cfg, schema); err != nil {
		return fmt.Errorf("river migrations failed: %w", err)
	}

	// ---------- 2. Billing Migrations (billing schema) ----------
	log.Info("Running Billing migrations (billing schema)...")
	if err := runBillingMigrations(ctx, bunDB, billingOpts); err != nil {
		return fmt.Errorf("billing migrations failed: %w", err)
	}

	// ---------- 3. ClickHouse Migrations ----------
	log.Info("Running ClickHouse migrations...")
	if err := applyClickHouseMigrations(ctx, cfg); err != nil {
		return fmt.Errorf("clickhouse migrations failed: %w", err)
	}

	log.Info("All migrations applied successfully (River + Billing + ClickHouse)")
	log.Info("Database migrations complete")
	return nil
}

// runAuthkitMigrations runs Authkit's built-in migrations to the profiles schema.
// Uses Bun migration tracking tables in profiles schema to avoid race conditions.
func runAuthkitMigrations(ctx context.Context, bunDB *bun.DB, bunOpts []migrate.MigratorOption) error {
	m := migrate.NewMigrator(bunDB, authkitmigrations.Migrations, bunOpts...)
	if err := m.Init(ctx); err != nil {
		log.WithError(err).Warn("authkit migrations: init returned error; continuing")
	}
	if err := acquireLockWithWait(ctx, m, "authkit"); err != nil {
		return fmt.Errorf("authkit migrations: lock wait: %w", err)
	}
	defer func() {
		if unlockErr := m.Unlock(ctx); unlockErr != nil {
			log.WithError(unlockErr).Warn("authkit migrations: unlock failed")
		}
	}()
	group, err := m.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("authkit migrations: apply: %w", err)
	}
	if group.ID == 0 {
		log.Info("No new Authkit migrations to apply")
	} else {
		log.WithFields(log.Fields{
			"group_id": group.ID,
			"count":    len(group.Migrations),
		}).Info("Applied Authkit migrations")
	}
	return nil
}

// runBillingMigrations runs the billing service's own migrations.
// Uses shared Bun migration tracking tables in billing schema.
func runBillingMigrations(ctx context.Context, bunDB *bun.DB, bunOpts []migrate.MigratorOption) error {
	m := migrate.NewMigrator(bunDB, postgresmigrations.Migrations, bunOpts...)
	if err := m.Init(ctx); err != nil {
		log.WithError(err).Warn("billing migrations: init returned error; continuing")
	}
	if err := acquireLockWithWait(ctx, m, "billing"); err != nil {
		return fmt.Errorf("billing migrations: lock wait: %w", err)
	}
	defer func() {
		if unlockErr := m.Unlock(ctx); unlockErr != nil {
			log.WithError(unlockErr).Warn("billing migrations: unlock failed")
		}
	}()
	group, err := m.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("billing migrations: apply: %w", err)
	}
	if group.ID == 0 {
		log.Info("No new Billing migrations to apply")
	} else {
		log.WithFields(log.Fields{
			"group_id": group.ID,
			"count":    len(group.Migrations),
		}).Info("Applied Billing migrations")
	}
	return nil
}

// acquireLockWithWait repeatedly attempts to acquire the Bun migration lock,
// waiting with exponential backoff so concurrent services serialize safely.
func acquireLockWithWait(ctx context.Context, m *migrate.Migrator, label string) error {
	// Max ~2 minutes with capped backoff
	const maxAttempts = 20
	backoff := 300 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := m.Lock(ctx); err != nil {
			// If context cancelled or deadline exceeded, abort early
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			// Log and wait, then retry
			log.WithFields(log.Fields{"attempt": attempt, "label": label}).Info("Migration lock busy; waiting...")
			time.Sleep(backoff)
			// Exponential backoff capped at 5s
			backoff *= 2
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			continue
		}
		return nil // acquired
	}
	return fmt.Errorf("timed out waiting for %s migration lock", label)
}

// runRiverMigrations runs River's built-in migrations to the billing schema.
// River uses its own migration table (billing.river_migration).
func runRiverMigrations(ctx context.Context, cfg *config.Config, schema string) error {
	// Create pgx pool for River migrator
	dbURL := cfg.DB.GetConnectionString()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("create pgx pool: %w", err)
	}
	defer pool.Close()

	// Create River migrator with billing schema
	riverCfg := &rivermigrate.Config{}
	if schema != "" && schema != "public" {
		riverCfg.Schema = schema
		log.Infof("River using schema: %s", schema)
	}

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), riverCfg)
	if err != nil {
		return fmt.Errorf("create river migrator: %w", err)
	}

	// Apply all pending migrations
	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("apply river migrations: %w", err)
	}

	if len(res.Versions) == 0 {
		log.Info("No new River migrations to apply")
	} else {
		log.Infof("Applied %d River migration(s):", len(res.Versions))
		for _, migration := range res.Versions {
			log.Infof("  - Version %d: %s", migration.Version, migration.Name)
		}
	}
	return nil
}
