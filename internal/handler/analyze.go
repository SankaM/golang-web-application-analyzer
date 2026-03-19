package handler

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/sanka/golang-web-application-analyzer/internal/analyzer"
	"github.com/sanka/golang-web-application-analyzer/internal/metrics"
)

// urlRegex validates that the input looks like an http/https URL before we
// even attempt to parse or fetch it. Using stdlib regexp as required.
var urlRegex = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)

// IndexData is the template context for the index page.
type IndexData struct {
	Error string
	URL   string
}

// ResultData is the template context for the results page.
type ResultData struct {
	Result   *analyzer.AnalysisResult
	Duration string
}

// Handler holds the shared dependencies for all HTTP handlers.
type Handler struct {
	templates *template.Template
	client    *http.Client
	logger    *slog.Logger
}

// New constructs a Handler. Dependencies are injected so they can be mocked
// in tests without touching global state.
func New(templates *template.Template, client *http.Client, logger *slog.Logger) *Handler {
	return &Handler{
		templates: templates,
		client:    client,
		logger:    logger,
	}
}

// Index serves the URL input form.
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	h.renderIndex(w, IndexData{})
}

// Analyze handles form submission: validates the URL, fetches the page,
// runs analysis, and renders the results or an error message.
func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderIndex(w, IndexData{Error: "Failed to parse form input."})
		return
	}

	rawURL := r.FormValue("url")

	if err := validateURL(rawURL); err != nil {
		h.renderIndex(w, IndexData{Error: err.Error(), URL: rawURL})
		return
	}

	start := time.Now()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		h.renderIndex(w, IndexData{Error: fmt.Sprintf("Invalid URL: %s", err.Error()), URL: rawURL})
		return
	}
	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Error("fetch failed", "url", rawURL, "error", err)
		h.renderIndex(w, IndexData{
			Error: fmt.Sprintf("URL is not reachable: %s", err.Error()),
			URL:   rawURL,
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		h.logger.Warn("target returned error status", "url", rawURL, "status", resp.StatusCode)
		h.renderIndex(w, IndexData{
			Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
			URL:   rawURL,
		})
		return
	}

	result, err := analyzer.Analyze(r.Context(), rawURL, resp.Body, h.client)
	if err != nil {
		h.logger.Error("analysis failed", "url", rawURL, "error", err)
		h.renderIndex(w, IndexData{
			Error: "Failed to parse page content.",
			URL:   rawURL,
		})
		return
	}

	duration := time.Since(start).Round(time.Millisecond).String()

	if result.InaccessibleCount > 0 {
		h.logger.Warn("inaccessible links found", "url", rawURL, "count", result.InaccessibleCount)
	}

	metrics.AnalyzedURLsTotal.Inc()

	if err := h.templates.ExecuteTemplate(w, "result.html", ResultData{
		Result:   result,
		Duration: duration,
	}); err != nil {
		h.logger.Error("template render failed", "template", "result.html", "error", err)
	}
}

// renderIndex is a helper that renders the index template with the given data.
// It is used for both the initial GET and any error path on POST.
func (h *Handler) renderIndex(w http.ResponseWriter, data IndexData) {
	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		h.logger.Error("template render failed", "template", "index.html", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// validateURL applies two layers of validation:
//  1. A regexp check to catch obviously malformed input quickly.
//  2. url.ParseRequestURI for structural correctness.
func validateURL(raw string) error {
	if raw == "" {
		return errors.New("URL is required")
	}
	if !urlRegex.MatchString(raw) {
		return fmt.Errorf("invalid URL format: %q", raw)
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https, got %q", parsed.Scheme)
	}
	return nil
}
