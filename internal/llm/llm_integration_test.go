// Integration tests for LLM package - these run against real APIs when keys are set
package llm

import (
	"os"
	"strings"
	"testing"
)

// skipIfNoKey skips the test if none of the provided environment variables are set
func skipIfNoKey(t *testing.T, envVars ...string) {
	for _, env := range envVars {
		if os.Getenv(env) != "" {
			return
		}
	}
	t.Skipf("skipping: none of %v set", envVars)
}

func TestIntegration_Anthropic(t *testing.T) {
	skipIfNoKey(t, "ANTHROPIC_API_KEY")

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	models, err := FetchModels("anthropic", apiKey, "")
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}

	if len(models) == 0 {
		t.Fatal("No models returned")
	}

	t.Logf("Found %d Anthropic models", len(models))
	for _, m := range models {
		if strings.Contains(m.ID, "claude") {
			t.Logf("  - %s (%s)", m.ID, m.Provider)
			break
		}
	}
}

func TestIntegration_OpenAI(t *testing.T) {
	skipIfNoKey(t, "OPENAI_API_KEY")

	apiKey := os.Getenv("OPENAI_API_KEY")
	models, err := FetchModels("openai", apiKey, "")
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}

	if len(models) == 0 {
		t.Fatal("No models returned")
	}

	t.Logf("Found %d OpenAI models", len(models))
	for _, m := range models {
		if strings.Contains(m.ID, "gpt-4") {
			t.Logf("  - %s (%s)", m.ID, m.Provider)
			break
		}
	}
}

func TestIntegration_Gemini(t *testing.T) {
	skipIfNoKey(t, "GEMINI_API_KEY", "GOOGLE_API_KEY")

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}

	models, err := FetchModels("gemini", apiKey, "")
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}

	if len(models) == 0 {
		t.Fatal("No models returned")
	}

	t.Logf("Found %d Gemini models", len(models))
	for _, m := range models {
		if strings.Contains(m.ID, "gemini") {
			t.Logf("  - %s (%s)", m.ID, m.Provider)
			break
		}
	}
}

func TestFetchModels_UnsupportedProvider(t *testing.T) {
	_, err := FetchModels("invalid-provider", "key", "")
	if err == nil {
		t.Fatal("Expected error for unsupported provider")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("Error should mention unsupported provider, got: %v", err)
	}
}

func TestFetchModels_InvalidBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr string
	}{
		{
			name:    "invalid scheme",
			baseURL: "ftp://example.com",
			wantErr: "invalid URL scheme",
		},
		{
			name:    "metadata endpoint",
			baseURL: "http://169.254.169.254/latest/meta-data/",
			wantErr: "blocked host",
		},
		{
			name:    "localhost blocked for cloud provider",
			baseURL: "http://localhost:8080",
			wantErr: "blocked host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FetchModels("openai", "fake-key", tt.baseURL)
			if err == nil {
				t.Fatalf("Expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestFetchModels_LocalAllowsLocalhost(t *testing.T) {
	// Local provider should allow localhost URLs
	_, err := FetchModels("local", "", "http://localhost:11434/v1")
	// Should fail with connection error, NOT URL validation error
	if err != nil && strings.Contains(err.Error(), "blocked host") {
		t.Error("Local provider should allow localhost URLs")
	}
}
