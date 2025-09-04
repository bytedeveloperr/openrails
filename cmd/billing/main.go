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

	rootCmd.AddCommand(serverCmd, workerCmd)
	if err := rootCmd.Execute(); err != nil {
		log.WithError(err).Fatal("Failed to execute command")
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	cfg := cmd.Context().Value(config.ConfigContextKey).(*config.Config)

	if cfg.Env == "production" || cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	billingServer, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create billing server: %w", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	server := &http.Server{
		Handler: billingServer.Handler(),
		Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	}

	// Start server in a goroutine
	go func() {
		log.Infof("Starting billing server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Failed to start server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	<-sigChan
	log.Info("Shutdown signal received, shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Server forced to shutdown")
	}

	if err := billingServer.Close(shutdownCtx); err != nil {
		log.WithError(err).Error("Error during billing server cleanup")
	}

	log.Info("Billing service shutdown complete")
	return nil
}

func runWorker(cmd *cobra.Command, args []string) error {
	cfg := cmd.Context().Value(config.ConfigContextKey).(*config.Config)
	billingServer, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create billing server: %w", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Info("Shutdown signal received, stopping workers...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := billingServer.Close(shutdownCtx); err != nil {
		log.WithError(err).Error("Error during billing server cleanup")
	}

	log.Info("Billing service workers shutdown complete")
	return nil
}
