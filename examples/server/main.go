//go:build example

// Package main is an example binary that runs a local in-memory L1 cache with
// the Prometheus metrics endpoint enabled. It is excluded from the default
// build so library consumers do not get a binary they did not ask for.
//
//	go build -tags example -o bin/gossipcache-example ./examples/server
//	go run -tags example ./examples/server -config config.yaml
//
// This is not a full hybrid hub + multi-L1 demo (see docs/impl/PHASE_07_DEMO_POLISH.md).
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

	"github.com/sanketn26/gossipcache/internal/config"
	"github.com/sanketn26/gossipcache/internal/observability"
	"github.com/sanketn26/gossipcache/pkg/gossipcache"
	"github.com/sanketn26/gossipcache/pkg/gossipcache/inmemory"
)

func main() {
	configPath := flag.String("config", "", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger := observability.NewLogger(cfg.Logging.Level, cfg.Logging.Format)
	logger.Info("starting gossipcache local L1 example", "node_id", cfg.NodeID)

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

	cache, err := inmemory.New(inmemory.Options{
		MaxSize:        cfg.Cache.MaxSize,
		DefaultTTL:     cfg.Cache.DefaultTTL,
		EvictionPolicy: cfg.Cache.EvictionPolicy,
		MaxKeySize:     cfg.Cache.MaxKeySize,
		MaxValueSize:   cfg.Cache.MaxValueSize,
		Metrics:        metrics,
	})
	if err != nil {
		logger.Error("create cache", "error", err)
		shutdown(ctx, registry, cfg, logger)
		os.Exit(1)
	}

	logger.Info("cache ready", "max_size", cfg.Cache.MaxSize, "default_ttl", cfg.Cache.DefaultTTL)

	waitForSignal()
	logger.Info("shutting down")

	if err := cache.Close(); err != nil {
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
