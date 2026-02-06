// Maps integration using Nominatim (OpenStreetMap) - 100% Free
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
)

// Maps tool using Nominatim (OpenStreetMap) - completely free
type Maps struct {
	client *http.Client
}

// MapsConfig for Maps tool
type MapsConfig struct {
	// No config needed - Nominatim is free!
}

// NewMaps creates a new Maps tool
func NewMaps(cfg *MapsConfig) *Maps {
	return &Maps{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns tool name
func (m *Maps) Name() string {
	return "maps"
}

// Description returns tool description
func (m *Maps) Description() string {
	return "Search places, geocode addresses (OpenStreetMap). Params: action (search|geocode|reverse), q (query), lat/lon (coordinates)"
}

// Execute performs maps operation
func (m *Maps) Execute(ctx context.Context, params map[string]string) (string, error) {
	action := params["action"]
	if action == "" {
		action = "search"
	}

	switch action {
	case "search":
		return m.searchPlaces(ctx, params)
	case "geocode":
		return m.geocode(ctx, params)
	case "reverse":
		return m.reverseGeocode(ctx, params)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// searchPlaces searches for places/POIs using Nominatim
func (m *Maps) searchPlaces(ctx context.Context, params map[string]string) (string, error) {
	query := params["q"]
	if query == "" {
		return "", fmt.Errorf("missing required parameter: q")
	}

	u, _ := url.Parse("https://nominatim.openstreetmap.org/search")
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("limit", "5")
	q.Set("addressdetails", "1")
	q.Set("extratags", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Magabot/1.0 (https://github.com/kusa/magabot)")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Nominatim API error: %s", string(body))
	}

	var results []struct {
		DisplayName string `json:"display_name"`
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
		Type        string `json:"type"`
		Class       string `json:"class"`
		Address     struct {
			Road        string `json:"road"`
			City        string `json:"city"`
			State       string `json:"state"`
			Country     string `json:"country"`
			Postcode    string `json:"postcode"`
		} `json:"address"`
		Extratags struct {
			Phone   string `json:"phone"`
			Website string `json:"website"`
			Opening string `json:"opening_hours"`
		} `json:"extratags"`
	}

	if err := json.Unmarshal(body, &results); err != nil {
		return "", err
	}

	if len(results) == 0 {
		return fmt.Sprintf("❌ No places found for: %s", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📍 **Places found for: %s**\n\n", query))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("**%d. %s**\n", i+1, truncate(r.DisplayName, 80)))
		
		// Type info
		if r.Type != "" {
			sb.WriteString(fmt.Sprintf("   📌 Type: %s\n", r.Type))
		}
		
		// Contact info
		if r.Extratags.Phone != "" {
			sb.WriteString(fmt.Sprintf("   📞 %s\n", r.Extratags.Phone))
		}
		if r.Extratags.Website != "" {
			sb.WriteString(fmt.Sprintf("   🌐 %s\n", truncate(r.Extratags.Website, 50)))
		}
		if r.Extratags.Opening != "" {
			sb.WriteString(fmt.Sprintf("   🕐 %s\n", r.Extratags.Opening))
		}
		
		sb.WriteString(fmt.Sprintf("   🗺️ %s, %s\n\n", r.Lat, r.Lon))
	}

	return sb.String(), nil
}

// geocode converts address to coordinates
func (m *Maps) geocode(ctx context.Context, params map[string]string) (string, error) {
	return m.searchPlaces(ctx, params)
}

// reverseGeocode converts coordinates to address
func (m *Maps) reverseGeocode(ctx context.Context, params map[string]string) (string, error) {
	lat := params["lat"]
	lon := params["lon"]
	if lat == "" || lon == "" {
		return "", fmt.Errorf("missing required parameters: lat, lon")
	}

	u := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%s&lon=%s&format=json&addressdetails=1",
		url.QueryEscape(lat), url.QueryEscape(lon))

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Magabot/1.0 (https://github.com/kusa/magabot)")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Nominatim API error: %s", string(body))
	}

	var result struct {
		DisplayName string `json:"display_name"`
		Address     struct {
			Road     string `json:"road"`
			City     string `json:"city"`
			State    string `json:"state"`
			Country  string `json:"country"`
			Postcode string `json:"postcode"`
		} `json:"address"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("📍 **Location Info**\n\n")
	sb.WriteString(fmt.Sprintf("📫 %s\n", result.DisplayName))
	sb.WriteString(fmt.Sprintf("🗺️ Coordinates: %s, %s\n", lat, lon))

	if result.Address.City != "" {
		sb.WriteString(fmt.Sprintf("🏙️ City: %s\n", result.Address.City))
	}
	if result.Address.State != "" {
		sb.WriteString(fmt.Sprintf("🗾 State: %s\n", result.Address.State))
	}
	if result.Address.Country != "" {
		sb.WriteString(fmt.Sprintf("🌍 Country: %s\n", result.Address.Country))
	}

	return sb.String(), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
