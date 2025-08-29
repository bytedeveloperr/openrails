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
		Use:   "billing-service",
		Short: "Doujins Billing Service",
		Long:  "Standalone billing service for handling payments and subscriptions",
	}

	rootCmd.PersistentFlags().StringP("config", "c", "", "Path to config file (default: config.yaml)")

	// Server command
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Start the billing service server",
		RunE:  runServer,
	}

	// Server flags
	serverCmd.Flags().IntP("public-port", "", 2052, "Public listener port (frontend and webhooks)")
	serverCmd.Flags().IntP("private-port", "", 8060, "Private listener port (api-server communication)")
	serverCmd.Flags().StringP("host", "", "0.0.0.0", "Host to bind to")

	// Worker command
	workerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Start the billing service background workers",
		RunE:  runWorker,
	}

	rootCmd.AddCommand(serverCmd, workerCmd)

	if err := rootCmd.Execute(); err != nil {
		log.WithError(err).Fatal("Failed to execute command")
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set gin mode based on environment
	if cfg.Env == "production" || cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Get ports from flags or config
	publicPort, _ := cmd.Flags().GetInt("public-port")
	privatePort, _ := cmd.Flags().GetInt("private-port")
	host, _ := cmd.Flags().GetString("host")

	// Create billing server
	billingServer, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create billing server: %w", err)
	}

	// Setup graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start public listener (frontend and webhooks)
	publicAddr := fmt.Sprintf("%s:%d", host, publicPort)
	publicServer := &http.Server{
		Addr:    publicAddr,
		Handler: billingServer.PublicHandler(),
	}

	go func() {
		log.Infof("Starting public billing service on %s", publicAddr)
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Public server failed to start")
		}
	}()

	// Start private listener (api-server communication)
	privateAddr := fmt.Sprintf("%s:%d", host, privatePort)
	privateServer := &http.Server{
		Addr:    privateAddr,
		Handler: billingServer.PrivateHandler(),
	}

	go func() {
		log.Infof("Starting private billing service on %s", privateAddr)
		if err := privateServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Private server failed to start")
		}
	}()

	// Wait for interrupt signal
	<-sigChan
	log.Info("Shutdown signal received, gracefully shutting down...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown both servers
	if err := publicServer.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Public server forced to shutdown")
	}

	if err := privateServer.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Private server forced to shutdown")
	}

	// Cleanup resources
	if err := billingServer.Close(shutdownCtx); err != nil {
		log.WithError(err).Error("Error during billing server cleanup")
	}

	log.Info("Billing service shutdown complete")
	return nil
}

func runWorker(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create billing server for worker access to dependencies
	billingServer, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create billing server: %w", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start background workers
	log.Info("Starting billing service background workers...")
	if err := billingServer.StartWorkers(ctx); err != nil {
		return fmt.Errorf("failed to start workers: %w", err)
	}

	// Wait for interrupt signal
	<-sigChan
	log.Info("Shutdown signal received, stopping workers...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop workers
	if err := billingServer.StopWorkers(shutdownCtx); err != nil {
		log.WithError(err).Error("Error stopping workers")
	}

	// Cleanup resources
	if err := billingServer.Close(shutdownCtx); err != nil {
		log.WithError(err).Error("Error during billing server cleanup")
	}

	log.Info("Billing service workers shutdown complete")
	return nil
}
