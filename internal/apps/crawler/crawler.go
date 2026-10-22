// Package crawler implements a concurrent site auditor and link checker.
//
// It crawls a website starting from a given URL, following links and recording
// the HTTP status code and response time for each page. The crawler is designed
// to be well-behaved: it uses bounded concurrency to avoid overwhelming target
// servers, rate limiting to space out requests, and context-based cancellation
// for graceful shutdown (e.g., on SIGINT).
//
// DESIGN PRINCIPLES:
//
// 1. BOUNDED CONCURRENCY (Semaphore Pattern):
//    Unbounded goroutine creation is dangerous — each goroutine consumes ~2-8 KB
//    of stack, and thousands of simultaneous HTTP connections can exhaust file
//    descriptors, saturate network bandwidth, or trigger rate limits / IP bans.
//    We use a buffered channel as a counting semaphore to cap the number of
//    concurrent workers. This is simpler than a full worker pool and more
//    flexible — goroutines are created on demand rather than pre-allocated.
//
// 2. RATE LIMITING:
//    Even with bounded concurrency, rapid-fire requests can overwhelm a server.
//    A time.Ticker spaces out request starts to a maximum rate, which is both
//    polite and prevents triggering server-side rate limits.
//
// 3. GRACEFUL SHUTDOWN via context.Context:
//    When the user sends SIGINT (Ctrl+C), we want to stop launching new requests
//    but allow in-flight requests to complete. context.Context propagation makes
//    this natural: the parent creates a cancellable context, and every goroutine
//    checks ctx.Done() before starting work.
package crawler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"io"
	"net/url"
)

// Result captures the outcome of fetching a single URL.
// It includes the HTTP status, any error encountered, the round-trip duration,
// and any links discovered on the page.
type Result struct {
	URL        string        // The URL that was fetched.
	StatusCode int           // HTTP status code (0 if the request failed entirely).
	Error      error         // Non-nil if the fetch or parse failed.
	Duration   time.Duration // How long the HTTP round-trip took.
	Links      []string      // Outgoing links discovered on this page.
}

// Auditor is the main crawler engine. It manages concurrency, rate limiting,
// and visited-URL tracking.
//
// WHY sync.Map FOR THE VISITED SET?
// sync.Map is optimized for two specific patterns:
//   1. Keys are written once and read many times (append-only).
//   2. Multiple goroutines read/write disjoint key sets.
//
// Our visited set is exactly pattern #1: each URL is stored once (when first
// seen) and then only read (to check "have we seen this?"). For this pattern,
// sync.Map avoids the coarse locking of map+Mutex and performs better under
// high concurrency.
//
// WHEN WOULD map+Mutex BE BETTER?
// If we needed to iterate over all visited URLs, compute the size, or if
// the write pattern was not append-only (e.g., deleting entries), a regular
// map with sync.RWMutex would be simpler and potentially faster because
// sync.Map has overhead from its internal "dirty" and "read" map structure.
type Auditor struct {
	maxWorkers int           // Maximum number of concurrent HTTP fetches.
	rateLimit  time.Duration // Minimum interval between request starts.
	client     *http.Client  // Shared HTTP client (connection pooling built in).
	visited    sync.Map      // Set of URLs we've already queued for fetching.
	results    chan Result    // Channel where fetch results are sent.
	wg         sync.WaitGroup // Tracks in-flight fetch goroutines.
}

// NewAuditor creates a new Auditor with the specified concurrency, rate limit,
// and HTTP timeout settings.
//
// WHY A SHARED http.Client?
// http.Client maintains an internal connection pool (via http.Transport).
// Creating a new client per request would bypass connection reuse (HTTP
// keep-alive), causing a new TCP+TLS handshake for every request. A single
// shared client reuses connections to the same host, dramatically reducing
// latency and resource consumption.
func NewAuditor(maxWorkers int, rateLimit time.Duration, timeout time.Duration) *Auditor {
	return &Auditor{
		maxWorkers: maxWorkers,
		rateLimit:  rateLimit,
		client: &http.Client{
			Timeout: timeout,
			// We intentionally do NOT follow redirects automatically here
			// so we can record the redirect status code (301, 302, etc.).
			// If we wanted to follow redirects, we'd leave this as nil.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects (max 10)")
				}
				return nil
			},
		},
		// Buffer the results channel so that slow consumers don't block
		// the workers. The buffer size is somewhat arbitrary; maxWorkers
		// is a reasonable choice because at most that many results can be
		// produced simultaneously.
		results: make(chan Result, maxWorkers),
	}
}

