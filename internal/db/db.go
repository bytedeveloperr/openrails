package database

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db/models"
)

type DB struct {
	db     bun.IDB
	schema string
}

func NewDB(cfg *config.DBConfig) (_ *DB, err error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("missing database url")
	}

	var db *bun.DB

	url := cfg.URL
	dialect := cfg.Dialect

	// Replace dialect if/else chain with a tagged switch
	switch dialect {
	case "postgres":
		connParams := map[string]any{}
		if cfg.Schema != "" {
			connParams["search_path"] = cfg.Schema
		}

		sqldb := sql.OpenDB(pgdriver.NewConnector(
			pgdriver.WithDSN(url),
			pgdriver.WithConnParams(connParams),
		))

		db = bun.NewDB(sqldb, pgdialect.New())
		models.RegisterModels(db)

	case "mysql":
		sqldb, mysqlErr := sql.Open("mysql", url)
		if mysqlErr != nil {
			return nil, mysqlErr // Return early if sql.Open fails
		}
		db = bun.NewDB(sqldb, mysqldialect.New())
		models.RegisterModels(db)

	default:
		err = fmt.Errorf("unsupported database dialect: %s", dialect)
	}

	if err != nil {
		return nil, err
	}

	// Verify database connection using the bun.DB interface
	if err := db.PingContext(context.Background()); err != nil {
		// Attempt to close the underlying *sql.DB if Ping fails
		if underlyingDB := db.DB; underlyingDB != nil {
			cerr := underlyingDB.Close()
			if err == nil {
				err = cerr
			}
		}
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Disable query logging
	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(false),
		bundebug.WithEnabled(false),
	))

	// Default schema to "doujins" if not specified for backward compatibility
	schema := cfg.Schema
	if schema == "" {
		schema = "doujins"
	}

	dbInstance := &DB{
		db:     db,
		schema: schema,
	}

	return dbInstance, nil
}

func (d *DB) GetDB() bun.IDB {
	return d.db
}

// GetSchema returns the configured database schema name
func (d *DB) GetSchema() string {
	return d.schema
}

// QualifiedTable returns a schema-qualified table name (schema.table_name)
func (d *DB) QualifiedTable(tableName string) string {
	return fmt.Sprintf("%s.%s", d.schema, tableName)
}

// Close closes the database connection
func (d *DB) Close() error {
	if bunDB, ok := d.db.(*bun.DB); ok {
		if underlyingDB := bunDB.DB; underlyingDB != nil {
			return underlyingDB.Close()
		}
	}
	return nil
}

func NewWithTx(tx bun.Tx) *DB {
	return &DB{
		db:     tx,
		schema: "doujins", // Default schema for transactions
	}
}

// NewWithTxAndSchema creates a new DB instance with transaction and specific schema
func (d *DB) NewWithTx(tx bun.Tx) *DB {
	return &DB{
		db:     tx,
		schema: d.schema, // Inherit schema from parent DB
	}
}

// CheckPostgreSQLExtensions checks if required PostgreSQL extensions are installed
func (d *DB) CheckPostgreSQLExtensions(ctx context.Context) error {
	requiredExtensions := []string{"pg_trgm"}

	type ExtensionInfo struct {
		ExtName   string `bun:"extname"`
		Installed bool   `bun:"installed"`
	}

	// Check each extension
	missingExtensions := []string{}
	for _, ext := range requiredExtensions {
		count, err := d.db.NewSelect().
			Table("pg_extension").
			Where("extname = ?", ext).
			Count(ctx)
		if err != nil {
			// If the query fails (e.g., pg_extension doesn't exist), log and continue
			log.WithError(err).Debugf("Failed to check for extension %s", ext)
			missingExtensions = append(missingExtensions, ext)
			continue
		}

		if count == 0 {
			missingExtensions = append(missingExtensions, ext)
		}
	}

	// Log extension status
	if len(missingExtensions) > 0 {
		log.WithField("missing_extensions", missingExtensions).
			Warn("PostgreSQL extensions not installed. Fuzzy search functionality will be limited.")
		log.Info("To enable fuzzy search, run the following SQL commands as a superuser:")
		for _, ext := range missingExtensions {
			log.Infof("  CREATE EXTENSION IF NOT EXISTS %s;", ext)
		}
		return fmt.Errorf("missing PostgreSQL extensions: %v", missingExtensions)
	}

	log.Info("All required PostgreSQL extensions are installed: pg_trgm")
	return nil
}
