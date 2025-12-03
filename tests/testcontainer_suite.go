//go:build integration

package tests

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/app"
	"github.com/doujins-org/doujins-billing/internal/migrate"
	"github.com/doujins-org/doujins-billing/internal/server"

	_ "github.com/lib/pq" // PostgreSQL driver for schema creation
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	redismodule "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
)

// TestContainerSuite manages test containers for integration testing
type TestContainerSuite struct {
	t *testing.T

	// Containers
	postgresContainer   *postgres.PostgresContainer
	redisContainer      *redismodule.RedisContainer
	clickhouseContainer *clickhouse.ClickHouseContainer

	// Application and database connections
	App         *app.App
	BunDB       *bun.DB
	RedisClient *redis.Client

	// Server and configuration
	Server     *server.Server
	httpServer *http.Server
	Config     *config.Config
	ServerURL  string

	// Context for container operations
	ctx context.Context
}

// NewTestContainerSuite creates a new test container suite
func NewTestContainerSuite(t *testing.T) *TestContainerSuite {
	suite := &TestContainerSuite{
		t:   t,
		ctx: context.Background(),
	}

	suite.SetupSuite()
	return suite
}

// SetupSuite initializes all test containers and services
func (suite *TestContainerSuite) SetupSuite() {
	suite.t.Helper()

	// Set log level to reduce noise during tests
	logrus.SetLevel(logrus.WarnLevel)

	// Start containers
	suite.startPostgresContainer()
	suite.startRedisContainer()
	suite.startClickHouseContainer()

	// Initialize config with container connection details
	suite.initializeDatabaseConnections()

	// Run database migrations (creates schema before app connects)
	suite.runDatabaseMigrations()

	// Initialize server (bootstraps the app and sets up DB connection)
	suite.initializeServer()

	// Wait for server to be ready
	suite.waitForServerReady()
}

