package postgresmigrations

import (
	"embed"

	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun/migrate"
)

//go:embed *.sql
var migrationFS embed.FS

// Migrations exposes discovered SQL migrations for Bun's migrator.
var Migrations = migrate.NewMigrations()

func init() {
	if err := Migrations.Discover(migrationFS); err != nil {
		log.WithError(err).Fatal("failed to discover Postgres migrations")
	}

	migrations := Migrations.Sorted()
	log.WithField("count", len(migrations)).Info("Postgres migrations discovered")
}
