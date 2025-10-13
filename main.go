package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/app"
	"github.com/doujins-org/doujins-billing/internal/migrate"
	"github.com/doujins-org/doujins-billing/internal/server"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "billing",
		Short: "Doujins Billing Service",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return fmt.Errorf("failed to get config flag: %w", err)
			}

			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			cmd.SetContext(context.WithValue(cmd.Context(), config.ConfigContextKey, cfg))
			return nil
		},
		Long: "Standalone billing service for handling payments and subscriptions",
	}

	rootCmd.PersistentFlags().
		StringP("config", "c", "config.yaml", "Path to config file")

	serverCmd := &cobra.Command{
		Use:   "server",
		RunE:  runServer,
		Short: "Start the billing service server",
	}

	workerCmd := &cobra.Command{
		Use:   "worker",
		RunE:  runWorker,
		Short: "Start the billing service background workers",
	}

	rootCmd.AddCommand(serverCmd, workerCmd, migrateCmd)
	if err := rootCmd.Execute(); err != nil {
		log.WithError(err).Fatal("Failed to execute command")
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	cfg := cmd.Context().Value(config.ConfigContextKey).(*config.Config)
	// Auto-apply app migrations on startup (idempotent; guarded by advisory lock)
	if err := migrate.Run(cmd.Context(), cfg); err != nil {
		return fmt.Errorf("auto-migrate failed: %w", err)
	}

	if cfg.Env == "production" || cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	application, err := app.Bootstrap(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap application: %w", err)
	}
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			if err := application.Close(context.Background()); err != nil {
				log.WithError(err).Error("Application cleanup failed")
			}
		}
	}()

	billingServer, err := server.New(server.Dependencies{
		Config:       application.Config,
		Cache:        application.Cache,
		State:        application.State,
		Redis:        application.RedisClient,
		AuthVerifier: application.AuthVerifier,
	})
	if err != nil {
		return fmt.Errorf("failed to create billing server: %w", err)
	}
	cleanupOnError = false

	// Ensure API routes are registered and background workers are started
	billingServer.StartWorkers(cmd.Context())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	srv := &http.Server{
		Handler: billingServer.Handler(),
		Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	}

	// Start server in a goroutine
	go func() {
		log.Infof("Starting billing server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Failed to start server")
		}
	}()

	// Start private admin listener (internal-only via compose network)
	adminAddr := ":8060"
	adminSrv := &http.Server{Addr: adminAddr, Handler: billingServer.AdminHandler()}
	go func() {
		log.Infof("Starting billing admin server on %s", adminAddr)
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Error("Admin server failed")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	<-sigChan
	log.Info("Shutdown signal received, shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Server forced to shutdown")
	}
	if err := adminSrv.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Admin server forced to shutdown")
	}

	if err := billingServer.Close(shutdownCtx); err != nil {
		log.WithError(err).Error("Error during billing server cleanup")
	}
	if err := application.Close(shutdownCtx); err != nil {
		log.WithError(err).Error("Application shutdown encountered issues")
	}

	log.Info("Billing service shutdown complete")
	return nil
}

func runWorker(cmd *cobra.Command, args []string) error {
	cfg := cmd.Context().Value(config.ConfigContextKey).(*config.Config)
	application, err := app.Bootstrap(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap application: %w", err)
	}
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			if err := application.Close(context.Background()); err != nil {
				log.WithError(err).Error("Application cleanup failed")
			}
		}
	}()

	billingServer, err := server.New(server.Dependencies{
		Config:       application.Config,
		Cache:        application.Cache,
		State:        application.State,
		Redis:        application.RedisClient,
		AuthVerifier: application.AuthVerifier,
	})
	if err != nil {
		return fmt.Errorf("failed to create billing server: %w", err)
	}
	cleanupOnError = false

	// Start only background workers (no HTTP server)
	billingServer.StartWorkers(cmd.Context())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Info("Shutdown signal received, stopping workers...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := billingServer.Close(shutdownCtx); err != nil {
		log.WithError(err).Error("Error during billing server cleanup")
	}
	if err := application.Close(shutdownCtx); err != nil {
		log.WithError(err).Error("Application shutdown encountered issues")
	}

	log.Info("Billing service workers shutdown complete")
	return nil
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply pending database migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := cmd.Context().Value(config.ConfigContextKey).(*config.Config)
		ctx := cmd.Context()
		if err := migrate.Run(ctx, cfg); err != nil {
			return fmt.Errorf("migrations failed: %w", err)
		}
		log.Info("Database migrations complete")
		return nil
	},
}
