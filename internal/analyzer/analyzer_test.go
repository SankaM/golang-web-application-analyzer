package analyzer

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// ── HTML Version ─────────────────────────────────────────────────────────────

func TestDetectHTMLVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"html5_lowercase", "<!doctype html><html></html>", "HTML5"},
		{"html5_uppercase", "<!DOCTYPE HTML><html></html>", "HTML5"},
		{
			"html4_strict",
			`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd">`,
			"HTML 4.01 Strict",
		},
		{
			"html4_transitional",
			`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">`,
			"HTML 4.01 Transitional",
		},
		{
			"html4_frameset",
			`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Frameset//EN">`,
			"HTML 4.01 Frameset",
		},
		{
			"xhtml_10_strict",
			`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">`,
			"XHTML 1.0 Strict",
		},
		{
			"xhtml_10_transitional",
			`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN">`,
			"XHTML 1.0 Transitional",
		},
		{
			"xhtml_11",
			`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">`,
			"XHTML 1.1",
		},
		{"no_doctype", "<html><body></body></html>", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectHTMLVersion(strings.NewReader(tt.input))
			if got != tt.expected {
				t.Errorf("DetectHTMLVersion() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ── Page Title ────────────────────────────────────────────────────────────────

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"has_title",
			"<html><head><title>Hello World</title></head><body></body></html>",
			"Hello World",
		},
		{
			"no_title",
			"<html><head></head><body></body></html>",
			"",
		},
		{
			"empty_title",
			"<html><head><title></title></head></html>",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustParse(t, tt.input)
			got := ExtractTitle(doc)
			if got != tt.expected {
				t.Errorf("ExtractTitle() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ── Headings ──────────────────────────────────────────────────────────────────

func TestCountHeadings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]int
	}{
		{
			"multiple_levels",
			"<html><body><h1>A</h1><h2>B</h2><h2>C</h2><h3>D</h3></body></html>",
			map[string]int{"h1": 1, "h2": 2, "h3": 1},
		},
		{
			"no_headings",
			"<html><body><p>text</p></body></html>",
			map[string]int{},
		},
		{
			"all_levels",
			"<html><body><h1/><h2/><h3/><h4/><h5/><h6/></body></html>",
			map[string]int{"h1": 1, "h2": 1, "h3": 1, "h4": 1, "h5": 1, "h6": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustParse(t, tt.input)
			got := CountHeadings(doc)
			if len(got) != len(tt.expected) {
				t.Errorf("CountHeadings() len = %d, want %d; got %v", len(got), len(tt.expected), got)
				return
			}
			for tag, want := range tt.expected {
				if got[tag] != want {
					t.Errorf("CountHeadings()[%q] = %d, want %d", tag, got[tag], want)
				}
			}
		})
	}
}

// ── Login Form ────────────────────────────────────────────────────────────────

func TestHasLoginForm(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		url      string
		expected bool
	}{
		{
			"input_submit",
			`<html><body><form><input type="password"><input type="submit"></form></body></html>`,
			"https://example.com",
			true,
		},
		{
			"button_submit",
			`<html><body><form><input type="password"><button type="submit">Login</button></form></body></html>`,
			"https://example.com",
			true,
		},
		{
			"button_no_type_defaults_submit",
			`<html><body><form><input type="password"><button>Login</button></form></body></html>`,
			"https://example.com",
			true,
		},
		{
			"no_password_field",
			`<html><body><form><input type="text"><input type="submit"></form></body></html>`,
			"https://example.com",
			false,
		},
		{
			"no_submit",
			`<html><body><form><input type="password"><input type="text"></form></body></html>`,
			"https://example.com",
			false,
		},
		{
			"no_form",
			`<html><body><p>just text</p></body></html>`,
			"https://example.com",
			false,
		},
		{
			"url_with_member_indicator",
			`<html><head><title>App</title></head><body><div id="root"></div></body></html>`,
			"https://member.assemblynow.net/",
			true,
		},
		{
			"url_with_login_path",
			`<html><head><title>Site</title></head><body></body></html>`,
			"https://example.com/login",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustParse(t, tt.input)
			got := HasLoginForm(doc, tt.url)
			if got != tt.expected {
				t.Errorf("HasLoginForm() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// ── Link Analysis ─────────────────────────────────────────────────────────────

func TestAnalyzeLinks_Classification(t *testing.T) {
	// Use a real httptest server as the "base" so link resolution works correctly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	htmlInput := `<html><body>
		<a href="/page">internal relative</a>
		<a href="` + srv.URL + `/other">internal absolute</a>
		<a href="https://external.example.com/page">external</a>
		<a href="mailto:test@example.com">skip mailto</a>
		<a href="#">skip anchor</a>
	</body></html>`

	doc := mustParse(t, htmlInput)
	baseURL := mustParseURL(t, srv.URL)

	result := AnalyzeLinks(doc, baseURL, srv.Client())

	if result.InternalCount != 2 {
		t.Errorf("InternalCount = %d, want 2", result.InternalCount)
	}
	if result.ExternalCount != 1 {
		t.Errorf("ExternalCount = %d, want 1", result.ExternalCount)
	}
}

func TestAnalyzeLinks_InaccessibleDetection(t *testing.T) {
	// Serve a 404 for /broken so it counts as inaccessible.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/broken" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	htmlInput := `<html><body>
		<a href="` + srv.URL + `/ok">ok link</a>
		<a href="` + srv.URL + `/broken">broken link</a>
	</body></html>`

	doc := mustParse(t, htmlInput)
	baseURL := mustParseURL(t, srv.URL)

	result := AnalyzeLinks(doc, baseURL, srv.Client())

	if result.InaccessibleCount != 1 {
		t.Errorf("InaccessibleCount = %d, want 1", result.InaccessibleCount)
	}
	if len(result.InaccessibleURLs) != 1 {
		t.Errorf("len(InaccessibleURLs) = %d, want 1", len(result.InaccessibleURLs))
	}
}

// ── Analyze (orchestrator) ────────────────────────────────────────────────────

func TestAnalyze(t *testing.T) {
	body := `<!DOCTYPE html>
	<html>
	<head><title>Test Page</title></head>
	<body>
		<h1>Main</h1>
		<h2>Sub</h2>
		<form><input type="password"><input type="submit"></form>
	</body>
	</html>`

	result, err := Analyze("https://example.com", bytes.NewReader([]byte(body)), &http.Client{})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.HTMLVersion != "HTML5" {
		t.Errorf("HTMLVersion = %q, want HTML5", result.HTMLVersion)
	}
	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want Test Page", result.Title)
	}
	if result.Headings["h1"] != 1 {
		t.Errorf("h1 count = %d, want 1", result.Headings["h1"])
	}
	if result.Headings["h2"] != 1 {
		t.Errorf("h2 count = %d, want 1", result.Headings["h2"])
	}
	if !result.HasLoginForm {
		t.Error("HasLoginForm = false, want true")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustParse(t *testing.T, input string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("html.Parse() error: %v", err)
	}
	return doc
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error: %v", raw, err)
	}
	return u
}
