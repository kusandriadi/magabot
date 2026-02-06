// Web Search - Brave API (free tier) with DuckDuckGo fallback (no API key)
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
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
		client:    &http.Client{Timeout: 30 * time.Second},
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
		fmt.Sscanf(c, "%d", &count)
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

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Brave API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return s.formatResults(query, result.Web.Results, "Brave"), nil
}

// duckDuckGoSearch scrapes DuckDuckGo HTML (no API key needed!)
func (s *Search) duckDuckGoSearch(ctx context.Context, query string, count int) (string, error) {
	c := colly.NewCollector(
		colly.UserAgent(s.userAgent),
		colly.Async(true),
	)
	c.SetRequestTimeout(30 * time.Second)

	var results []struct {
		Title       string
		URL         string
		Description string
	}

	// DuckDuckGo HTML search results
	c.OnHTML(".result", func(e *colly.HTMLElement) {
		if len(results) >= count {
			return
		}

		title := strings.TrimSpace(e.ChildText(".result__title"))
		rawURL := e.ChildAttr(".result__url", "href")
		snippet := strings.TrimSpace(e.ChildText(".result__snippet"))

		// Extract actual URL from DuckDuckGo redirect
		actualURL := rawURL
		if strings.Contains(rawURL, "uddg=") {
			if u, err := url.Parse(rawURL); err == nil {
				if uddg := u.Query().Get("uddg"); uddg != "" {
					actualURL = uddg
				}
			}
		}

		if title != "" && actualURL != "" {
			results = append(results, struct {
				Title       string
				URL         string
				Description string
			}{title, actualURL, snippet})
		}
	})

	// Alternative selector for newer DDG layout
	c.OnHTML(".result__body", func(e *colly.HTMLElement) {
		if len(results) >= count {
			return
		}

		title := strings.TrimSpace(e.ChildText("a.result__a"))
		rawURL := e.ChildAttr("a.result__a", "href")
		snippet := strings.TrimSpace(e.ChildText(".result__snippet"))

		actualURL := rawURL
		if strings.Contains(rawURL, "uddg=") {
			if u, err := url.Parse(rawURL); err == nil {
				if uddg := u.Query().Get("uddg"); uddg != "" {
					actualURL = uddg
				}
			}
		}

		if title != "" && actualURL != "" {
			results = append(results, struct {
				Title       string
				URL         string
				Description string
			}{title, actualURL, snippet})
		}
	})

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	if err := c.Visit(searchURL); err != nil {
		return "", fmt.Errorf("DDG search failed: %w", err)
	}

	c.Wait()

	if len(results) == 0 {
		return fmt.Sprintf("🔍 No results found for: %s", query), nil
	}

	// Convert to interface for formatResults
	interfaceResults := make([]struct {
		Title       string
		URL         string
		Description string
	}, len(results))
	copy(interfaceResults, results)

	return s.formatResultsGeneric(query, interfaceResults, "DuckDuckGo"), nil
}

// formatResults formats Brave search results
func (s *Search) formatResults(query string, results []struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}, source string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **Search: %s** (via %s)\n\n", query, source))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("**%d. %s**\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   🔗 %s\n", r.URL))
		if r.Description != "" {
			desc := r.Description
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("   %s\n", desc))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatResultsGeneric formats generic search results
func (s *Search) formatResultsGeneric(query string, results []struct {
	Title       string
	URL         string
	Description string
}, source string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **Search: %s** (via %s)\n\n", query, source))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("**%d. %s**\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   🔗 %s\n", r.URL))
		if r.Description != "" {
			desc := r.Description
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("   %s\n", desc))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
