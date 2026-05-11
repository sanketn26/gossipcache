package observability

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sanketn26/gossipcache/internal/config"
)

func TestMetricsServiceStartServesMetricsEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.Metrics.Port = 0

	service := NewMetricsService(nil)
	if err := service.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := service.Shutdown(ctx, cfg); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	}()

	if service.Metrics() == nil {
		t.Fatal("Metrics() returned nil after Start")
	}

	body := getMetricsBody(t, service.Address())
	if !strings.Contains(body, "gossipcache_cache_size_bytes") {
		t.Fatalf("metrics body does not contain gossipcache metric names:\n%s", body)
	}
}

func TestMetricsServiceStartDoesNothingWhenDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Metrics.Enabled = false

	service := NewMetricsService(nil)
	if err := service.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if service.Metrics() != nil {
		t.Fatal("Metrics() returned non-nil when metrics are disabled")
	}
	if service.Address() != "" {
		t.Fatalf("Address() = %q, want empty", service.Address())
	}
	if err := service.Shutdown(context.Background(), cfg); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestMetricsServiceRejectsDoubleStart(t *testing.T) {
	cfg := config.Default()
	cfg.Metrics.Port = 0

	service := NewMetricsService(NewMetrics(nil))
	if err := service.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer service.Shutdown(context.Background(), cfg)

	err := service.Start(context.Background(), cfg)
	if err == nil {
		t.Fatal("second Start succeeded, want error")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Fatalf("second Start error = %q, want already started", err.Error())
	}
}

func getMetricsBody(t *testing.T, address string) string {
	t.Helper()

	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split metrics address %q: %v", address, err)
	}

	client := http.Client{Timeout: time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	return string(body)
}
