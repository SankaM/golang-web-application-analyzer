package server

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // registers pprof routes on DefaultServeMux
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sanka/golang-web-application-analyzer/internal/handler"
)

// Server wraps http.Server and holds shared infrastructure.
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