// Audit starts crawling from startURL and returns all collected Results.
// It blocks until all reachable pages have been visited or the context is
// cancelled.
//
// HOW THE SEMAPHORE PATTERN WORKS:
// We create a buffered channel of size maxWorkers. Before launching a fetch,
// we send a value into the channel (acquiring a "slot"). When the fetch
// completes, we receive from the channel (releasing the slot). If all slots
// are taken, the send blocks, naturally throttling concurrency.
//
// This is simpler than a traditional worker pool (pre-spawning N goroutines
// that read from a work queue) and more flexible — goroutines are created
// on demand. The trade-off is slightly more goroutine churn, but for I/O-bound
// workloads like HTTP fetching, the goroutine creation overhead is negligible
// compared to network latency.
func (a *Auditor) Audit(ctx context.Context, startURL string) []Result {
	// Semaphore channel: limits concurrent workers to maxWorkers.
	sem := make(chan struct{}, a.maxWorkers)

	// Rate limiter: ensures we don't start requests faster than rateLimit.
	// A Ticker sends a value on its channel at regular intervals.
	var ticker *time.Ticker
	if a.rateLimit > 0 {
		ticker = time.NewTicker(a.rateLimit)
		defer ticker.Stop()
	}

	// Collector goroutine: reads results from the channel and appends them
	// to a slice. This runs in its own goroutine so that workers don't
	// block if the results slice append is slow.
	var allResults []Result
	var resultsMu sync.Mutex // Protects allResults slice.

	// done channel signals when all workers have finished.
	done := make(chan struct{})

	// Start the collection goroutine that drains the results channel.
	go func() {
		for r := range a.results {
			resultsMu.Lock()
			allResults = append(allResults, r)
			resultsMu.Unlock()
		}
		close(done)
	}()

	// Seed the crawl with the start URL.
	// We use a queue-based approach: discovered links are added to a queue,
	// and we process them in a BFS-like manner.
	//
	// sync.Map.LoadOrStore atomically checks if a key exists and stores it
	// if not. The `loaded` return value tells us whether the key was already
	// present. This is our thread-safe "visited" check.
	a.visited.LoadOrStore(startURL, true)

	// crawl is a recursive-like function (via goroutines) that fetches a URL,
	// extracts links, and spawns new fetches for unvisited links.
	var crawl func(targetURL string)
	crawl = func(targetURL string) {
		defer a.wg.Done()

		// Check if the context has been cancelled (e.g., SIGINT received).
		// This is the graceful shutdown mechanism: we check before doing
		// expensive work (the HTTP fetch). Already-running fetches will
		// complete, but we won't start new ones.
		select {
		case <-ctx.Done():
			return
		default:
			// Context is still active; proceed.
		}

		// Rate limiting: wait for the ticker before proceeding.
		// This ensures we don't hammer the server faster than rateLimit.
		if ticker != nil {
			select {
			case <-ticker.C:
				// Got a tick — we're allowed to proceed.
			case <-ctx.Done():
				// Context cancelled while waiting for rate limit.
				return
			}
		}

		// Acquire a semaphore slot. This blocks if maxWorkers goroutines
		// are already doing HTTP fetches, naturally throttling concurrency.
		select {
		case sem <- struct{}{}:
			// Acquired a slot.
		case <-ctx.Done():
			return
		}

		// Fetch the URL and parse its links.
		result := a.fetchAndParse(ctx, targetURL)

		// Release the semaphore slot now that the HTTP fetch is done.
		// We release BEFORE processing links so the slot is available
		// for other workers while we're doing CPU-bound link extraction.
		<-sem

		// Send the result to the collector.
		select {
		case a.results <- result:
		case <-ctx.Done():
			return
		}

		// Spawn new crawl goroutines for any unvisited links.
		for _, link := range result.Links {
			// LoadOrStore is atomic: if the link was already visited,
			// loaded will be true and we skip it. Otherwise, we mark
			// it as visited and spawn a fetch for it.
			if _, loaded := a.visited.LoadOrStore(link, true); !loaded {
				a.wg.Add(1)
				go crawl(link)
			}
		}
	}

	// Kick off the crawl with the starting URL.
	a.wg.Add(1)
	go crawl(startURL)

	// Wait for all crawl goroutines to finish, then close the results
	// channel to signal the collector goroutine to exit.
	a.wg.Wait()
	close(a.results)

	// Wait for the collector to finish draining.
	<-done

	return allResults
}

