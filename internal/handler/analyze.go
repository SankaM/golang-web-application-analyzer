package handler

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sanka/golang-web-application-analyzer/internal/analyzer"
	"github.com/sanka/golang-web-application-analyzer/internal/browserfetch"
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
	templates      *template.Template
	client         *http.Client
	logger         *slog.Logger
	browserFetcher browserFetcher
}

type browserFetcher interface {
	FetchHTML(ctx context.Context, targetURL string) (string, error)
}

// New constructs a Handler. Dependencies are injected so they can be mocked
// in tests without touching global state.
func New(templates *template.Template, client *http.Client, logger *slog.Logger) *Handler {
	return &Handler{
		templates:      templates,
		client:         client,
		logger:         logger,
		browserFetcher: browserfetch.New(15 * time.Second),
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
	setBrowserHeaders(req)
	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Error("fetch failed", "url", rawURL, "error", err)
		h.renderIndex(w, IndexData{
			Error: fmt.Sprintf("URL is not reachable: %s", err.Error()),
			URL:   rawURL,
		})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	bodyReader := io.Reader(resp.Body)
	usedBrowserFallback := false
	if shouldAttemptBrowserFallback(resp) {
		htmlContent, fallbackErr := h.browserFetcher.FetchHTML(r.Context(), rawURL)
		if fallbackErr == nil {
			h.logger.Info("browser fallback succeeded", "url", rawURL, "status", resp.StatusCode)
			bodyReader = strings.NewReader(htmlContent)
			usedBrowserFallback = true
		} else {
			h.logger.Warn("browser fallback failed", "url", rawURL, "status", resp.StatusCode, "error", fallbackErr)
		}
	}

	if resp.StatusCode >= http.StatusBadRequest && !usedBrowserFallback {
		h.logger.Warn("target returned error status", "url", rawURL, "status", resp.StatusCode)
		h.renderIndex(w, IndexData{
			Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
			URL:   rawURL,
		})
		return
	}

	result, err := analyzer.Analyze(r.Context(), rawURL, bodyReader, h.client)
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

// setBrowserHeaders sets common browser-like headers on r so that servers
// using basic bot-detection (User-Agent checks, Accept sniffing) don't
// reject the request before we can read the page.
func setBrowserHeaders(r *http.Request) {
	r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
}

func shouldAttemptBrowserFallback(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusBadRequest {
		// Some heavily protected targets (for example Meta properties) return
		// 400 to non-browser clients. Treat that as a candidate for browser
		// fallback when anti-bot headers are present.
		server := strings.ToLower(resp.Header.Get("server"))
		if resp.Header.Get("x-fb-debug") != "" || strings.Contains(server, "proxygen") {
			return true
		}
	}
	if resp.StatusCode != http.StatusForbidden &&
		resp.StatusCode != http.StatusBadRequest &&
		resp.StatusCode != http.StatusTooManyRequests &&
		resp.StatusCode != http.StatusServiceUnavailable {
		return false
	}

	challengeHints := []string{
		resp.Header.Get("cf-mitigated"),
		resp.Header.Get("server"),
		resp.Header.Get("x-sucuri-id"),
	}
	for _, hint := range challengeHints {
		lower := strings.ToLower(hint)
		if strings.Contains(lower, "challenge") || strings.Contains(lower, "cloudflare") || strings.Contains(lower, "sucuri") {
			return true
		}
	}

	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable
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
