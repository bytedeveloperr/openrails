package migrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	postgresmigrations "github.com/doujins-org/doujins-billing/migrations/postgres"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

// Run applies Postgres migrations using Bun migrate (tracked in billing.migrations) and
// then applies ClickHouse migrations with per-file tracking in clickhouse_migrations.
func Run(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.DB == nil {
		return fmt.Errorf("missing database config")
	}

	// ---------- Postgres via Bun migrate ----------
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
	opts := []migrate.MigratorOption{
		migrate.WithTableName(schema + ".migrations"),
		migrate.WithLocksTableName(schema + ".migration_locks"),
		migrate.WithMarkAppliedOnSuccess(true),
	}
	m := migrate.NewMigrator(bunDB, postgresmigrations.Migrations, opts...)
	if err := m.Init(ctx); err != nil {
		// Init is idempotent but may fail if concurrent; continue if already exists
		log.WithError(err).Warn("migrations: init returned error; continuing")
	}
	if err := m.Lock(ctx); err != nil {
		return fmt.Errorf("migrations: lock: %w", err)
	}
	var migErr error
	defer func() {
		if unlockErr := m.Unlock(ctx); unlockErr != nil {
			log.WithError(unlockErr).Warn("migrations: unlock failed")
		}
	}()
	if _, err := m.Migrate(ctx); err != nil {
		// Bun returns an error when there are zero discovered migrations.
		// Treat that as a no-op so ClickHouse migrations can still run.
		if strings.Contains(err.Error(), "there are no migrations") {
			log.Info("No Postgres migrations discovered; skipping PG migrate")
		} else {
			migErr = fmt.Errorf("migrations: apply: %w", err)
		}
	}
	if migErr != nil {
		return migErr
	}

	// ---------- ClickHouse (per-file tracked) ----------
	if err := applyClickHouseMigrations(ctx, cfg); err != nil {
		return err
	}

	log.Info("All migrations applied successfully (Postgres + ClickHouse)")
	return nil
}
