package analyzer

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"golang.org/x/net/html"
)

// AnalysisResult holds all data produced by analyzing a single web page.
type AnalysisResult struct {
	URL               string
	HTMLVersion       string
	Title             string
	Headings          map[string]int
	InternalLinks     int
	ExternalLinks     int
	InaccessibleCount int
	InaccessibleURLs  []string
	HasLoginForm      bool
}

// Analyze reads the HTML body, runs all analysis functions, and returns a
// populated AnalysisResult. The client is passed through so link accessibility
// checks reuse the same configured HTTP client (timeouts, transport, etc.).
func Analyze(targetURL string, body io.Reader, client *http.Client) (*AnalysisResult, error) {
	// Buffer the body so we can read it twice:
	// once for DOCTYPE detection (needs the raw token stream),
	// once for full tree parsing.
	buf, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	htmlVersion := DetectHTMLVersion(bytes.NewReader(buf))

	doc, err := html.Parse(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	baseURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("parsing target URL: %w", err)
	}

	linkResult := AnalyzeLinks(doc, baseURL, client)

	return &AnalysisResult{
		URL:               targetURL,
		HTMLVersion:       htmlVersion,
		Title:             ExtractTitle(doc),
		Headings:          CountHeadings(doc),
		InternalLinks:     linkResult.InternalCount,
		ExternalLinks:     linkResult.ExternalCount,
		InaccessibleCount: linkResult.InaccessibleCount,
		InaccessibleURLs:  linkResult.InaccessibleURLs,
		HasLoginForm:      HasLoginForm(doc, targetURL),
	}, nil
}
