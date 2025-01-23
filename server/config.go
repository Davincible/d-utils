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
}

// ServerOption defines a functional option for configuring the server
type ServerOption func(*ServerConfig)

// Default configuration values
const (
	defaultAddr           = ":8080"
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = 5 * time.Second
	defaultMaxRequestSize = 10 << 20 // 10 MB
	defaultTimeout        = 60 * time.Second
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

// WithMetrics enables or disables Prometheus metrics
func WithMetrics(enable bool) ServerOption {
	return func(c *ServerConfig) {
		c.enableMetrics = enable
	}
}

// WithLogger enables or disables the chi logger middleware
func WithLogger(enable bool) ServerOption {
	return func(c *ServerConfig) {
		c.enableLogger = enable
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

// WithCorsCredentials sets whether CORS requests can include credentials
func WithCorsCredentials(allow bool) ServerOption {
	return func(c *ServerConfig) {
		if c.corsOptions == nil {
			c.corsOptions = &cors.Options{}
		}
		c.corsOptions.AllowCredentials = allow
	}
}