// startPostgresContainer starts a PostgreSQL test container
func (suite *TestContainerSuite) startPostgresContainer() {
	suite.t.Helper()

	container, err := postgres.Run(suite.ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("test_db"),
		postgres.WithUsername("test_user"),
		postgres.WithPassword("test_password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(suite.t, err)

	suite.postgresContainer = container
}

// startRedisContainer starts a Redis test container
func (suite *TestContainerSuite) startRedisContainer() {
	suite.t.Helper()

	container, err := redismodule.Run(suite.ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(suite.t, err)

	suite.redisContainer = container
}

// startClickHouseContainer starts a ClickHouse test container
func (suite *TestContainerSuite) startClickHouseContainer() {
	suite.t.Helper()

	container, err := clickhouse.Run(suite.ctx,
		"clickhouse/clickhouse-server:23.8-alpine",
		clickhouse.WithUsername("test_user"),
		clickhouse.WithPassword("test_password"),
		clickhouse.WithDatabase("test_analytics"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/ping").
				WithPort("8123/tcp").
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(suite.t, err)

	suite.clickhouseContainer = container
}

// initializeDatabaseConnections sets up database connections
func (suite *TestContainerSuite) initializeDatabaseConnections() {
	suite.t.Helper()

	// Get PostgreSQL connection string
	postgresConnStr, err := suite.postgresContainer.ConnectionString(suite.ctx, "sslmode=disable")
	require.NoError(suite.t, err)

	// Get Redis connection details
	redisHost, err := suite.redisContainer.Host(suite.ctx)
	require.NoError(suite.t, err)
	redisPort, err := suite.redisContainer.MappedPort(suite.ctx, "6379")
	require.NoError(suite.t, err)

	// Get ClickHouse connection details
	clickhouseHost, err := suite.clickhouseContainer.Host(suite.ctx)
	require.NoError(suite.t, err)
	clickhousePort, err := suite.clickhouseContainer.MappedPort(suite.ctx, "8123")
	require.NoError(suite.t, err)

	// Create configuration
	// Use "dev" to skip NMI/CCBill validation in config.Validate()
	// Use the JWKS server URL as the issuer so auth verification works
	jwksIssuer := GetTestIssuerURL()
	suite.Config = &config.Config{
		Env:  "dev",
		Host: "localhost",
		Port: 8080, // Fixed port for testing
		DB: &config.DBConfig{
			URL: postgresConnStr,
		},
		Redis: &config.RedisConfig{
			Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort.Port()),
			Password: "",
			DB:       0,
		},
		ClickHouse: &config.ClickHouseConfig{
			HTTPAddr: fmt.Sprintf("http://%s:%s", clickhouseHost, clickhousePort.Port()),
			Database: "test_analytics",
			Username: "test_user",
			Password: "test_password",
		},
		Auth: &config.AuthConfig{
			Issuers:          []string{jwksIssuer},
			ExpectedAudience: "test-app",
		},
	}

	// Initialize Redis connection
	suite.RedisClient = redis.NewClient(&redis.Options{
		Addr:     suite.Config.Redis.Addr,
		Password: suite.Config.Redis.Password,
		DB:       suite.Config.Redis.DB,
	})

	// Test Redis connection
	err = suite.RedisClient.Ping(suite.ctx).Err()
	require.NoError(suite.t, err)
}

// runDatabaseMigrations runs database migrations for testing
func (suite *TestContainerSuite) runDatabaseMigrations() {
	suite.t.Helper()

	// First, create the required schemas (normally done by bootstrap SQL)
	postgresConnStr, err := suite.postgresContainer.ConnectionString(suite.ctx, "sslmode=disable")
	require.NoError(suite.t, err)

	// Connect directly to create schemas
	sqlDB, err := sql.Open("postgres", postgresConnStr)
	require.NoError(suite.t, err)
	defer sqlDB.Close()

	// Create required schemas
	_, err = sqlDB.ExecContext(suite.ctx, `
		CREATE SCHEMA IF NOT EXISTS billing;
		CREATE SCHEMA IF NOT EXISTS profiles;
	`)
	require.NoError(suite.t, err)

	// Run all migrations using the migrate package
	err = migrate.RunPostgres(suite.ctx, suite.Config)
	require.NoError(suite.t, err)
}

// initializeServer starts the billing server for testing
func (suite *TestContainerSuite) initializeServer() {
	suite.t.Helper()

	// Bootstrap the application (creates runtime, cache, auth verifier, etc.)
	application, err := app.Bootstrap(suite.Config)
	require.NoError(suite.t, err)
	suite.App = application

	// Get the BunDB from the app runtime
	suite.BunDB = application.Runtime.DB.GetDB().(*bun.DB)

	// Create server with dependencies
	billingServer, err := server.New(server.Dependencies{
		Config:       suite.Config,
		Cache:        application.Cache,
		Runtime:      application.Runtime,
		Redis:        application.RedisClient,
		AuthVerifier: application.AuthVerifier,
	})
	require.NoError(suite.t, err)

	suite.Server = billingServer

	// Start workers
	suite.Server.StartWorkers(suite.ctx)

	// Create HTTP server
	httpServer := &http.Server{
		Handler: suite.Server.Handler(),
		Addr:    fmt.Sprintf("%s:%d", suite.Config.Host, suite.Config.Port),
	}

	// Start server in a goroutine
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			suite.t.Logf("Server failed to start: %v", err)
		}
	}()

	// Store the HTTP server for cleanup
	suite.httpServer = httpServer
	suite.ServerURL = fmt.Sprintf("http://localhost:%d", suite.Config.Port)
}

// waitForServerReady waits for the server to be ready to accept requests
func (suite *TestContainerSuite) waitForServerReady() {
	suite.t.Helper()

	// Wait for server to start
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(suite.ServerURL + "/health/live")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	suite.t.Log("Server ready (or timeout reached)")
}

// Cleanup cleans up all test containers and resources
func (suite *TestContainerSuite) Cleanup() {
	suite.t.Helper()

	// Stop HTTP server
	if suite.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		suite.httpServer.Shutdown(ctx)
	}

	// Stop billing server
	if suite.Server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		suite.Server.Close(ctx)
	}

	// Close application (handles DB, Redis, cache, etc.)
	if suite.App != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		suite.App.Close(ctx)
	}

	// Terminate containers
	if suite.postgresContainer != nil {
		if err := suite.postgresContainer.Terminate(suite.ctx); err != nil {
			suite.t.Logf("Failed to terminate postgres container: %v", err)
		}
	}

	if suite.redisContainer != nil {
		if err := suite.redisContainer.Terminate(suite.ctx); err != nil {
			suite.t.Logf("Failed to terminate redis container: %v", err)
		}
	}

	if suite.clickhouseContainer != nil {
		if err := suite.clickhouseContainer.Terminate(suite.ctx); err != nil {
			suite.t.Logf("Failed to terminate clickhouse container: %v", err)
		}
	}
}

// ExecuteSQL executes a SQL query on the test database
func (suite *TestContainerSuite) ExecuteSQL(query string, args ...interface{}) (sql.Result, error) {
	return suite.BunDB.ExecContext(suite.ctx, query, args...)
}

// ResetDatabase clears all data from test tables for clean test state
func (suite *TestContainerSuite) ResetDatabase() {
	suite.t.Helper()

	// List of tables to truncate (in dependency order)
	tables := []string{
		"billing.subscriptions",
		"billing.payments",
		"billing.payment_methods",
		"billing.prices",
		"billing.products",
	}

	for _, table := range tables {
		_, err := suite.BunDB.ExecContext(suite.ctx, fmt.Sprintf("TRUNCATE TABLE IF EXISTS %s CASCADE", table))
		if err != nil {
			// Log but don't fail - table might not exist
			suite.t.Logf("Failed to truncate table %s: %v", table, err)
		}
	}
}
