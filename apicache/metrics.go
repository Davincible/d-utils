package apicache

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PrometheusMetrics holds all Prometheus metrics for the cache
type PrometheusMetrics struct {
	hits            *prometheus.CounterVec
	misses          *prometheus.CounterVec
	errors          *prometheus.CounterVec
	inFlight        *prometheus.GaugeVec
	itemCount       *prometheus.GaugeVec
	fetchDuration   *prometheus.HistogramVec
	staleServes     *prometheus.CounterVec
	evictions       *prometheus.CounterVec
	circuitBreakers *prometheus.GaugeVec
}

// NewPrometheusMetrics creates and registers Prometheus metrics
func NewPrometheusMetrics(namespace string) *PrometheusMetrics {
	m := &PrometheusMetrics{
		hits: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_hits_total",
				Help:      "Number of cache hits",
			},
			[]string{"cache"},
		),
		misses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_misses_total",
				Help:      "Number of cache misses",
			},
			[]string{"cache"},
		),
		errors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_errors_total",
				Help:      "Number of cache errors",
			},
			[]string{"cache", "type"},
		),
		inFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "cache_inflight_requests",
				Help:      "Number of in-flight cache requests",
			},
			[]string{"cache"},
		),
		itemCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "cache_items",
				Help:      "Number of items in cache",
			},
			[]string{"cache"},
		),
		fetchDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "cache_fetch_duration_seconds",
				Help:      "Duration of fetch operations",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
			},
			[]string{"cache"},
		),
		staleServes: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_stale_serves_total",
				Help:      "Number of stale cache serves",
			},
			[]string{"cache"},
		),
		evictions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_evictions_total",
				Help:      "Number of cache evictions",
			},
			[]string{"cache"},
		),
		circuitBreakers: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "cache_circuit_breaker_state",
				Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
			},
			[]string{"cache"},
		),
	}

	// Register all metrics
	prometheus.MustRegister(
		m.hits,
		m.misses,
		m.errors,
		m.inFlight,
		m.itemCount,
		m.fetchDuration,
		m.staleServes,
		m.evictions,
		m.circuitBreakers,
	)

	return m
}
