package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"
)

type Config struct {
	TableName     string
	Bucket        string
	AuthIssuer    string
	AuthClientID  string
	AllowedOrigin string
}

func Load() (Config, error) {
	cfg := Config{
		TableName:     os.Getenv("DYNAMODB_TABLE"),
		Bucket:        os.Getenv("S3_BUCKET"),
		AuthIssuer:    os.Getenv("AUTH_ISSUER"),
		AuthClientID:  os.Getenv("AUTH_CLIENT_ID"),
		AllowedOrigin: os.Getenv("ALLOWED_ORIGIN"),
	}
	if strings.TrimSpace(cfg.TableName) == "" {
		return Config{}, fmt.Errorf("environment variable DYNAMODB_TABLE is not set")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return Config{}, fmt.Errorf("environment variable S3_BUCKET is not set")
	}
	if err := ValidateAuth(cfg.AuthIssuer, cfg.AuthClientID); err != nil {
		return Config{}, err
	}
	if err := ValidateAllowedOrigin(cfg.AllowedOrigin); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ValidateAuth rejects incomplete or unsafe issuer/client configuration before
// constructing a verifier. The issuer must exactly match the token's iss claim.
func ValidateAuth(issuer, clientID string) error {
	u, err := parseWebURL(issuer)
	if err != nil || u.Scheme != "https" || strings.HasSuffix(issuer, "/") {
		return fmt.Errorf("AUTH_ISSUER must be an HTTPS issuer URL without credentials, query, fragment, or trailing slash")
	}
	if clientID == "" || strings.ContainsFunc(clientID, unicode.IsSpace) {
		return fmt.Errorf("AUTH_CLIENT_ID must be a nonempty client ID without whitespace")
	}
	return nil
}

// ValidateAllowedOrigin accepts one exact browser origin. Plain HTTP is only
// permitted for loopback development origins; paths and wildcards are invalid.
func ValidateAllowedOrigin(origin string) error {
	u, err := parseWebURL(origin)
	if err != nil || u.Path != "" || u.RawPath != "" {
		return fmt.Errorf("ALLOWED_ORIGIN must be one exact origin without a path, credentials, query, or fragment")
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("ALLOWED_ORIGIN must use HTTPS except for loopback development")
}

func parseWebURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(raw, "*#") || strings.ContainsFunc(raw, unicode.IsSpace) {
		return nil, fmt.Errorf("invalid URL")
	}
	if !validHost(u.Hostname()) || strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("invalid hostname")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid port")
		}
	}
	return u, nil
}

func validHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
