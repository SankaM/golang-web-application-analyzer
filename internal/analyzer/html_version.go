package analyzer

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// DetectHTMLVersion reads the DOCTYPE token from the HTML stream and maps it
// to a human-readable version string. It reads only until the first token,
// so it is efficient even for large documents.
func DetectHTMLVersion(r io.Reader) string {
	tokenizer := html.NewTokenizer(r)
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return "Unknown"
		case html.DoctypeToken:
			return parseDoctype(string(tokenizer.Raw()))
		}
	}
}

// parseDoctype maps a raw DOCTYPE string to a version name using
// case-insensitive substring matching — no regex needed.
func parseDoctype(raw string) string {
	s := strings.ToLower(raw)

	switch {
	case strings.Contains(s, "xhtml 1.1"):
		return "XHTML 1.1"
	case strings.Contains(s, "xhtml 1.0") && strings.Contains(s, "strict"):
		return "XHTML 1.0 Strict"
	case strings.Contains(s, "xhtml 1.0") && strings.Contains(s, "transitional"):
		return "XHTML 1.0 Transitional"
	case strings.Contains(s, "xhtml 1.0") && strings.Contains(s, "frameset"):
		return "XHTML 1.0 Frameset"
	case strings.Contains(s, "html 4.01") && strings.Contains(s, "strict"):
		return "HTML 4.01 Strict"
	case strings.Contains(s, "html 4.01") && strings.Contains(s, "transitional"):
		return "HTML 4.01 Transitional"
	case strings.Contains(s, "html 4.01") && strings.Contains(s, "frameset"):
		return "HTML 4.01 Frameset"
	case strings.Contains(s, "html 4.0") && strings.Contains(s, "strict"):
		return "HTML 4.0 Strict"
	case strings.Contains(s, "html 4.0") && strings.Contains(s, "transitional"):
		return "HTML 4.0 Transitional"
	// HTML5: <!DOCTYPE html> has no PUBLIC identifier
	case !strings.Contains(s, "public") && !strings.Contains(s, "system"):
		return "HTML5"
	default:
		return "Unknown"
	}
}
