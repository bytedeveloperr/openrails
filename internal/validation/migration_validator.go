package validation

import (
	"context"
	"fmt"

	postgresmigrations "github.com/doujins-org/doujins-billing/migrations/postgres"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

// ValidateMigrations checks that all embedded migrations have been applied to the database.
// Uses Bun's built-in migration status checking instead of custom queries.
// Returns an error if any migrations are pending, indicating the application should not start.
func ValidateMigrations(ctx context.Context, db *bun.DB, schema string) error {
	log.WithField("schema", schema).Info("Validating database migrations on startup")

	if schema == "" {
		schema = "billing"
	}

	// Create migrator with the same configuration used for actual migrations
	migrator := migrate.NewMigrator(db, postgresmigrations.Migrations,
		migrate.WithTableName(schema+".migrations"),
		migrate.WithLocksTableName(schema+".migration_locks"),
	)

	// Use Bun's built-in MigrationsWithStatus to get migration status
	migrations, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get migration status: %w", err)
	}

	if len(migrations) == 0 {
		log.Warn("No migrations found - this may be intentional during development")
		return nil
	}

	// Check for unapplied migrations using Bun's IsApplied() method
	var pendingMigrations []string
	appliedCount := 0
	for _, migration := range migrations {
		if !migration.IsApplied() {
			pendingMigrations = append(pendingMigrations, migration.Name)
		} else {
			appliedCount++
		}
	}

	// Check for orphaned migrations (exist in database but not in code)
	// These are warnings, not errors - app will still start
	missingMigrations, err := migrator.MissingMigrations(ctx)
	if err != nil {
		log.WithError(err).Warn("Failed to check for orphaned migrations - continuing anyway")
	} else if len(missingMigrations) > 0 {
		orphanedNames := make([]string, len(missingMigrations))
		for i, m := range missingMigrations {
			orphanedNames[i] = m.Name
		}
		log.WithFields(log.Fields{
			"orphaned_count":      len(missingMigrations),
			"orphaned_migrations": orphanedNames,
		}).Warn("Database contains migrations not present in code - these may be from a rollback or old version")
	}

	if len(pendingMigrations) > 0 {
		log.WithFields(log.Fields{
			"pending_count":      len(pendingMigrations),
			"pending_migrations": pendingMigrations,
		}).Error("Pending migrations detected - application cannot start")
		return fmt.Errorf("cannot start: %d pending migrations must be applied first: %v",
			len(pendingMigrations), pendingMigrations)
	}

	log.WithFields(log.Fields{
		"total_migrations":   len(migrations),
		"applied_migrations": appliedCount,
	}).Info("Migration validation passed")

	return nil
}
