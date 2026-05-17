package observability

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/sanketn26/gossipcache/internal/config"
	"github.com/sanketn26/gossipcache/pkg/gossipcache"
)

var _ gossipcache.Service[config.Config] = (*MetricsService)(nil)

// MetricsService manages the optional Prometheus metrics endpoint lifecycle.
type MetricsService struct {
	mu      sync.Mutex
	metrics *Metrics
	logger  *Logger
	server  *http.Server
	address string
}

// NewMetricsService builds a metrics service with a default logger. Use
// NewMetricsServiceWithLogger to plug in the application logger.
func NewMetricsService(metrics *Metrics) *MetricsService {
	return NewMetricsServiceWithLogger(metrics, nil)
}

// NewMetricsServiceWithLogger builds a metrics service that surfaces serve
// errors through the supplied logger. A nil logger falls back to a default
// JSON error-level logger so failures still reach stdout.
func NewMetricsServiceWithLogger(metrics *Metrics, logger *Logger) *MetricsService {
	if logger == nil {
		logger = NewLogger("error", "json").WithComponent("metrics_service")
	}
	return &MetricsService{
		metrics: metrics,
		logger:  logger,
	}
}

func (s *MetricsService) Start(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	if !cfg.Metrics.Enabled {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return errors.New("metrics service already started")
	}
	if s.metrics == nil {
		s.metrics = NewMetrics(nil)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", s.metrics.Handler())

	server := &http.Server{
		Addr:    s.metrics.Address(cfg.Metrics.Port),
		Handler: mux,
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen on metrics address %q: %w", server.Addr, err)
	}

	s.server = server
	s.address = listener.Addr().String()

	logger := s.logger
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server serve failed", "error", err)
			_ = server.Close()
		}
	}()

	return nil
}

func (s *MetricsService) Shutdown(ctx context.Context, cfg *config.Config) error {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.address = ""
	s.mu.Unlock()

	if server == nil {
		return nil
	}

	if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown metrics server: %w", err)
	}
	return nil
}

func (s *MetricsService) Metrics() *Metrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *MetricsService) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.address
}
