package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServerHTTP represents the HTTP server instance
type ServerHTTP struct {
	server  http.Server
	logger  *slog.Logger
	metrics *Metrics
	router  *chi.Mux
	config  *ServerConfig
}

// Metrics holds all Prometheus metrics
type Metrics struct {
	TotalRequests  *prometheus.CounterVec
	ResponseStatus *prometheus.CounterVec
	HTTPDuration   *prometheus.HistogramVec
}

// NewMetrics initializes and registers Prometheus metrics
func NewMetrics(namespace string) *Metrics {
	m := &Metrics{
		TotalRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"path"},
		),
		ResponseStatus: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_response_status",
				Help:      "HTTP response status codes",
			},
			[]string{"status"},
		),
		HTTPDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request latency",
			},
			[]string{"path"},
		),
	}

	prometheus.MustRegister(m.TotalRequests, m.ResponseStatus, m.HTTPDuration)
	return m
}

// Add after NewServer but before setupMiddleware
func validateConfig(config *ServerConfig) error {
	if config.addr == "" {
		return errors.New("server address cannot be empty")
	}
	if config.readTimeout <= 0 {
		return errors.New("read timeout must be positive")
	}
	if config.writeTimeout <= 0 {
		return errors.New("write timeout must be positive")
	}
	if config.maxRequestSize <= 0 {
		return errors.New("max request size must be positive")
	}
	if config.timeoutDuration <= 0 {
		return errors.New("timeout duration must be positive")
	}
	return nil
}

// NewServer creates a new HTTP server instance
func NewServer(logger *slog.Logger, opts ...ServerOption) (*ServerHTTP, error) {
	if logger == nil {
		return nil, errors.New("logger is required")
	}

	// Initialize default configuration
	config := &ServerConfig{
		addr:            defaultAddr,
		readTimeout:     defaultReadTimeout,
		writeTimeout:    defaultWriteTimeout,
		maxRequestSize:  defaultMaxRequestSize,
		timeoutDuration: defaultTimeout,
		enableMetrics:   true,
		enableLogger:    true,
		corsOptions: &cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"*"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: true,
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	s := &ServerHTTP{
		logger: logger,
		router: chi.NewRouter(),
		config: config,
	}

	// Initialize metrics if enabled
	if config.enableMetrics {
		s.metrics = NewMetrics("http_server")
	}

	// Configure the HTTP server
	s.server = http.Server{
		Addr:              config.addr,
		Handler:           s.router,
		ReadTimeout:       config.readTimeout,
		WriteTimeout:      config.writeTimeout,
		ReadHeaderTimeout: 2 * time.Second,
	}

	// Setup middleware and routes
	s.setupMiddleware()
	s.setupDefaultRoutes()

	return s, nil
}

// Router returns the chi router instance for adding custom routes
func (s *ServerHTTP) Router() *chi.Mux {
	return s.router
}

func (s *ServerHTTP) setupMiddleware() {
	baseMiddleware := []func(http.Handler) http.Handler{
		middleware.RequestID,
		middleware.RealIP,
		middleware.Recoverer,
		middleware.Timeout(s.config.timeoutDuration),
		middleware.RequestSize(s.config.maxRequestSize),
		s.trimSuffix,
	}

	if s.config.enableLogger {
		baseMiddleware = append(baseMiddleware, middleware.Logger)
	}

	if s.config.enableMetrics {
		baseMiddleware = append(baseMiddleware, s.prometheusMiddleware)
	}

	if s.config.corsOptions != nil {
		baseMiddleware = append(baseMiddleware, cors.Handler(*s.config.corsOptions))
	}

	middleware := append(baseMiddleware, s.config.customMiddleware...)
	s.router.Use(middleware...)
}

func (s *ServerHTTP) setupDefaultRoutes() {
	if s.config.enableMetrics {
		s.router.Mount("/metrics", promhttp.Handler())
	}

	// Health check endpoint
	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
}

// Start begins listening for requests
func (s *ServerHTTP) Start() error {
	l, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("start HTTP server: %w", err)
	}

	go func() {
		s.logger.Info("starting HTTP server", slog.String("addr", s.server.Addr))
		if err := s.server.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("server error", err)
		}
	}()

	return nil
}

// Shutdown gracefully stops the server
func (s *ServerHTTP) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Middleware helpers
func (s *ServerHTTP) trimSuffix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "//")
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
		next.ServeHTTP(w, r)
	})
}

func (s *ServerHTTP) prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timer := prometheus.NewTimer(s.metrics.HTTPDuration.WithLabelValues(r.URL.Path))
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)

		statusCode := rw.getStatus()
		s.metrics.ResponseStatus.WithLabelValues(strconv.Itoa(statusCode)).Inc()
		s.metrics.TotalRequests.WithLabelValues(r.URL.Path).Inc()
		timer.ObserveDuration()
	})
}

// ResponseWriter wrapper for tracking status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) getStatus() int {
	return rw.statusCode
}

// Addr returns the server's address
func (s *ServerHTTP) Addr() string {
	return s.server.Addr
}

// IsMetricsEnabled returns whether metrics are enabled
func (s *ServerHTTP) IsMetricsEnabled() bool {
	return s.config.enableMetrics
}

// IsLoggerEnabled returns whether logging is enabled
func (s *ServerHTTP) IsLoggerEnabled() bool {
	return s.config.enableLogger
}
