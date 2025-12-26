package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/docker-faas/docker-faas/pkg/config"
	"github.com/docker-faas/docker-faas/pkg/gateway"
	"github.com/docker-faas/docker-faas/pkg/middleware"
	"github.com/docker-faas/docker-faas/pkg/provider"
	"github.com/docker-faas/docker-faas/pkg/router"
	"github.com/docker-faas/docker-faas/pkg/store"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Setup logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	logger.Info("Starting Docker FaaS Gateway...")
	logger.Infof("Configuration: port=%s, network=%s, auth=%v", cfg.GatewayPort, cfg.FunctionsNetwork, cfg.AuthEnabled)

	// Initialize store
	st, err := store.NewStore(cfg.StateDBPath)
	if err != nil {
		logger.Fatalf("Failed to initialize store: %v", err)
	}
	defer st.Close()

	// Initialize Docker provider
	dockerProvider, err := provider.NewDockerProvider(cfg.DockerHost, cfg.FunctionsNetwork, cfg.DebugBindAddress, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize Docker provider: %v", err)
	}
	defer dockerProvider.Close()

	// Initialize router
	rt := router.NewRouter(dockerProvider, logger, cfg.ReadTimeout, cfg.WriteTimeout, cfg.ExecTimeout)

	// Initialize gateway
	gw := gateway.NewGateway(st, dockerProvider, rt, logger, cfg.FunctionsNetwork)

	// Setup HTTP router
	r := mux.NewRouter()

	// System endpoints
	r.HandleFunc("/system/info", gw.HandleSystemInfo).Methods("GET")
	r.HandleFunc("/system/functions", gw.HandleListFunctions).Methods("GET")
	r.HandleFunc("/system/functions", gw.HandleDeployFunction).Methods("POST")
	r.HandleFunc("/system/functions", gw.HandleUpdateFunction).Methods("PUT")
	r.HandleFunc("/system/functions", gw.HandleDeleteFunction).Methods("DELETE")
	r.HandleFunc("/system/scale-function/{name}", gw.HandleScaleFunction).Methods("POST")
	r.HandleFunc("/system/logs", gw.HandleGetLogs).Methods("GET")

	// Secret management endpoints
	r.HandleFunc("/system/secrets", gw.HandleCreateSecret).Methods("POST")
	r.HandleFunc("/system/secrets", gw.HandleUpdateSecret).Methods("PUT")
	r.HandleFunc("/system/secrets", gw.HandleDeleteSecret).Methods("DELETE")
	r.HandleFunc("/system/secrets", gw.HandleListSecrets).Methods("GET")
	r.HandleFunc("/system/secrets/{name}", gw.HandleGetSecret).Methods("GET")

	// Function invocation
	r.HandleFunc("/function/{name}", gw.HandleInvokeFunction).Methods("POST", "GET", "PUT", "DELETE", "PATCH")

	// Health check
	r.HandleFunc("/healthz", gw.HandleHealthz).Methods("GET")

	// Apply middleware
	loggingMiddleware := middleware.NewLoggingMiddleware(logger)
	authMiddleware := middleware.NewBasicAuthMiddleware(cfg.AuthUser, cfg.AuthPassword, cfg.AuthEnabled, logger)

	handler := loggingMiddleware.Middleware(authMiddleware.Middleware(r))

	// Setup HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.GatewayPort),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Start metrics server if enabled
	if cfg.MetricsEnabled {
		metricsRouter := http.NewServeMux()
		metricsRouter.Handle("/metrics", promhttp.Handler())

		metricsSrv := &http.Server{
			Addr:    fmt.Sprintf(":%s", cfg.MetricsPort),
			Handler: metricsRouter,
		}

		go func() {
			logger.Infof("Metrics server listening on :%s", cfg.MetricsPort)
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Errorf("Metrics server error: %v", err)
			}
		}()
	}

	// Start server in goroutine
	go func() {
		logger.Infof("Gateway server listening on :%s", cfg.GatewayPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorf("Server shutdown error: %v", err)
	}

	logger.Info("Server stopped")
}
