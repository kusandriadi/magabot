// Web scraper using Colly - lightweight browser automation alternative
package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/kusa/magabot/internal/security"
)

// Scraper tool using Colly for web scraping
type Scraper struct {
	userAgent   string
	timeout     time.Duration
	maxDepth    int
	parallelism int
}

// ScraperConfig for Scraper tool
type ScraperConfig struct {
	UserAgent   string
	Timeout     time.Duration
	MaxDepth    int
	Parallelism int
}

// NewScraper creates a new Scraper tool
func NewScraper(cfg *ScraperConfig) *Scraper {
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxDepth == 0 {
		cfg.MaxDepth = 1
	}
	if cfg.Parallelism == 0 {
		cfg.Parallelism = 2
	}

	return &Scraper{
		userAgent:   cfg.UserAgent,
		timeout:     cfg.Timeout,
		maxDepth:    cfg.MaxDepth,
		parallelism: cfg.Parallelism,
	}
}

// Name returns tool name
func (s *Scraper) Name() string {
	return "scraper"
}

// Description returns tool description
func (s *Scraper) Description() string {
	return "Scrape web pages. Params: url (page URL), selector (CSS selector, optional), action (text|links|images|html)"
}

// Execute performs web scraping
func (s *Scraper) Execute(ctx context.Context, params map[string]string) (string, error) {
	targetURL := params["url"]
	if targetURL == "" {
		return "", fmt.Errorf("missing required parameter: url")
	}

	// Validate URL syntax
	if _, err := url.Parse(targetURL); err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// SSRF prevention - validate URL is safe to fetch
	if err := security.ValidateURL(targetURL); err != nil {
		return "", fmt.Errorf("blocked URL: %w", err)
	}

	action := params["action"]
	if action == "" {
		action = "text"
	}

	selector := params["selector"]

	switch action {
	case "text":
		return s.scrapeText(ctx, targetURL, selector)
	case "links":
		return s.scrapeLinks(ctx, targetURL, selector)
	case "images":
		return s.scrapeImages(ctx, targetURL)
	case "html":
		return s.scrapeHTML(ctx, targetURL, selector)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// scrapeText extracts text content from a page
func (s *Scraper) scrapeText(ctx context.Context, targetURL, selector string) (string, error) {
	c := s.newCollector()

	var texts []string
	var title string

	c.OnHTML("title", func(e *colly.HTMLElement) {
		title = e.Text
	})

	if selector != "" {
		c.OnHTML(selector, func(e *colly.HTMLElement) {
			text := strings.TrimSpace(e.Text)
			if text != "" {
				texts = append(texts, text)
			}
		})
	} else {
		// Default: extract main content
		c.OnHTML("article, main, .content, .post, .entry, #content, #main", func(e *colly.HTMLElement) {
			text := strings.TrimSpace(e.Text)
			if text != "" {
				texts = append(texts, text)
			}
		})

		// Fallback to body paragraphs
		c.OnHTML("body p", func(e *colly.HTMLElement) {
			text := strings.TrimSpace(e.Text)
			if len(text) > 50 { // Only meaningful paragraphs
				texts = append(texts, text)
			}
		})
	}

	if err := c.Visit(targetURL); err != nil {
		return "", fmt.Errorf("scrape failed: %w", err)
	}

	c.Wait()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📄 **%s**\n", title))
	sb.WriteString(fmt.Sprintf("🔗 %s\n\n", targetURL))

	if len(texts) == 0 {
		sb.WriteString("(No content found)")
	} else {
		// Limit output
		content := strings.Join(texts, "\n\n")
		if len(content) > 3000 {
			content = content[:3000] + "\n\n... (truncated)"
		}
		sb.WriteString(content)
	}

	return sb.String(), nil
}

// scrapeLinks extracts all links from a page
func (s *Scraper) scrapeLinks(ctx context.Context, targetURL, selector string) (string, error) {
	c := s.newCollector()

	var links []struct {
		Text string
		URL  string
	}
	var title string

	c.OnHTML("title", func(e *colly.HTMLElement) {
		title = e.Text
	})

	linkSelector := "a[href]"
	if selector != "" {
		linkSelector = selector + " a[href]"
	}

	c.OnHTML(linkSelector, func(e *colly.HTMLElement) {
		href := e.Attr("href")
		text := strings.TrimSpace(e.Text)

		// Skip empty, anchor-only, and javascript links
		if href == "" || href == "#" || strings.HasPrefix(href, "javascript:") {
			return
		}

		// Make absolute URL
		absURL := e.Request.AbsoluteURL(href)
		if absURL != "" {
			links = append(links, struct {
				Text string
				URL  string
			}{text, absURL})
		}
	})

	if err := c.Visit(targetURL); err != nil {
		return "", fmt.Errorf("scrape failed: %w", err)
	}

	c.Wait()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔗 **Links from: %s**\n\n", title))

	if len(links) == 0 {
		sb.WriteString("(No links found)")
	} else {
		// Limit to first 20 links
		limit := 20
		if len(links) < limit {
			limit = len(links)
		}
		for i := 0; i < limit; i++ {
			l := links[i]
			if l.Text != "" {
				sb.WriteString(fmt.Sprintf("• [%s](%s)\n", truncate(l.Text, 50), l.URL))
			} else {
				sb.WriteString(fmt.Sprintf("• %s\n", l.URL))
			}
		}
		if len(links) > 20 {
			sb.WriteString(fmt.Sprintf("\n... and %d more links", len(links)-20))
		}
	}

	return sb.String(), nil
}

// scrapeImages extracts image URLs from a page
func (s *Scraper) scrapeImages(ctx context.Context, targetURL string) (string, error) {
	c := s.newCollector()

	var images []struct {
		Alt string
		URL string
	}

	c.OnHTML("img[src]", func(e *colly.HTMLElement) {
		src := e.Attr("src")
		alt := e.Attr("alt")

		// Skip tiny images (likely icons)
		if strings.Contains(src, "icon") || strings.Contains(src, "logo") {
			return
		}

		absURL := e.Request.AbsoluteURL(src)
		if absURL != "" {
			images = append(images, struct {
				Alt string
				URL string
			}{alt, absURL})
		}
	})

	if err := c.Visit(targetURL); err != nil {
		return "", fmt.Errorf("scrape failed: %w", err)
	}

	c.Wait()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🖼️ **Images from: %s**\n\n", targetURL))

	if len(images) == 0 {
		sb.WriteString("(No images found)")
	} else {
		limit := 10
		if len(images) < limit {
			limit = len(images)
		}
		for i := 0; i < limit; i++ {
			img := images[i]
			if img.Alt != "" {
				sb.WriteString(fmt.Sprintf("• %s: %s\n", img.Alt, img.URL))
			} else {
				sb.WriteString(fmt.Sprintf("• %s\n", img.URL))
			}
		}
		if len(images) > 10 {
			sb.WriteString(fmt.Sprintf("\n... and %d more images", len(images)-10))
		}
	}

	return sb.String(), nil
}

// scrapeHTML gets raw HTML of a selector
func (s *Scraper) scrapeHTML(ctx context.Context, targetURL, selector string) (string, error) {
	c := s.newCollector()

	var html string

	sel := "body"
	if selector != "" {
		sel = selector
	}

	c.OnHTML(sel, func(e *colly.HTMLElement) {
		h, _ := e.DOM.Html()
		html = h
	})

	if err := c.Visit(targetURL); err != nil {
		return "", fmt.Errorf("scrape failed: %w", err)
	}

	c.Wait()

	if html == "" {
		return "(No content found)", nil
	}

	// Limit output
	if len(html) > 5000 {
		html = html[:5000] + "\n... (truncated)"
	}

	return html, nil
}

// DuckDuckGoSearch searches using DuckDuckGo HTML (no API key needed)
func (s *Scraper) DuckDuckGoSearch(ctx context.Context, query string, count int) (string, error) {
	if count <= 0 {
		count = 5
	}

	c := s.newCollector()

	var results []struct {
		Title   string
		URL     string
		Snippet string
	}

	// DuckDuckGo HTML search results
	c.OnHTML(".result", func(e *colly.HTMLElement) {
		title := e.ChildText(".result__title")
		link := e.ChildAttr(".result__url", "href")
		snippet := e.ChildText(".result__snippet")

		if title != "" && link != "" {
			results = append(results, struct {
				Title   string
				URL     string
				Snippet string
			}{title, link, snippet})
		}
	})

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	if err := c.Visit(searchURL); err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	c.Wait()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **Search results for: %s**\n\n", query))

	if len(results) == 0 {
		sb.WriteString("(No results found)")
	} else {
		limit := count
		if len(results) < limit {
			limit = len(results)
		}
		for i := 0; i < limit; i++ {
			r := results[i]
			sb.WriteString(fmt.Sprintf("**%d. %s**\n", i+1, r.Title))
			sb.WriteString(fmt.Sprintf("   🔗 %s\n", r.URL))
			if r.Snippet != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", truncate(r.Snippet, 150)))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// newCollector creates a configured Colly collector
func (s *Scraper) newCollector() *colly.Collector {
	c := colly.NewCollector(
		colly.UserAgent(s.userAgent),
		colly.MaxDepth(s.maxDepth),
		colly.Async(true),
	)

	c.SetRequestTimeout(s.timeout)

	_ = c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: s.parallelism,
		Delay:       500 * time.Millisecond,
	})

	// Handle errors silently
	c.OnError(func(r *colly.Response, err error) {
		// Log or ignore
	})

	return c
}
