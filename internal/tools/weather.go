// Weather integration using wttr.in (free, no API key)
package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Weather tool using wttr.in
type Weather struct {
	client *http.Client
}

// NewWeather creates a new Weather tool
func NewWeather() *Weather {
	return &Weather{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns tool name
func (w *Weather) Name() string {
	return "weather"
}

// Description returns tool description
func (w *Weather) Description() string {
	return "Get weather information. Params: location (city name or coordinates)"
}

// Execute gets weather for a location
func (w *Weather) Execute(ctx context.Context, params map[string]string) (string, error) {
	location := params["location"]
	if location == "" {
		location = params["q"]
	}
	if location == "" {
		return "", fmt.Errorf("missing required parameter: location")
	}

	// wttr.in format: ?format=...
	// %l = location, %c = condition icon, %C = condition text
	// %t = temperature, %h = humidity, %w = wind
	// %p = precipitation, %P = pressure
	
	format := params["format"]
	if format == "" {
		format = "detailed"
	}

	var wttrFormat string
	switch format {
	case "simple":
		wttrFormat = "%l:+%c+%t+%C"
	case "detailed":
		wttrFormat = "%l\\n🌡️+Temperature:+%t+(feels+like+%f)\\n💧+Humidity:+%h\\n💨+Wind:+%w\\n🌧️+Precipitation:+%p\\n📊+Pressure:+%P\\n☁️+Condition:+%C"
	case "emoji":
		wttrFormat = "%c+%t"
	default:
		wttrFormat = "%l:+%c+%t+%C"
	}

	u := fmt.Sprintf("https://wttr.in/%s?format=%s",
		url.PathEscape(location),
		url.PathEscape(wttrFormat))

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Magabot/1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("weather request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	result := string(body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("weather API error: %s", result)
	}

	// Check for error responses
	if strings.Contains(result, "Unknown location") {
		return fmt.Sprintf("❌ Location not found: %s", location), nil
	}

	return "🌤️ **Weather Report**\n\n" + strings.ReplaceAll(result, "+", " "), nil
}

// GetForecast gets multi-day forecast
func (w *Weather) GetForecast(ctx context.Context, location string, days int) (string, error) {
	if days <= 0 {
		days = 3
	}
	if days > 3 {
		days = 3
	}

	// wttr.in text forecast
	u := fmt.Sprintf("https://wttr.in/%s?format=4&lang=id", url.PathEscape(location))

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Magabot/1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

// GetCurrentSimple gets simple current weather
func (w *Weather) GetCurrentSimple(ctx context.Context, location string) (string, error) {
	return w.Execute(ctx, map[string]string{
		"location": location,
		"format":   "emoji",
	})
}
