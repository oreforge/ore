package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/oreforge/ore/internal/server/errs"
)

func BearerAuth(token string) func(http.Handler) http.Handler {
	expected := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var got []byte

			auth := r.Header.Get("Authorization")
			switch {
			case strings.HasPrefix(auth, "Bearer "):
				got = []byte(strings.TrimPrefix(auth, "Bearer "))
			case isWebSocketUpgrade(r) && r.URL.Query().Get("token") != "":
				got = []byte(r.URL.Query().Get("token"))
			default:
				errs.Write(w, http.StatusUnauthorized, "missing or invalid authorization header")
				return
			}

			if subtle.ConstantTimeCompare(got, expected) != 1 {
				errs.Write(w, http.StatusUnauthorized, "invalid token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
