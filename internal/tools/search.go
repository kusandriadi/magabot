// Web Search - Brave API (free tier) with DuckDuckGo fallback (no API key)
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/kusa/magabot/internal/util"
)

const braveSearchURL = "https://api.search.brave.com/res/v1/web/search"

// Search tool - Brave API primary, DuckDuckGo fallback
type Search struct {
	apiKey    string
	client    *http.Client
	userAgent string
}

// SearchConfig for Search tool
type SearchConfig struct {
	APIKey string // Brave Search API key (optional - will use DDG if empty)
}

// NewSearch creates a new Search tool
func NewSearch(cfg *SearchConfig) *Search {
	return &Search{
		apiKey:    cfg.APIKey,
		client:    util.NewHTTPClient(0),
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	}
}

// Name returns tool name
func (s *Search) Name() string {
	return "search"
}

// Description returns tool description
func (s *Search) Description() string {
	return "Search the web. Params: q (query), count (1-10, default 5)"
}

// Execute performs a web search
func (s *Search) Execute(ctx context.Context, params map[string]string) (string, error) {
	query := params["q"]
	if query == "" {
		return "", fmt.Errorf("missing required parameter: q")
	}

	count := 5
	if c := params["count"]; c != "" {
		_, _ = fmt.Sscanf(c, "%d", &count)
		if count < 1 {
			count = 1
		}
		if count > 10 {
			count = 10
		}
	}

	// Try Brave first if API key is set
	if s.apiKey != "" {
		result, err := s.braveSearch(ctx, query, count)
		if err == nil {
			return result, nil
		}
		// Fall through to DuckDuckGo
	}

	// Fallback to DuckDuckGo (no API key needed)
	return s.duckDuckGoSearch(ctx, query, count)
}

// braveSearch uses Brave Search API
func (s *Search) braveSearch(ctx context.Context, query string, count int) (string, error) {
	u, _ := url.Parse(braveSearchURL)
	q := u.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprintf("%d", count))
	u.RawQuery = q.Encode()

	resp, err := util.DoGET(ctx, s.client, u.String(), map[string]string{
		"Accept":               "application/json",
		"X-Subscription-Token": s.apiKey,
	})
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := util.ReadHTTPResponse(resp, "Brave API")
	if err != nil {
		return "", err
	}

	var raw struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	results := make([]util.SearchResult, len(raw.Web.Results))
	for i, r := range raw.Web.Results {
		results[i] = util.SearchResult{Title: r.Title, URL: r.URL, Description: r.Description}
	}

	return util.FormatSearchResults(fmt.Sprintf("🔍 **Search: %s** (via Brave)", query), results, 200), nil
}

// duckDuckGoSearch scrapes DuckDuckGo HTML (no API key needed!)
func (s *Search) duckDuckGoSearch(ctx context.Context, query string, count int) (string, error) {
	c := colly.NewCollector(
		colly.UserAgent(s.userAgent),
		colly.Async(true),
	)
	c.SetRequestTimeout(30 * time.Second)

	var results []util.SearchResult

	appendResult := func(title, rawURL, snippet string) {
		if len(results) >= count || title == "" || rawURL == "" {
			return
		}
		// Extract actual URL from DuckDuckGo redirect
		actualURL := rawURL
		if strings.Contains(rawURL, "uddg=") {
			if u, err := url.Parse(rawURL); err == nil {
				if uddg := u.Query().Get("uddg"); uddg != "" {
					actualURL = uddg
				}
			}
		}
		results = append(results, util.SearchResult{Title: title, URL: actualURL, Description: snippet})
	}

	// DuckDuckGo HTML search results
	c.OnHTML(".result", func(e *colly.HTMLElement) {
		appendResult(
			strings.TrimSpace(e.ChildText(".result__title")),
			e.ChildAttr(".result__url", "href"),
			strings.TrimSpace(e.ChildText(".result__snippet")),
		)
	})

	// Alternative selector for newer DDG layout
	c.OnHTML(".result__body", func(e *colly.HTMLElement) {
		appendResult(
			strings.TrimSpace(e.ChildText("a.result__a")),
			e.ChildAttr("a.result__a", "href"),
			strings.TrimSpace(e.ChildText(".result__snippet")),
		)
	})

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	if err := c.Visit(searchURL); err != nil {
		return "", fmt.Errorf("DDG search failed: %w", err)
	}

	c.Wait()

	if len(results) == 0 {
		return fmt.Sprintf("🔍 No results found for: %s", query), nil
	}

	return util.FormatSearchResults(fmt.Sprintf("🔍 **Search: %s** (via DuckDuckGo)", query), results, 200), nil
}
