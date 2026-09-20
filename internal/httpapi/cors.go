package httpapi

import (
	"context"
	"net/http"
	"time"
)

// CORS sits outside authentication so an allowed browser can read error responses
// and perform a preflight without presenting a token. It is not an auth gate.
func withCORS(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Vary", "Origin")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		origin := r.Header.Get("Origin")
		if origin != "" && (allowedOrigin == "" || origin != allowedOrigin) {
			writeError(w, http.StatusForbidden)
			return
		}
		if origin != "" && origin == allowedOrigin {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
			h.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		}
		if r.Method == http.MethodOptions {
			if origin == "" || origin != allowedOrigin {
				writeError(w, http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