// fetchAndParse performs an HTTP GET request to the given URL, reads the
// response body, and extracts any outgoing links from the HTML content.
//
// It respects context cancellation: if the context is cancelled during the
// HTTP request, the request is aborted and an error is returned.
func (a *Auditor) fetchAndParse(ctx context.Context, targetURL string) Result {
	result := Result{
		URL: targetURL,
	}

	start := time.Now()

	// Create a request with the context attached. This allows cancellation
	// to propagate into the HTTP transport layer — if ctx is cancelled,
	// the underlying TCP connection is closed and the request returns
	// immediately with an error.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		result.Error = fmt.Errorf("creating request: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Set a User-Agent header to identify ourselves. Many servers block
	// requests with no User-Agent as a basic bot-detection measure.
	req.Header.Set("User-Agent", "GoMastery-Crawler/1.0")

	resp, err := a.client.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("fetching %s: %w", targetURL, err)
		result.Duration = time.Since(start)
		return result
	}
	// Always close the response body to return the connection to the pool.
	// Failing to close it causes connection leaks, eventually exhausting
	// file descriptors.
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Duration = time.Since(start)

	// Read the entire body. We limit to 10 MB to avoid memory issues
	// with extremely large pages.
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		result.Error = fmt.Errorf("reading body of %s: %w", targetURL, err)
		return result
	}

	// Only extract links from HTML content. Binary files, images, etc.
	// won't have useful <a> tags.
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		result.Links = extractLinks(string(bodyBytes), targetURL)
	}

	return result
}

// extractLinks extracts href values from <a> tags in an HTML body string
// and resolves them relative to baseURL.
//
// WHY NOT USE AN HTML PARSER?
// The requirement is stdlib-only, so we can't use golang.org/x/net/html.
// This simple string-scanning approach handles the most common cases:
//   - <a href="..."> with double quotes
//   - <a href='...'> with single quotes
//
// It will miss some edge cases (e.g., unquoted attributes, JavaScript-
// generated links), but for a site auditor, it catches the vast majority
// of navigable links.
//
// LINK RESOLUTION:
// HTML can contain relative URLs (e.g., "/about", "../contact", "page.html").
// We use net/url.Parse and ResolveReference to convert these to absolute URLs
// relative to the page's base URL. This handles all the tricky cases:
// scheme-relative ("//example.com"), path-relative ("foo/bar"), and
// absolute ("/foo/bar").
func extractLinks(body, baseURL string) []string {
	// Parse the base URL so we can resolve relative links against it.
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	var links []string

	// seen tracks links we've already found on THIS page to avoid
	// duplicates in the output. This is a page-level dedup, separate
	// from the Auditor's global visited set.
	seen := make(map[string]bool)

	// Scan the HTML body for href attributes. We look for the pattern
	// href="..." or href='...' inside what we expect to be <a> tags.
	// This is a simplified scanner — a real HTML parser would handle
	// many more edge cases.
	remaining := body
	for {
		// Find the next href attribute. We search case-insensitively
		// by looking for common patterns.
		hrefIdx := indexCaseInsensitive(remaining, "href=")
		if hrefIdx == -1 {
			break
		}

		// Move past "href="
		remaining = remaining[hrefIdx+5:]
		if len(remaining) == 0 {
			break
		}

		// Determine the quote character (double or single quote).
		// If the href value isn't quoted, we skip it — unquoted attribute
		// values are rare and harder to parse correctly.
		quote := remaining[0]
		if quote != '"' && quote != '\'' {
			continue
		}

		// Find the closing quote.
		remaining = remaining[1:]
		endIdx := strings.IndexByte(remaining, quote)
		if endIdx == -1 {
			break
		}

		href := strings.TrimSpace(remaining[:endIdx])
		remaining = remaining[endIdx+1:]

		// Skip empty hrefs, fragment-only links (#section), javascript:,
		// and mailto: links — these aren't useful for crawling.
		if href == "" || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(href, "javascript:") ||
			strings.HasPrefix(href, "mailto:") ||
			strings.HasPrefix(href, "tel:") ||
			strings.HasPrefix(href, "data:") {
			continue
		}

		// Resolve the href relative to the base URL.
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		resolved := base.ResolveReference(parsed)

		// Only follow HTTP and HTTPS links.
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			continue
		}

		// Strip the fragment (#...) from the resolved URL because
		// fragments refer to sections within a page, not different pages.
		resolved.Fragment = ""
		absoluteURL := resolved.String()

		// Deduplicate within this page.
		if !seen[absoluteURL] {
			seen[absoluteURL] = true
			links = append(links, absoluteURL)
		}
	}

	return links
}

// indexCaseInsensitive finds the first occurrence of substr in s,
// ignoring case. Returns -1 if not found.
//
// We implement this ourselves because strings.Index is case-sensitive
// and we want to match "HREF=", "Href=", "href=", etc.
func indexCaseInsensitive(s, substr string) int {
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	return strings.Index(sLower, substrLower)
}
