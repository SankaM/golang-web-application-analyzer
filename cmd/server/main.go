package main

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sanka/golang-web-application-analyzer/internal/handler"
	"github.com/sanka/golang-web-application-analyzer/internal/server"

	// Importing metrics triggers the prometheus.MustRegister calls in init().
	_ "github.com/sanka/golang-web-application-analyzer/internal/metrics"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Register the custom "list" function so result.html can iterate headings
	// in a fixed order (h1..h6) — Go maps have non-deterministic iteration.
	tmpl := template.Must(
		template.New("").Funcs(template.FuncMap{
			"list": func(args ...string) []string { return args },
		}).ParseGlob("web/templates/*.html"),
	)

	// The fetch client has a generous timeout for slow target pages.
	// Link-check requests use a separate shorter-timeout client created in
	// the analyzer so they don't block the whole analysis for too long.
	fetchClient := &http.Client{Timeout: 10 * time.Second}

	h := handler.New(tmpl, fetchClient, logger)
	srv := server.New(h, logger, port)

	// Run the server in a goroutine so we can listen for OS signals below.
	go func() {
		logger.Info("server starting", "port", port,
			"metrics", "http://localhost:"+port+"/metrics",
			"pprof", "http://localhost:"+port+"/debug/pprof/",
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Block until SIGINT or SIGTERM, then gracefully drain in-flight requests.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("server stopped")
}
