package migrate

import (
	"context"
	"database/sql"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
)

// Run applies pending SQL files from migrations/postgres_ongoing into DB_SCHEMA.
func Run(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.DB == nil {
		return fmt.Errorf("missing database config")
	}
	schema := cfg.DB.Schema
	if schema == "" {
		schema = "billing"
	}

	database, err := db.NewDB(cfg.DB)
	if err != nil {
		return err
	}
	defer database.Close()

	sqldb := database.GetDB()
	// Ensure search_path
	if _, err := sqldb.ExecContext(ctx, fmt.Sprintf("SET search_path TO \"%s\", public", schema)); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	// Acquire advisory lock (bigint key derived from schema name) for cross-process safety
	lockKey := advisoryKey(schema)
	ctxLock, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
    // bun/pgdriver uses '?' placeholders, not $1
    if _, err := sqldb.ExecContext(ctxLock, "SELECT pg_advisory_lock(?)", lockKey); err != nil {
        return fmt.Errorf("acquire advisory lock: %w", err)
    }
    defer func() { _, _ = sqldb.ExecContext(context.Background(), "SELECT pg_advisory_unlock(?)", lockKey) }()

    // Ensure migrations table
    create := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s".schema_migrations (
        id bigserial PRIMARY KEY,
        filename text NOT NULL UNIQUE,
        checksum text NOT NULL,
        applied_at timestamptz NOT NULL DEFAULT now()
    );`, schema)
    if _, err := sqldb.ExecContext(ctx, create); err != nil {
        return fmt.Errorf("ensure schema_migrations: %w", err)
    }

    // Apply SQL under migrations/postgres (single source of truth)
    if err := applyDir(ctx, sqldb, schema, "migrations/postgres"); err != nil {
        return err
    }
    return nil
}

// advisoryKey creates a stable bigint key from schema string.
func advisoryKey(schema string) int64 {
	// simple 64-bit hash: md5 then take first 8 bytes
	sum := md5.Sum([]byte(schema))
	// interpret as signed 64-bit
	var v int64
	for i := 0; i < 8; i++ {
		v = (v << 8) | int64(sum[i])
	}
	if v == 0 {
		v = 0x62696C6C696E67
	} // 'billing'
	return v
}

// (no try loop; we rely on ExecContext + context timeout)

// applyDir applies all .sql files in the given directory in lexicographic order,
// recording each (filename, checksum) in the schema_migrations table to avoid reapplication.
func applyDir(ctx context.Context, sqldb interface{
    ExecContext(context.Context, string, ...any) (sql.Result, error)
    QueryRowContext(context.Context, string, ...any) *sql.Row
}, schema, dir string) error {
    // Read files; skip if dir is missing
    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return fmt.Errorf("read migrations dir %s: %w", dir, err)
    }
    files := make([]string, 0, len(entries))
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        name := e.Name()
        if strings.HasSuffix(strings.ToLower(name), ".sql") {
            files = append(files, filepath.Join(dir, name))
        }
    }
    sort.Strings(files)
    for _, path := range files {
        b, err := os.ReadFile(path)
        if err != nil {
            return fmt.Errorf("read %s: %w", path, err)
        }
        sumBytes := md5.Sum(b)
        checksum := hex.EncodeToString(sumBytes[:])
        base := filepath.Base(path)
        var exists int
        q := fmt.Sprintf("SELECT 1 FROM \"%s\".schema_migrations WHERE filename=? AND checksum=? LIMIT 1", schema)
        if err := sqldb.QueryRowContext(ctx, q, base, checksum).Scan(&exists); err == nil && exists == 1 {
            continue
        }
        if _, err := sqldb.ExecContext(ctx, string(b)); err != nil {
            return fmt.Errorf("apply %s failed: %w", base, err)
        }
        ins := fmt.Sprintf("INSERT INTO \"%s\".schema_migrations(filename, checksum) VALUES(?,?) ON CONFLICT (filename) DO UPDATE SET checksum=EXCLUDED.checksum, applied_at=now()", schema)
        if _, err := sqldb.ExecContext(ctx, ins, base, checksum); err != nil {
            return fmt.Errorf("record migration %s: %w", base, err)
        }
    }
    return nil
}
