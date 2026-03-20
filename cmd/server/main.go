package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aliipou/observability-platform/internal/alert"
	"github.com/aliipou/observability-platform/internal/api"
	"github.com/aliipou/observability-platform/internal/config"
	"github.com/aliipou/observability-platform/internal/storage"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	log, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// In-memory time series store
	ts := storage.NewTimeSeriesStore()

	// Postgres (optional — gracefully degrade if unavailable)
	var pg *storage.PostgresStore
	pg, err = storage.NewPostgresStore(cfg.DatabaseURL)
	if err != nil {
		log.Warn("postgres unavailable, running without persistent storage", zap.Error(err))
	} else {
		if err := pg.RunMigrations("internal/storage/migrations/001_init.sql"); err != nil {
			log.Warn("migration failed", zap.Error(err))
		}
	}

	// Alert engine
	alertEngine := alert.New(ts, pg, log)
	if err := alertEngine.LoadRules(cfg.AlertRulesDir); err != nil {
		log.Warn("could not load alert rules, starting with empty rule set", zap.Error(err))
	}
	go alertEngine.Run(ctx, 30*time.Second)

	// Periodic cleanup of old in-memory data
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ts.Cleanup()
			}
		}
	}()

	// HTTP API
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(api.Logger(log), api.CORS(), gin.Recovery())

	h := api.New(ts, pg, alertEngine, log)
	h.RegisterRoutes(engine)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      engine,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Info("observability server listening", zap.Int("port", cfg.ServerPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("listen", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	if pg != nil {
		pg.Close()
	}
	log.Info("server stopped")
}
