package handler

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

type mockBrowserFetcher struct {
	html string
	err  error
}

func (m mockBrowserFetcher) FetchHTML(_ context.Context, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.html, nil
}

// buildHandler creates a Handler with real templates and the given HTTP client.
// The templates are parsed from inline strings so the test has no filesystem
// dependency — no real HTTP requests, no real files.
func buildHandler(t *testing.T, client *http.Client) *Handler {
	t.Helper()
	const indexTmpl = `{{ define "index.html" }}ERROR:{{ .Error }} URL:{{ .URL }}{{ end }}`
	const resultTmpl = `{{ define "result.html" }}TITLE:{{ .Result.Title }} VERSION:{{ .Result.HTMLVersion }}{{ end }}`

	tmpl := template.Must(
		template.New("").Funcs(template.FuncMap{
			"list": func(args ...string) []string { return args },
		}).Parse(indexTmpl + resultTmpl),
	)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return New(tmpl, client, logger)
}

// TestAnalyzeHandler_ValidURL verifies the happy path: a valid URL pointing to
// a mock server returns a 200 with the page title in the response body.
func TestAnalyzeHandler_ValidURL(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
		<html><head><title>Mock Page</title></head>
		<body><h1>Hello</h1></body></html>`)
	}))
	defer target.Close()

	h := buildHandler(t, target.Client())

	form := url.Values{"url": {target.URL}}
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.Analyze(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Mock Page") {
		t.Errorf("body does not contain page title; body = %q", rr.Body.String())
	}
}

// TestAnalyzeHandler_InvalidURL verifies that a malformed URL re-renders the
// index page with an error message — no panic, no 500.
func TestAnalyzeHandler_InvalidURL(t *testing.T) {
	h := buildHandler(t, &http.Client{})

	form := url.Values{"url": {"not-a-url"}}
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.Analyze(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "ERROR:") || strings.Contains(body, "ERROR: ") {
		// ERROR: should be followed by a non-empty message
		t.Errorf("expected non-empty error in body; got %q", body)
	}
}

// TestAnalyzeHandler_EmptyURL verifies that submitting an empty URL returns
// the "URL is required" error message.
func TestAnalyzeHandler_EmptyURL(t *testing.T) {
	h := buildHandler(t, &http.Client{})

	form := url.Values{"url": {""}}
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.Analyze(rr, req)

	if !strings.Contains(rr.Body.String(), "URL is required") {
		t.Errorf("expected 'URL is required' in body; got %q", rr.Body.String())
	}
}

// TestAnalyzeHandler_TargetReturns404 verifies that a 404 from the target
// page is surfaced as a user-facing error, not a server error.
func TestAnalyzeHandler_TargetReturns404(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer target.Close()

	h := buildHandler(t, target.Client())

	form := url.Values{"url": {target.URL}}
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.Analyze(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "404") {
		t.Errorf("expected '404' in error body; got %q", rr.Body.String())
	}
}

func TestAnalyzeHandler_BrowserFallbackSucceeds(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("cf-mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer target.Close()

	h := buildHandler(t, target.Client())
	h.browserFetcher = mockBrowserFetcher{
		html: `<!DOCTYPE html><html><head><title>From Browser</title></head><body><h1>Ok</h1></body></html>`,
	}

	form := url.Values{"url": {target.URL}}
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.Analyze(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "From Browser") {
		t.Errorf("expected browser fallback title in body; got %q", rr.Body.String())
	}
}

func TestAnalyzeHandler_BrowserFallbackFailureKeepsHTTPError(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("cf-mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer target.Close()

	h := buildHandler(t, target.Client())
	h.browserFetcher = mockBrowserFetcher{
		err: fmt.Errorf("browser not available"),
	}

	form := url.Values{"url": {target.URL}}
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.Analyze(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "403") {
		t.Errorf("expected 403 in body; got %q", rr.Body.String())
	}
}

// TestIndex verifies the index handler renders without error.
func TestIndex(t *testing.T) {
	h := buildHandler(t, &http.Client{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.Index(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// ── validateURL unit tests ────────────────────────────────────────────────────

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid_https", "https://example.com", false},
		{"valid_http", "http://example.com/path?q=1", false},
		{"empty", "", true},
		{"no_scheme", "example.com", true},
		{"ftp_scheme", "ftp://example.com", true},
		{"spaces", "https://exam ple.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestShouldAttemptBrowserFallback(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		headers    map[string]string
		want       bool
	}{
		{
			name:       "cloudflare_403",
			statusCode: http.StatusForbidden,
			headers: map[string]string{
				"cf-mitigated": "challenge",
				"server":       "cloudflare",
			},
			want: true,
		},
		{
			name:       "too_many_requests",
			statusCode: http.StatusTooManyRequests,
			headers:    map[string]string{},
			want:       true,
		},
		{
			name:       "forbidden_without_hints",
			statusCode: http.StatusForbidden,
			headers:    map[string]string{},
			want:       false,
		},
		{
			name:       "normal_404",
			statusCode: http.StatusNotFound,
			headers:    map[string]string{},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     make(http.Header),
			}
			for key, value := range tt.headers {
				resp.Header.Set(key, value)
			}

			got := shouldAttemptBrowserFallback(resp)
			if got != tt.want {
				t.Errorf("shouldAttemptBrowserFallback() = %v, want %v", got, tt.want)
			}
		})
	}
}
