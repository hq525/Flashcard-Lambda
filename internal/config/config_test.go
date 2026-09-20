package config

import (
	"strings"
	"testing"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DYNAMODB_TABLE", "flash-card-app-dev")
	t.Setenv("S3_BUCKET", "flash-card-app-images-dev")
	t.Setenv("AUTH_ISSUER", "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_example")
	t.Setenv("AUTH_CLIENT_ID", "exampleclient123")
	t.Setenv("ALLOWED_ORIGIN", "https://flashcards.example.com")
}

func TestLoad(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.TableName != "flash-card-app-dev" || cfg.Bucket != "flash-card-app-images-dev" {
		t.Errorf("unexpected resource configuration: %+v", cfg)
	}
}

func TestLoadRejectsMissingSettings(t *testing.T) {
	for _, key := range []string{"DYNAMODB_TABLE", "S3_BUCKET", "AUTH_ISSUER", "AUTH_CLIENT_ID", "ALLOWED_ORIGIN"} {
		t.Run(key, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(key, "")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Load() missing %s = %v; want configuration error naming setting", key, err)
			}
		})
	}
}

func TestLoadRejectsInvalidAuthenticationSettings(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"AUTH_ISSUER", "http://cognito-idp.us-east-1.amazonaws.com/pool"},
		{"AUTH_ISSUER", "https://user:password@example.com/pool"},
		{"AUTH_ISSUER", "https://example.com/pool?key=value"},
		{"AUTH_ISSUER", "https://example.com/pool#fragment"},
		{"AUTH_ISSUER", "https://example.com/pool/"},
		{"AUTH_ISSUER", "https://"},
		{"AUTH_ISSUER", "https://example.com:99999/pool"},
		{"AUTH_CLIENT_ID", " "},
		{"AUTH_CLIENT_ID", "client\nother"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() accepted invalid %s", tc.key)
			}
		})
	}
}

func TestLoadRejectsInvalidOrigins(t *testing.T) {
	for _, origin := range []string{
		"*", "null", "https://*", "https://example.com/", "https://example.com/path",
		"https://example.com?query", "https://example.com#fragment", "https://user@example.com",
		"http://example.com", "https://example.com:99999", "https://example.com https://other.example.com",
		"https://example.com:", "https://.", "https://bad..example.com", "https://-bad.example.com",
	} {
		t.Run(origin, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv("ALLOWED_ORIGIN", origin)
			if _, err := Load(); err == nil {
				t.Fatal("Load() accepted invalid ALLOWED_ORIGIN")
			}
		})
	}
}

func TestLoadAllowsLoopbackDevelopmentOrigins(t *testing.T) {
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://[::1]:5173"} {
		t.Run(origin, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv("ALLOWED_ORIGIN", origin)
			if _, err := Load(); err != nil {
				t.Fatalf("Load() rejected loopback development origin: %v", err)
			}
		})
	}
}
