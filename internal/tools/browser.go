// Browser automation using Rod (Chromium-based)
package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/kusa/magabot/internal/security"
)

// Browser tool using Rod for full browser automation
type Browser struct {
	headless bool
	timeout  time.Duration
	browser  *rod.Browser
}

// BrowserConfig for Browser tool
type BrowserConfig struct {
	Headless bool          // Run in headless mode (default: true)
	Timeout  time.Duration // Page load timeout
}

// NewBrowser creates a new Browser tool
func NewBrowser(cfg *BrowserConfig) *Browser {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &Browser{
		headless: cfg.Headless,
		timeout:  cfg.Timeout,
	}
}

// Name returns tool name
func (b *Browser) Name() string {
	return "browser"
}

// Description returns tool description
func (b *Browser) Description() string {
	return "Browser automation (JavaScript rendering). Params: url, action (text|screenshot|click|type|links), selector, value"
}

// Start initializes the browser
func (b *Browser) Start() error {
	// Auto-download Chromium if needed
	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(b.headless).MustLaunch()

	b.browser = rod.New().ControlURL(u).MustConnect()
	return nil
}

// Stop closes the browser
func (b *Browser) Stop() {
	if b.browser != nil {
		b.browser.MustClose()
	}
}

// Execute performs browser action
func (b *Browser) Execute(ctx context.Context, params map[string]string) (string, error) {
	targetURL := params["url"]
	if targetURL == "" {
		return "", fmt.Errorf("missing required parameter: url")
	}

	// SSRF prevention - validate URL is safe to fetch
	if err := security.ValidateURL(targetURL); err != nil {
		return "", fmt.Errorf("blocked URL: %w", err)
	}

	action := params["action"]
	if action == "" {
		action = "text"
	}

	// Ensure browser is started
	if b.browser == nil {
		if err := b.Start(); err != nil {
			return "", fmt.Errorf("start browser: %w", err)
		}
	}

	switch action {
	case "text":
		return b.getText(ctx, targetURL, params["selector"])
	case "screenshot":
		return b.getScreenshot(ctx, targetURL, params["selector"])
	case "links":
		return b.getLinks(ctx, targetURL, params["selector"])
	case "click":
		return b.click(ctx, targetURL, params["selector"])
	case "type":
		return b.typeText(ctx, targetURL, params["selector"], params["value"])
	case "eval":
		return b.evaluate(ctx, targetURL, params["script"])
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// getText extracts text from page (with JavaScript rendered)
func (b *Browser) getText(ctx context.Context, targetURL, selector string) (string, error) {
	page := b.browser.MustPage(targetURL)
	defer page.MustClose()

	page.MustWaitLoad()

	var text string
	if selector != "" {
		el, err := page.Element(selector)
		if err != nil {
			return "", fmt.Errorf("element not found: %s", selector)
		}
		text = el.MustText()
	} else {
		// Get main content
		text = page.MustElement("body").MustText()
	}

	// Limit output
	if len(text) > 5000 {
		text = text[:5000] + "\n... (truncated)"
	}

	title := page.MustInfo().Title

	return fmt.Sprintf("📄 **%s**\n🔗 %s\n\n%s", title, targetURL, text), nil
}

// getScreenshot takes a screenshot
func (b *Browser) getScreenshot(ctx context.Context, targetURL, selector string) (string, error) {
	page := b.browser.MustPage(targetURL)
	defer page.MustClose()

	page.MustWaitLoad()

	var data []byte
	if selector != "" {
		el, err := page.Element(selector)
		if err != nil {
			return "", fmt.Errorf("element not found: %s", selector)
		}
		data, _ = el.Screenshot(proto.PageCaptureScreenshotFormatPng, 80)
	} else {
		quality := 80
		data, _ = page.Screenshot(true, &proto.PageCaptureScreenshot{
			Format:  proto.PageCaptureScreenshotFormatPng,
			Quality: &quality,
		})
	}

	// Return base64 or save to file
	// For now, return info about the screenshot
	return fmt.Sprintf("📸 Screenshot captured (%d bytes)\n🔗 %s", len(data), targetURL), nil
}

// getLinks extracts all links (JavaScript rendered)
func (b *Browser) getLinks(ctx context.Context, targetURL, selector string) (string, error) {
	page := b.browser.MustPage(targetURL)
	defer page.MustClose()

	page.MustWaitLoad()

	linkSelector := "a[href]"
	if selector != "" {
		linkSelector = selector + " a[href]"
	}

	elements, err := page.Elements(linkSelector)
	if err != nil {
		return "", err
	}

	var links []struct {
		Text string
		URL  string
	}

	for _, el := range elements {
		href, _ := el.Attribute("href")
		text := el.MustText()

		if href != nil && *href != "" && *href != "#" && !strings.HasPrefix(*href, "javascript:") {
			links = append(links, struct {
				Text string
				URL  string
			}{strings.TrimSpace(text), *href})
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔗 **Links from: %s**\n\n", targetURL))

	limit := 20
	if len(links) < limit {
		limit = len(links)
	}

	for i := 0; i < limit; i++ {
		l := links[i]
		if l.Text != "" {
			sb.WriteString(fmt.Sprintf("• %s: %s\n", truncate(l.Text, 50), l.URL))
		} else {
			sb.WriteString(fmt.Sprintf("• %s\n", l.URL))
		}
	}

	if len(links) > 20 {
		sb.WriteString(fmt.Sprintf("\n... and %d more links", len(links)-20))
	}

	return sb.String(), nil
}

// click clicks an element
func (b *Browser) click(ctx context.Context, targetURL, selector string) (string, error) {
	if selector == "" {
		return "", fmt.Errorf("selector required for click action")
	}

	page := b.browser.MustPage(targetURL)
	defer page.MustClose()

	page.MustWaitLoad()

	el, err := page.Element(selector)
	if err != nil {
		return "", fmt.Errorf("element not found: %s", selector)
	}

	el.MustClick()
	time.Sleep(1 * time.Second) // Wait for action

	return fmt.Sprintf("✅ Clicked element: %s", selector), nil
}

// typeText types text into an element
func (b *Browser) typeText(ctx context.Context, targetURL, selector, value string) (string, error) {
	if selector == "" {
		return "", fmt.Errorf("selector required for type action")
	}

	page := b.browser.MustPage(targetURL)
	defer page.MustClose()

	page.MustWaitLoad()

	el, err := page.Element(selector)
	if err != nil {
		return "", fmt.Errorf("element not found: %s", selector)
	}

	el.MustInput(value)

	return fmt.Sprintf("✅ Typed into element: %s", selector), nil
}

// blockedJSPatterns are dangerous JavaScript patterns that could be abused
var blockedJSPatterns = []string{
	"fetch(",
	"XMLHttpRequest",
	"document.cookie",
	"localStorage",
	"sessionStorage",
	"window.open(",
	"eval(",
	"Function(",
	"importScripts",
	"navigator.sendBeacon",
	"WebSocket(",
	"EventSource(",
	"window.location",
	"document.location",
	"location.href",
	"location.assign",
	"location.replace",
	"postMessage(",
	"crypto.subtle",
}

// evaluate runs JavaScript on the page
func (b *Browser) evaluate(ctx context.Context, targetURL, script string) (string, error) {
	if script == "" {
		return "", fmt.Errorf("script required for eval action")
	}

	// Block dangerous JS patterns
	scriptLower := strings.ToLower(script)
	for _, pattern := range blockedJSPatterns {
		if strings.Contains(scriptLower, strings.ToLower(pattern)) {
			return "", fmt.Errorf("blocked: script contains disallowed pattern %q", pattern)
		}
	}

	page := b.browser.MustPage(targetURL)
	defer page.MustClose()

	page.MustWaitLoad()

	result := page.MustEval(script)

	return fmt.Sprintf("📜 Result: %v", result), nil
}

// Search performs a Google search using browser
func (b *Browser) Search(ctx context.Context, query string, count int) (string, error) {
	if b.browser == nil {
		if err := b.Start(); err != nil {
			return "", err
		}
	}

	page := b.browser.MustPage("https://www.google.com")
	defer page.MustClose()

	// Type search query and submit
	page.MustElement("textarea[name=q]").MustInput(query + "\n")

	// Wait for results
	page.MustWaitLoad()
	time.Sleep(2 * time.Second)

	// Extract results
	elements, _ := page.Elements("div.g")

	var results []struct {
		Title   string
		URL     string
		Snippet string
	}

	for _, el := range elements {
		if len(results) >= count {
			break
		}

		titleEl, _ := el.Element("h3")
		linkEl, _ := el.Element("a")
		snippetEl, _ := el.Element("div[data-sncf]")

		if titleEl != nil && linkEl != nil {
			href, _ := linkEl.Attribute("href")
			result := struct {
				Title   string
				URL     string
				Snippet string
			}{
				Title: titleEl.MustText(),
				URL:   *href,
			}
			if snippetEl != nil {
				result.Snippet = snippetEl.MustText()
			}
			results = append(results, result)
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **Google Search: %s**\n\n", query))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("**%d. %s**\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   🔗 %s\n", r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", truncate(r.Snippet, 150)))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
