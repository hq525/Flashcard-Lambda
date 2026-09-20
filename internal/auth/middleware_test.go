package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

func generateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signToken(t *testing.T, key *rsa.PrivateKey, algorithm string, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": algorithm, "kid": "test-key", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	raw := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	var signature []byte
	if algorithm != "none" {
		hash := crypto.SHA256
		digest := sha256.Sum256([]byte(raw))
		bytes := digest[:]
		if algorithm == "RS512" {
			hash = crypto.SHA512
			digest := sha512.Sum512([]byte(raw))
			bytes = digest[:]
		}
		signature, err = rsa.SignPKCS1v15(rand.Reader, key, hash, bytes)
		if err != nil {
			t.Fatal(err)
		}
	}
	return raw + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func serveKeys(w http.ResponseWriter, key *rsa.PrivateKey) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test-key",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}})
}

func validClaims(issuer string) map[string]any {
	return map[string]any{
		"iss": issuer, "aud": "testclient", "sub": "owner-subject", "token_use": "id",
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
		"cognito:groups": []string{"owner"},
	}
}

// Removing any signature, issuer, audience, expiry, token-use, or owner check
// must allow a request here that is currently denied before the handler runs.
func TestMiddlewareRequiresVerifiedOwnerIDToken(t *testing.T) {
	key := generateKey(t)
	forgedKey := generateKey(t)
	var keyRequests atomic.Int64
	keys := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keyRequests.Add(1)
		if r.URL.Path != "/pool/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		serveKeys(w, key)
	}))
	defer keys.Close()
	issuer := keys.URL + "/pool"
	ctx := oidc.ClientContext(context.Background(), keys.Client())
	middleware, err := NewMiddleware(ctx, issuer, "testclient")
	if err != nil {
		t.Fatal(err)
	}
	if keyRequests.Load() != 0 {
		t.Fatal("constructing middleware must not make discovery or JWKS requests")
	}

	for _, tc := range []struct {
		name      string
		mutate    func(map[string]any)
		algorithm string
		signer    *rsa.PrivateKey
		header    string
		duplicate bool
		want      int
	}{
		{name: "owner", want: http.StatusNoContent},
		{name: "owner among groups", mutate: func(c map[string]any) { c["cognito:groups"] = []string{"reader", "owner"} }, want: http.StatusNoContent},
		{name: "duplicate authorization", duplicate: true, want: http.StatusUnauthorized},
		{name: "anonymous", header: "missing", want: http.StatusUnauthorized},
		{name: "malformed", header: "Bearer not-a-jwt", want: http.StatusUnauthorized},
		{name: "basic auth", header: "Basic credentials", want: http.StatusUnauthorized},
		{name: "oversized", header: "Bearer " + strings.Repeat("x", 20*1024), want: http.StatusUnauthorized},
		{name: "expired", mutate: func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, want: http.StatusUnauthorized},
		{name: "missing expiry", mutate: func(c map[string]any) { delete(c, "exp") }, want: http.StatusUnauthorized},
		{name: "wrong issuer", mutate: func(c map[string]any) { c["iss"] = "https://other.example.com/pool" }, want: http.StatusUnauthorized},
		{name: "wrong audience", mutate: func(c map[string]any) { c["aud"] = "otherclient" }, want: http.StatusUnauthorized},
		{name: "missing audience", mutate: func(c map[string]any) { delete(c, "aud") }, want: http.StatusUnauthorized},
		{name: "missing subject", mutate: func(c map[string]any) { delete(c, "sub") }, want: http.StatusUnauthorized},
		{name: "access token", mutate: func(c map[string]any) { c["token_use"] = "access" }, want: http.StatusUnauthorized},
		{name: "missing token use", mutate: func(c map[string]any) { delete(c, "token_use") }, want: http.StatusUnauthorized},
		{name: "nonowner", mutate: func(c map[string]any) { c["cognito:groups"] = []string{"reader"} }, want: http.StatusForbidden},
		{name: "owner substring", mutate: func(c map[string]any) { c["cognito:groups"] = []string{"not-owner"} }, want: http.StatusForbidden},
		{name: "missing groups", mutate: func(c map[string]any) { delete(c, "cognito:groups") }, want: http.StatusForbidden},
		{name: "malformed groups", mutate: func(c map[string]any) { c["cognito:groups"] = "owner" }, want: http.StatusUnauthorized},
		{name: "forged signature and owner claim", signer: forgedKey, want: http.StatusUnauthorized},
		{name: "unsigned", algorithm: "none", want: http.StatusUnauthorized},
		{name: "wrong signing algorithm", algorithm: "RS512", want: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := validClaims(issuer)
			if tc.mutate != nil {
				tc.mutate(claims)
			}
			signer, algorithm := tc.signer, tc.algorithm
			if signer == nil {
				signer = key
			}
			if algorithm == "" {
				algorithm = "RS256"
			}
			header := tc.header
			if header == "" {
				header = "Bearer " + signToken(t, signer, algorithm, claims)
			}
			called := false
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/category", nil)
			if header != "missing" {
				request.Header.Set("Authorization", header)
			}
			if tc.duplicate {
				request.Header.Add("Authorization", header)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want || called != (tc.want == http.StatusNoContent) {
				t.Fatalf("status = %d, handler called = %t; want status %d", response.Code, called, tc.want)
			}
			if tc.want == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Error("unauthorized response is missing Bearer challenge")
			}
		})
	}
}

func TestMiddlewareRejectsAnonymousRequestsForEveryMethod(t *testing.T) {
	middleware, err := NewMiddleware(context.Background(), "https://issuer.example.com/pool", "testclient")
	if err != nil {
		t.Fatal(err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("anonymous request reached private handler")
	}))
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(method, "/category", nil))
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d; want unauthorized", response.Code)
			}
		})
	}
}

func TestMiddlewareRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ issuer, clientID string }{
		{"", "client"}, {"http://issuer.example.com/pool", "client"}, {"https://issuer.example.com/pool", ""},
		{"https://user:password@issuer.example.com/pool", "client"}, {"https://issuer.example.com/pool?other", "client"},
	} {
		if _, err := NewMiddleware(context.Background(), tc.issuer, tc.clientID); err == nil {
			t.Fatalf("NewMiddleware(%q, %q) accepted unsafe configuration", tc.issuer, tc.clientID)
		}
	}
}

func TestMiddlewareFailsClosedWhenKeysUnavailable(t *testing.T) {
	key := generateKey(t)
	keys := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer keys.Close()
	middleware, err := NewMiddleware(oidc.ClientContext(context.Background(), keys.Client()), keys.URL+"/pool", "testclient")
	if err != nil {
		t.Fatal(err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unverified token reached private handler")
	}))
	request := httptest.NewRequest(http.MethodGet, "/category", nil)
	request.Header.Set("Authorization", "Bearer "+signToken(t, key, "RS256", validClaims(keys.URL+"/pool")))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want unauthorized", response.Code)
	}
}
