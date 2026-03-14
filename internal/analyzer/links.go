package analyzer

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// LinkResult holds the outcome of the link analysis phase.
type LinkResult struct {
	InternalCount     int
	ExternalCount     int
	InaccessibleCount int
	InaccessibleURLs  []string
}

const maxConcurrent = 10

// AnalyzeLinks collects all <a href> links from the document, classifies them
// as internal or external relative to base, then checks each for accessibility
// using up to maxConcurrent goroutines in parallel.
func AnalyzeLinks(doc *html.Node, base *url.URL, client *http.Client) LinkResult {
	links := collectLinks(doc, base)

	var result LinkResult
	result.InternalCount = links.internal
	result.ExternalCount = links.external

	inaccessible, urls := checkAccessibility(links.all, client)
	result.InaccessibleCount = inaccessible
	result.InaccessibleURLs = urls
	return result
}

type collectedLinks struct {
	internal int
	external int
	all      []string
}

// collectLinks walks the node tree, resolves every href against base, and
// classifies each as internal (same host) or external (different host).
func collectLinks(doc *html.Node, base *url.URL) collectedLinks {
	var cl collectedLinks
	seen := make(map[string]bool)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attrVal(n, "href")
			if shouldSkip(href) {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				return
			}

			resolved, err := base.Parse(href)
			if err != nil {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				return
			}

			full := resolved.String()
			if resolved.Host == base.Host {
				cl.internal++
			} else {
				cl.external++
			}

			// Deduplicate before accessibility checks to avoid redundant requests.
			if !seen[full] {
				seen[full] = true
				cl.all = append(cl.all, full)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return cl
}

// shouldSkip returns true for hrefs that are not real HTTP links.
func shouldSkip(href string) bool {
	if href == "" || href == "#" {
		return true
	}
	for _, prefix := range []string{"mailto:", "tel:", "javascript:", "#"} {
		if strings.HasPrefix(href, prefix) {
			return true
		}
	}
	return false
}

// checkAccessibility checks each URL concurrently using a semaphore channel
// to cap parallelism at maxConcurrent. A WaitGroup ensures we wait for all
// goroutines before returning. A Mutex protects the shared result slice.
func checkAccessibility(links []string, client *http.Client) (int, []string) {
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex

	var inaccessibleURLs []string

	for _, link := range links {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if !isAccessible(l, client) {
				mu.Lock()
				inaccessibleURLs = append(inaccessibleURLs, l)
				mu.Unlock()
			}
		}(link)
	}
	wg.Wait()

	return len(inaccessibleURLs), inaccessibleURLs
}

// isAccessible sends a HEAD request and falls back to GET on 405.
// Any status >= 400 or network error is treated as inaccessible.
func isAccessible(rawURL string, client *http.Client) bool {
	resp, err := client.Head(rawURL)
	if err != nil {
		return false
	}
	resp.Body.Close()

	// Some servers do not support HEAD; retry with GET.
	if resp.StatusCode == http.StatusMethodNotAllowed {
		resp, err = client.Get(rawURL)
		if err != nil {
			return false
		}
		resp.Body.Close()
	}

	return resp.StatusCode < http.StatusBadRequest
}
