package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/auvitly/cmdui.git/internal/api"
	"github.com/auvitly/cmdui.git/internal/config"
	"github.com/auvitly/cmdui.git/internal/repository/sqlite"
	authservice "github.com/auvitly/cmdui.git/internal/service/auth"
	commandservice "github.com/auvitly/cmdui.git/internal/service/commands"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	configPath := os.Getenv("CMDUI_CONFIG")
	if configPath == "" {
		configPath = "config.json"
	}
	appConfig, err := config.Load(configPath)
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	store, err := sqlite.OpenStore(appConfig.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	auth := authservice.New(store)
	if err := auth.EnsureConfigured(appConfig.Users); err != nil {
		logger.Error("initialize configured users", "error", err)
		os.Exit(1)
	}
	commandService := commandservice.New(appConfig, store, store, store)

	server := &http.Server{
		Addr:              appConfig.Address,
		Handler:           api.NewApp(appConfig, auth, commandService, logger).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("shutdown HTTP server", "error", err)
		}
	}()

	logger.Info("starting HTTP server", "address", appConfig.Address, "database", appConfig.DatabasePath)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
