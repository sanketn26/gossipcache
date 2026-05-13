package observability

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRecordsOperationsAndStats(t *testing.T) {
	metrics := NewMetrics(prometheus.NewRegistry())

	metrics.RecordCacheOperation("get", nil)
	metrics.RecordCacheOperation("get", errors.New("miss"))
	metrics.SetCacheStats(128, 3)

	body := scrapeMetrics(t, metrics)
	for _, want := range []string{
		`gossipcache_cache_operations_total{operation="get",result="success"} 1`,
		`gossipcache_cache_operations_total{operation="get",result="error"} 1`,
		"gossipcache_cache_size_bytes 128",
		"gossipcache_cache_keys 3",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}

func TestMetricsAddress(t *testing.T) {
	metrics := NewMetrics(prometheus.NewRegistry())
	if got := metrics.Address(9090); got != ":9090" {
		t.Fatalf("Address(9090) = %q, want :9090", got)
	}
}

func scrapeMetrics(t *testing.T, metrics *Metrics) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, req)

	resp := recorder.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics response: %v", err)
	}
	return string(body)
}
