package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sanketn26/gossipcache/internal/cache"
	"github.com/sanketn26/gossipcache/internal/config"
	"github.com/sanketn26/gossipcache/internal/observability"
	"github.com/sanketn26/gossipcache/internal/storage/memory"
	"github.com/sanketn26/gossipcache/pkg/gossipcache"
)

func main() {
	configPath := flag.String("config", "", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger := observability.NewLogger(cfg.Logging.Level, cfg.Logging.Format)
	logger.Info("starting gossipcache", "node_id", cfg.NodeID, "mode", cfg.Mode)

	metrics := observability.NewMetrics(nil)

	registry := gossipcache.NewServiceRegistry[config.Config]()
	metricsService := observability.NewMetricsServiceWithLogger(metrics, logger.WithComponent("metrics_service"))
	if err := registry.Register("metrics", metricsService); err != nil {
		logger.Error("register metrics service", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := registry.Start(ctx, cfg); err != nil {
		logger.Error("start services", "error", err)
		os.Exit(1)
	}

	store, err := memory.New(memory.Options{
		MaxSize:        cfg.Cache.MaxSize,
		EvictionPolicy: cfg.Cache.EvictionPolicy,
		MaxKeySize:     cfg.Cache.MaxKeySize,
		MaxValueSize:   cfg.Cache.MaxValueSize,
	})
	if err != nil {
		logger.Error("create memory storage", "error", err)
		shutdown(ctx, registry, cfg, logger)
		os.Exit(1)
	}

	var cacheManager gossipcache.Cache = cache.NewManager(store, &cache.CacheConfig{
		DefaultTTL: cfg.Cache.DefaultTTL,
		Metrics:    metrics,
	})

	logger.Info("cache ready", "max_size", cfg.Cache.MaxSize, "default_ttl", cfg.Cache.DefaultTTL)

	waitForSignal()
	logger.Info("shutting down")

	if err := cacheManager.Close(); err != nil {
		logger.Error("close cache", "error", err)
	}
	shutdown(ctx, registry, cfg, logger)
}

func waitForSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
}

func shutdown(ctx context.Context, registry *gossipcache.ServiceRegistry[config.Config], cfg *config.Config, logger *observability.Logger) {
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := registry.Shutdown(shutdownCtx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("shutdown services", "error", err)
	}
}
