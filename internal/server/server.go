package server

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // registers pprof routes on DefaultServeMux
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sanka/golang-web-application-analyzer/internal/handler"
	"github.com/sanka/golang-web-application-analyzer/internal/metrics"
)

// Server wraps the standard http.Server and holds shared infrastructure.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New wires up all routes and middleware, then returns a ready-to-use Server.
// Routes:
//
//	GET  /             → index form
//	POST /analyze      → analysis handler
//	GET  /metrics      → Prometheus metrics
//	GET  /debug/pprof/ → pprof (registered by blank import above)
func New(h *handler.Handler, logger *slog.Logger, port string) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", h.Index)
	mux.HandleFunc("POST /analyze", h.Analyze)
	mux.Handle("GET /metrics", promhttp.Handler())

	// pprof routes are registered on http.DefaultServeMux by the blank import.
	// We register each sub-path explicitly with GET so there is no ambiguity
	// with the method-specific patterns above (Go 1.22 mux requirement).
	for _, path := range []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
	} {
		mux.Handle("GET "+path, http.DefaultServeMux)
	}

	wrapped := loggingMiddleware(logger, metricsMiddleware(mux))

	return &Server{
		httpServer: &http.Server{
			Addr:         ":" + port,
			Handler:      wrapped,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 60 * time.Second, // generous: analysis can take time
			IdleTimeout:  120 * time.Second,
		},
		logger: logger,
	}
}

// ListenAndServe starts the HTTP server. It blocks until the server stops.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server with the given context.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// responseWriter wraps http.ResponseWriter to capture the status code so
// middleware can log and record it after the handler returns.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs every request with method, path, status, and duration.
func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// metricsMiddleware records Prometheus counters and histograms per request.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		statusStr := strconv.Itoa(rw.status)
		metrics.RequestsTotal.WithLabelValues(r.Method, r.URL.Path, statusStr).Inc()
		metrics.RequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(start).Seconds())
	})
}
