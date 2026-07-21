package observability

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "gossipcache"

// Metrics owns L1 cache Prometheus collectors.
// Later phases (P5) extend this with stream, L2, and readiness instruments.
type Metrics struct {
	registry *prometheus.Registry

	cacheOperations *prometheus.CounterVec
	cacheSizeBytes  prometheus.Gauge
	cacheKeys       prometheus.Gauge
	cacheHits       *prometheus.CounterVec
	cacheMisses     *prometheus.CounterVec
}

func NewMetrics(registry *prometheus.Registry) *Metrics {
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

	m := &Metrics{
		registry: registry,
		cacheOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_operations_total",
				Help:      "Total cache operations by operation and result.",
			},
			[]string{"operation", "result"},
		),
		cacheSizeBytes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "cache_size_bytes",
				Help:      "Current cache size in bytes.",
			},
		),
		cacheKeys: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "cache_keys",
				Help:      "Current number of cache keys.",
			},
		),
		cacheHits: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_hits_total",
				Help:      "Total cache hits.",
			},
			[]string{"operation"},
		),
		cacheMisses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_misses_total",
				Help:      "Total cache misses.",
			},
			[]string{"operation"},
		),
	}

	registry.MustRegister(m.cacheOperations, m.cacheSizeBytes, m.cacheKeys, m.cacheHits, m.cacheMisses)
	return m
}

func (m *Metrics) RecordCacheOperation(operation string, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}
	m.cacheOperations.WithLabelValues(operation, result).Inc()
}

func (m *Metrics) SetCacheStats(sizeBytes, keys int64) {
	m.cacheSizeBytes.Set(float64(sizeBytes))
	m.cacheKeys.Set(float64(keys))
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Address(port int) string {
	return ":" + strconv.Itoa(port)
}
