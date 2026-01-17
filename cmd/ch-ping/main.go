package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func main() {
	httpAddr := flag.String("http", "http://localhost:8123", "ClickHouse HTTP address (used to derive native port if --client not set)")
	clientAddr := flag.String("client", "", "ClickHouse native address host:port (overrides --http)")
	db := flag.String("db", "analytics", "ClickHouse database")
	user := flag.String("user", "analytics_user", "ClickHouse username")
	password := flag.String("password", "analytics_password", "ClickHouse password")
	timeout := flag.Duration("timeout", 5*time.Second, "Ping timeout")
	flag.Parse()

	addr := deriveAddr(*clientAddr, *httpAddr)
	if addr == "" {
		fmt.Fprintf(os.Stderr, "failed to derive ClickHouse address\n")
		os.Exit(1)
	}

	options := &clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: *db,
			Username: *user,
			Password: *password,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 30,
		},
		DialTimeout: *timeout,
	}

	conn, err := clickhouse.Open(options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open connection failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("ClickHouse ping OK to %s (db=%s, user=%s)\n", addr, *db, *user)
}

func deriveAddr(clientAddr, httpAddr string) string {
	if strings.TrimSpace(clientAddr) != "" {
		return strings.TrimSpace(clientAddr)
	}

	// Try to convert HTTP to native port
	if strings.HasPrefix(httpAddr, "http://") || strings.HasPrefix(httpAddr, "https://") {
		if u, err := url.Parse(httpAddr); err == nil {
			host := u.Hostname()
			port := u.Port()
			if port == "" || port == "8123" {
				port = "9000"
			}
			return host + ":" + port
		}
	}

	// Fall back to provided string, append default port if missing
	if httpAddr == "" {
		return ""
	}
	if strings.Contains(httpAddr, ":") {
		return httpAddr
	}
	return httpAddr + ":9000"
}
