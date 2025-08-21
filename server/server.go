package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
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
		addr:              defaultAddr,
		readTimeout:       defaultReadTimeout,
		writeTimeout:      defaultWriteTimeout,
		maxRequestSize:    defaultMaxRequestSize,
		timeoutDuration:   defaultTimeout,
		enableMetrics:     true,
		enableLogger:      true,
		enableHealthCheck: true,
		healthCheckPath:   defaultHealthCheckPath,
		shutdownTimeout:   defaultShutdownTimeout,
		idleTimeout:       defaultIdleTimeout,
		keepAliveTimeout:  defaultKeepAliveTimeout,
		maxHeaderBytes:    defaultMaxHeaderBytes,
		brotliLevel:       defaultBrotliLevel,
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
		IdleTimeout:       config.idleTimeout,
		MaxHeaderBytes:    config.maxHeaderBytes,
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

	// Add compression middleware
	if s.config.enableBrotli {
		baseMiddleware = append(baseMiddleware, s.brotliMiddleware)
	} else if s.config.enableGzip {
		baseMiddleware = append(baseMiddleware, middleware.Compress(5))
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
	if s.config.enableHealthCheck {
		s.router.Get(s.config.healthCheckPath, func(w http.ResponseWriter, r *http.Request) {
			health := map[string]interface{}{
				"status":     "OK",
				"timestamp":  time.Now().UTC(),
				"goroutines": runtime.NumGoroutine(),
			}

			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			health["memory"] = map[string]interface{}{
				"alloc":      m.Alloc,
				"totalAlloc": m.TotalAlloc,
				"sys":        m.Sys,
				"numGC":      m.NumGC,
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(health)
		})
	}

	// Setup pprof endpoints
	if s.config.enableProfiling {
		s.router.Mount("/debug/pprof", http.HandlerFunc(pprof.Index))
		s.router.Get("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		s.router.Get("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		s.router.Get("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		s.router.Get("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
		s.router.Get("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
		s.router.Get("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
		s.router.Get("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
		s.router.Get("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
	}
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
	if s.config.shutdownTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.shutdownTimeout)
		defer cancel()
	}
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

// brotliMiddleware implements Brotli compression
func (s *ServerHTTP) brotliMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts brotli
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "br") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip compression for small responses or certain content types
		if r.Header.Get("Range") != "" || r.Header.Get("Content-Encoding") != "" {
			next.ServeHTTP(w, r)
			return
		}

		// Create brotli writer
		w.Header().Set("Content-Encoding", "br")
		w.Header().Del("Content-Length")
		w.Header().Add("Vary", "Accept-Encoding")

		bw := &brotliResponseWriter{
			ResponseWriter: w,
			writer:         brotli.NewWriterLevel(w, s.config.brotliLevel),
		}
		defer bw.Close()

		next.ServeHTTP(bw, r)
	})
}

// brotliResponseWriter wraps http.ResponseWriter to provide Brotli compression
type brotliResponseWriter struct {
	http.ResponseWriter
	writer *brotli.Writer
}

func (w *brotliResponseWriter) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

func (w *brotliResponseWriter) Close() error {
	return w.writer.Close()
}

func (w *brotliResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
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
