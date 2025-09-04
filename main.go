package main

import (
    "context"
    "fmt"
    "net/http"
    "crypto/tls"
    "crypto/x509"
    "io/ioutil"
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

    // Optionally start private TLS listener (mTLS-capable)
    var tlsSrv *http.Server
    if cfg.TLS != nil && cfg.TLS.Private != nil && cfg.TLS.Private.Enabled {
        addr := cfg.TLS.Private.Addr
        if addr == "" {
            addr = ":8060"
        }
        tlsCfg := &tls.Config{}
        // Load server cert/key
        cert, err := tls.LoadX509KeyPair(cfg.TLS.Private.CertFile, cfg.TLS.Private.KeyFile)
        if err != nil {
            return fmt.Errorf("failed loading TLS cert/key: %w", err)
        }
        tlsCfg.Certificates = []tls.Certificate{cert}
        // Optional client cert verification (mTLS)
        if cfg.TLS.Private.ClientCAFile != "" && cfg.TLS.Private.RequireClientCert {
            caCertPEM, err := ioutil.ReadFile(cfg.TLS.Private.ClientCAFile)
            if err != nil {
                return fmt.Errorf("failed reading client CA: %w", err)
            }
            caPool := x509.NewCertPool()
            if ok := caPool.AppendCertsFromPEM(caCertPEM); !ok {
                return fmt.Errorf("failed to parse client CA PEM")
            }
            tlsCfg.ClientCAs = caPool
            tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
        }
        tlsSrv = &http.Server{Addr: addr, Handler: billingServer.Handler(), TLSConfig: tlsCfg}
        go func() {
            log.Infof("Starting private TLS listener on %s", addr)
            if err := tlsSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
                log.WithError(err).Error("Private TLS server failed")
            }
        }()
    }

    // Wait for interrupt signal to gracefully shutdown the server
    <-sigChan
    log.Info("Shutdown signal received, shutting down server...")

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        log.WithError(err).Error("Server forced to shutdown")
    }
    if tlsSrv != nil {
        if err := tlsSrv.Shutdown(shutdownCtx); err != nil {
            log.WithError(err).Error("Private TLS server forced to shutdown")
        }
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

    log.Info("Billing service workers shutdown complete")
    return nil
}
