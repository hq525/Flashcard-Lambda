// Package auth verifies Cognito ID tokens before allowing access to the owner's
// private library. Neither API Gateway headers nor decoded claims are trusted.
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"flashcard_lambda/internal/config"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	verificationTimeout  = 5 * time.Second
	maxAuthorizationSize = 16 * 1024
)

// NewMiddleware creates one cached verifier for the configured Cognito issuer.
// No network call is made until a signed token needs its issuer's public keys.
// All methods require authentication; a surrounding CORS handler may separately
// answer valid browser preflight requests without reaching this middleware.
func NewMiddleware(ctx context.Context, issuer, clientID string) (func(http.Handler) http.Handler, error) {
	if err := config.ValidateAuth(issuer, clientID); err != nil {
		return nil, err
	}

	// RemoteKeySet refreshes in the background, independently of request
	// cancellation. Bound the client as well as each verification request.
	client := http.DefaultClient
	if configured, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok && configured != nil {
		client = configured
	}
	boundedClient := *client
	if boundedClient.Timeout <= 0 || boundedClient.Timeout > verificationTimeout {
		boundedClient.Timeout = verificationTimeout
	}
	boundedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	keyContext := oidc.ClientContext(ctx, &boundedClient)
	keys := oidc.NewRemoteKeySet(keyContext, issuer+"/.well-known/jwks.json")
	verifier := oidc.NewVerifier(issuer, keys, &oidc.Config{
		ClientID:             clientID,
		SupportedSigningAlgs: []string{oidc.RS256},
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			headers := r.Header.Values("Authorization")
			if len(headers) != 1 || len(headers[0]) > maxAuthorizationSize {
				deny(w, http.StatusUnauthorized)
				return
			}
			parts := strings.Fields(headers[0])
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				deny(w, http.StatusUnauthorized)
				return
			}
			verificationContext, cancel := context.WithTimeout(r.Context(), verificationTimeout)
			defer cancel()
			token, err := verifier.Verify(verificationContext, parts[1])
			if err != nil || token.Subject == "" {
				deny(w, http.StatusUnauthorized)
				return
			}
			var claims struct {
				TokenUse string   `json:"token_use"`
				Groups   []string `json:"cognito:groups"`
			}
			if token.Claims(&claims) != nil || claims.TokenUse != "id" {
				deny(w, http.StatusUnauthorized)
				return
			}
			for _, group := range claims.Groups {
				if group == "owner" {
					next.ServeHTTP(w, r)
					return
				}
			}
			deny(w, http.StatusForbidden)
		})
	}, nil
}

func deny(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("{\"error\":\"unauthorized\"}\n"))
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte("{\"error\":\"forbidden\"}\n"))
}
