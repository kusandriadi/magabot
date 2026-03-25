// Weather integration using wttr.in (free, no API key)
package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kusa/magabot/internal/util"
)

// Weather tool using wttr.in
type Weather struct {
	client *http.Client
}

// NewWeather creates a new Weather tool
func NewWeather() *Weather {
	return &Weather{
		client: util.NewHTTPClient(0),
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

	resp, err := util.DoGET(ctx, w.client, u, map[string]string{"User-Agent": util.DefaultUserAgent})
	if err != nil {
		return "", fmt.Errorf("weather request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := util.ReadHTTPResponse(resp, "weather API")
	if err != nil {
		return "", err
	}
	result := string(body)

	// Check for error responses
	if strings.Contains(result, "Unknown location") {
		return fmt.Sprintf("❌ Location not found: %s", location), nil
	}

	return "🌤️ **Weather Report**\n\n" + strings.ReplaceAll(result, "+", " "), nil
}

// GetForecast gets multi-day forecast
func (w *Weather) GetForecast(ctx context.Context, location string, days int) (string, error) {
	// wttr.in text forecast
	u := fmt.Sprintf("https://wttr.in/%s?format=4&lang=id", url.PathEscape(location))

	resp, err := util.DoGET(ctx, w.client, u, map[string]string{"User-Agent": util.DefaultUserAgent})
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := util.ReadHTTPBody(resp, 0)
	if err != nil {
		return "", fmt.Errorf("read forecast response: %w", err)
	}
	return string(body), nil
}

// GetCurrentSimple gets simple current weather
func (w *Weather) GetCurrentSimple(ctx context.Context, location string) (string, error) {
	return w.Execute(ctx, map[string]string{
		"location": location,
		"format":   "emoji",
	})
}
