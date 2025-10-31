package migrate

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/doujins-org/doujins-billing/config"
	chmigrations "github.com/doujins-org/doujins-billing/migrations/clickhouse"
	log "github.com/sirupsen/logrus"
)

// splitCHStatements splits SQL text into semicolon-terminated statements, removing block/line comments.
func splitCHStatements(sql string) []string {
	s := strings.ReplaceAll(sql, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	for {
		start := strings.Index(s, "/*")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+2:], "*/")
		if end < 0 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+2+end+2:]
	}
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "--") || t == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	cleaned := b.String()
	raw := strings.Split(cleaned, ";")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		stmt := strings.TrimSpace(part)
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}

func applyClickHouseMigrations(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || cfg.ClickHouse == nil || cfg.ClickHouse.HTTPAddr == "" {
		log.Info("ClickHouse not configured; skipping CH migrations")
		return nil
	}
	// Parse HTTPAddr to decide protocol and address
	u, err := url.Parse(cfg.ClickHouse.HTTPAddr)
	if err != nil {
		return fmt.Errorf("invalid clickhouse http_addr: %w", err)
	}
	protocol := ch.Native
	addr := u.Host
	if u.Scheme == "http" || u.Scheme == "https" {
		// For the HTTP protocol, clickhouse-go expects bare host:port in Addr
		// and constructs the scheme itself. Supplying a scheme here results in
		// a malformed URL like "http://http://host:8123".
		protocol = ch.HTTP
		addr = u.Host
	} else if addr == "" {
		// Fallback for bare host:port style strings
		addr = cfg.ClickHouse.HTTPAddr
	}

	// Connect to default DB first to ensure target DB exists
	ensureConn, err := ch.Open(&ch.Options{
		Protocol: protocol,
		Addr:     []string{addr},
		Auth: ch.Auth{
			Database: "default",
			Username: cfg.ClickHouse.Username,
			Password: cfg.ClickHouse.Password,
		},
		DialTimeout: 10 * time.Second,
		Settings:    ch.Settings{"async_insert": 0},
	})
	if err != nil {
		return fmt.Errorf("clickhouse connect (ensure): %w", err)
	}
	defer ensureConn.Close()
	if cfg.ClickHouse.Database != "" {
		if err := ensureConn.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", cfg.ClickHouse.Database)); err != nil {
			return fmt.Errorf("create database: %w", err)
		}
	}

	// Connect to target database for migrations
	conn, err := ch.Open(&ch.Options{
		Protocol: protocol,
		Addr:     []string{addr},
		Auth: ch.Auth{
			Database: cfg.ClickHouse.Database,
			Username: cfg.ClickHouse.Username,
			Password: cfg.ClickHouse.Password,
		},
		DialTimeout: 10 * time.Second,
		Settings:    ch.Settings{"async_insert": 0},
	})
	if err != nil {
		return fmt.Errorf("clickhouse connect: %w", err)
	}
	defer conn.Close()

	// Init tracking table
	if err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS clickhouse_migrations (migration_name String, applied_at DateTime('UTC') DEFAULT now()) ENGINE = MergeTree() ORDER BY migration_name`); err != nil {
		return fmt.Errorf("init clickhouse_migrations: %w", err)
	}

	// Collect embedded migration files
	dirEntries, err := chmigrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read embedded clickhouse migrations: %w", err)
	}
	files := make([]string, 0, len(dirEntries))
	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".sql") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	for _, path := range files {
		base := path
		// Check already applied using parameterized query
		var cnt uint64
		if err := conn.QueryRow(ctx, "SELECT count() FROM clickhouse_migrations WHERE migration_name = ?", base).Scan(&cnt); err == nil && cnt > 0 {
			log.WithField("migration", base).Debug("CH migration already applied; skipping")
			continue
		}
		b, err := chmigrations.FS.ReadFile(base)
		if err != nil {
			return fmt.Errorf("read CH migration %s: %w", base, err)
		}
		for _, stmt := range splitCHStatements(string(b)) {
			if err := conn.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("clickhouse migration %s failed: %w", base, err)
			}
		}
		if err := conn.Exec(ctx, "INSERT INTO clickhouse_migrations (migration_name) VALUES (?)", base); err != nil {
			return fmt.Errorf("record CH migration %s failed: %w", base, err)
		}
		log.WithField("migration", base).Info("Applied ClickHouse migration")
	}
	return nil
}
