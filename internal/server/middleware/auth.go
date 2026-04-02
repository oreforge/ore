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
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				errs.Write(w, http.StatusUnauthorized, "missing or invalid authorization header")
				return
			}
			got := []byte(strings.TrimPrefix(auth, "Bearer "))
			if subtle.ConstantTimeCompare(got, expected) != 1 {
				errs.Write(w, http.StatusUnauthorized, "invalid token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
