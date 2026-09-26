package rbac

import (
	"database/sql"
	"net/http"
	"strings"
)

// Middleware returns an HTTP middleware that validates Bearer tokens.
// Paths in skipPaths are exempt from authentication.
func Middleware(db *sql.DB, skipPaths ...string) func(http.Handler) http.Handler {
	store := New(db)
	skip := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}

			rawToken := strings.TrimPrefix(auth, "Bearer ")
			if _, err := store.LookupToken(r.Context(), rawToken); err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
