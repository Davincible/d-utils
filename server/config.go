package server

import (
	"net/http"
	"time"

	"github.com/go-chi/cors"
)

// ServerHTTP configuration fields
type ServerConfig struct {
	// addr is the address the server will listen on (e.g. ":8080")
	addr string

	// readTimeout is the maximum duration for reading the entire request
	readTimeout time.Duration

	// writeTimeout is the maximum duration before timing out writes of the response
	writeTimeout time.Duration

	// maxRequestSize is the maximum size in bytes for the request body
	maxRequestSize int64

	// timeoutDuration is the maximum duration before timing out the entire request
	timeoutDuration time.Duration

	// corsOptions configures Cross-Origin Resource Sharing settings
	corsOptions *cors.Options

	// enableMetrics determines if Prometheus metrics are enabled
	enableMetrics bool

	// enableLogger determines if request logging is enabled
	enableLogger bool

	// customMiddleware contains additional middleware to be added to the server
	customMiddleware []func(http.Handler) http.Handler

	// enableProfiling enables pprof debugging endpoints
	enableProfiling bool

	// enableGzip enables gzip compression for responses
	enableGzip bool

	// enableBrotli enables brotli compression for responses
	enableBrotli bool

	// brotliLevel sets the compression level for brotli (1-11, default: 4)
	brotliLevel int

	// enableHealthCheck enables the health check endpoint
	enableHealthCheck bool

	// healthCheckPath customizes the health check endpoint path (default: /health)
	healthCheckPath string

	// shutdownTimeout is the maximum duration to wait for server shutdown
	shutdownTimeout time.Duration

	// idleTimeout is the maximum amount of time to wait for the next request
	idleTimeout time.Duration

	// keepAliveTimeout is the duration to keep connections alive
	keepAliveTimeout time.Duration

	// maxHeaderBytes controls the maximum number of bytes the server will read parsing the request header
	maxHeaderBytes int
}

// ServerOption defines a functional option for configuring the server
type ServerOption func(*ServerConfig)

// Default configuration values
const (
	defaultAddr             = ":8080"
	defaultReadTimeout      = 5 * time.Second
	defaultWriteTimeout     = 5 * time.Second
	defaultMaxRequestSize   = 10 << 20 // 10 MB
	defaultTimeout          = 60 * time.Second
	defaultShutdownTimeout  = 30 * time.Second
	defaultIdleTimeout      = 120 * time.Second
	defaultKeepAliveTimeout = 60 * time.Second
	defaultMaxHeaderBytes   = 1 << 20 // 1 MB
	defaultHealthCheckPath  = "/health"
	defaultBrotliLevel      = 4 // Default Brotli compression level
)

// WithAddr sets the server address
func WithAddr(addr string) ServerOption {
	return func(c *ServerConfig) {
		c.addr = addr
	}
}

// WithTimeouts sets the read and write timeouts
func WithTimeouts(read, write time.Duration) ServerOption {
	return func(c *ServerConfig) {
		c.readTimeout = read
		c.writeTimeout = write
	}
}

// WithRequestTimeout sets the global request timeout
func WithRequestTimeout(timeout time.Duration) ServerOption {
	return func(c *ServerConfig) {
		c.timeoutDuration = timeout
	}
}

// WithMaxRequestSize sets the maximum request size
func WithMaxRequestSize(size int64) ServerOption {
	return func(c *ServerConfig) {
		c.maxRequestSize = size
	}
}

// WithCORS sets the CORS options
func WithCORS(options *cors.Options) ServerOption {
	return func(c *ServerConfig) {
		c.corsOptions = options
	}
}

// WithMetrics enables Prometheus metrics
func WithMetrics() ServerOption {
	return func(c *ServerConfig) {
		c.enableMetrics = true
	}
}

// WithLogger enables the chi logger middleware
func WithLogger() ServerOption {
	return func(c *ServerConfig) {
		c.enableLogger = true
	}
}

// WithMiddleware adds custom middleware
func WithMiddleware(middleware ...func(http.Handler) http.Handler) ServerOption {
	return func(c *ServerConfig) {
		c.customMiddleware = append(c.customMiddleware, middleware...)
	}
}

// WithCorsDomains sets the allowed CORS domains
func WithCorsDomains(domains []string) ServerOption {
	return func(c *ServerConfig) {
		if c.corsOptions == nil {
			c.corsOptions = &cors.Options{}
		}
		c.corsOptions.AllowedOrigins = domains
	}
}

// WithCorsMethods sets the allowed CORS methods
func WithCorsMethods(methods []string) ServerOption {
	return func(c *ServerConfig) {
		if c.corsOptions == nil {
			c.corsOptions = &cors.Options{}
		}
		c.corsOptions.AllowedMethods = methods
	}
}

// WithCorsHeaders sets the allowed CORS headers
func WithCorsHeaders(headers []string) ServerOption {
	return func(c *ServerConfig) {
		if c.corsOptions == nil {
			c.corsOptions = &cors.Options{}
		}
		c.corsOptions.AllowedHeaders = headers
	}
}

// WithCorsCredentials enables CORS credentials
func WithCorsCredentials() ServerOption {
	return func(c *ServerConfig) {
		if c.corsOptions == nil {
			c.corsOptions = &cors.Options{}
		}
		c.corsOptions.AllowCredentials = true
	}
}

// WithProfiling enables pprof debugging endpoints
func WithProfiling() ServerOption {
	return func(c *ServerConfig) {
		c.enableProfiling = true
	}
}

// WithGzip enables gzip compression
func WithGzip() ServerOption {
	return func(c *ServerConfig) {
		c.enableGzip = true
	}
}

// WithBrotli enables brotli compression with optional compression level
func WithBrotli(level ...int) ServerOption {
	return func(c *ServerConfig) {
		c.enableBrotli = true
		if len(level) > 0 {
			// Ensure level is between 1 and 11
			if level[0] < 1 {
				level[0] = 1
			} else if level[0] > 11 {
				level[0] = 11
			}
			c.brotliLevel = level[0]
		} else {
			c.brotliLevel = defaultBrotliLevel
		}
	}
}

// WithHealthCheck configures the health check endpoint with optional custom path
func WithHealthCheck(path ...string) ServerOption {
	return func(c *ServerConfig) {
		c.enableHealthCheck = true
		if len(path) > 0 && path[0] != "" {
			c.healthCheckPath = path[0]
		}
	}
}

// WithShutdownTimeout sets the maximum duration to wait for server shutdown
func WithShutdownTimeout(timeout time.Duration) ServerOption {
	return func(c *ServerConfig) {
		c.shutdownTimeout = timeout
	}
}

// WithIdleTimeout sets the maximum duration to wait for the next request
func WithIdleTimeout(timeout time.Duration) ServerOption {
	return func(c *ServerConfig) {
		c.idleTimeout = timeout
	}
}

// WithKeepAliveTimeout sets the duration to keep connections alive
func WithKeepAliveTimeout(timeout time.Duration) ServerOption {
	return func(c *ServerConfig) {
		c.keepAliveTimeout = timeout
	}
}

// WithMaxHeaderBytes sets the maximum number of bytes for request headers
func WithMaxHeaderBytes(bytes int) ServerOption {
	return func(c *ServerConfig) {
		c.maxHeaderBytes = bytes
	}
}
