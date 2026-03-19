package browserfetch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Fetcher retrieves rendered HTML using a headless browser.
// This is a best-effort fallback for sites that block plain HTTP clients.
type Fetcher struct {
	timeout time.Duration
}

func New(timeout time.Duration) *Fetcher {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Fetcher{timeout: timeout}
}

func (f *Fetcher) FetchHTML(ctx context.Context, targetURL string) (string, error) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	runCtx, runCancel := context.WithTimeout(browserCtx, f.timeout)
	defer runCancel()

	var docType string
	var outerHTML string
	err := chromedp.Run(runCtx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`document.doctype ? new XMLSerializer().serializeToString(document.doctype) : ""`, &docType),
		chromedp.OuterHTML("html", &outerHTML, chromedp.ByQuery),
	)
	if err != nil {
		return "", fmt.Errorf("browser fetch failed: %w", err)
	}

	outerHTML = strings.TrimSpace(outerHTML)
	if outerHTML == "" {
		return "", errors.New("browser fetch returned empty HTML")
	}

	docType = strings.TrimSpace(docType)
	if docType != "" {
		return docType + "\n" + outerHTML, nil
	}

	return outerHTML, nil
}
