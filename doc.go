// Package dutils provides a comprehensive collection of utility packages for building
// production-ready Go applications with focus on caching, concurrency, and HTTP services.
//
// This module contains the following packages:
//
// CACHING & STORAGE:
//
//   - cache: Generic type-safe in-memory caching with TTL, file persistence (GOB),
//     NATS JetStream KeyValue integration, and automatic cleanup routines
//   - apicache: HTTP API response caching with circuit breaker pattern, request
//     coalescing, LRU eviction, stale-while-revalidate, and Prometheus metrics
//   - tscache: Time-series data caching system with automatic partitioning,
//     persistent storage, range queries, and comprehensive statistics
//
// CONCURRENCY & SAFETY:
//
//   - safemap: Thread-safe concurrent data structures with generic types,
//     providing lock-free operations for high-throughput scenarios
//
// HTTP SERVER & MIDDLEWARE:
//
//   - server: Production-ready HTTP server built on chi router with CORS,
//     Brotli/Gzip compression, request timeouts, health checks with memory stats,
//     Prometheus metrics, pprof profiling endpoints, and graceful shutdown
//
// UTILITIES & HELPERS:
//
//   - completion: Command-line completion utilities for CLI applications
//   - debug: Development and debugging utilities with runtime introspection
//   - jwt: JSON Web Token creation and validation with HS256 signing,
//     Bearer/Token header extraction, WebSocket protocol support, and custom claims
//   - limiter: Rate limiting utilities with token bucket and sliding window algorithms
//   - taskrunner: Concurrent task execution with worker pools, priority queues,
//     and error handling strategies
//   - utils: General purpose utilities including duration parsing, string manipulation,
//     and common data transformations
//
// All packages are designed with production use in mind, featuring comprehensive
// error handling, observability through metrics and logging, and can be used
// independently or composed together to build scalable, maintainable applications.
package dutils