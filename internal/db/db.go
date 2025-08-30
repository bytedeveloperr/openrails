package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/sirupsen/logrus"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
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
	default:
		err = fmt.Errorf("unsupported database dialect: %s", dialect)
	}

	if err != nil {
		return nil, err
	}

	if err := db.PingContext(context.Background()); err != nil {
		if underlyingDB := db.DB; underlyingDB != nil {
			if cerr := underlyingDB.Close(); cerr != nil {
				logrus.Errorf("failed to close database connection: %v", cerr)
			}
		}

		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(false),
		bundebug.WithEnabled(false),
	))

	schema := cfg.Schema
	if schema == "" {
		schema = "billings"
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

func (d *DB) GetSchema() string {
	return d.schema
}

func (d *DB) QualifiedTable(tableName string) string {
	return fmt.Sprintf("%s.%s", d.schema, tableName)
}

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
		schema: "billings",
	}
}

func (d *DB) NewWithTx(tx bun.Tx) *DB {
	return &DB{
		db:     tx,
		schema: d.schema,
	}
}
