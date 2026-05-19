package server

import (
	"net/http"
	"strings"
)

func AuthMiddleware(service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimRight(r.URL.Path, "/")
			if path == "/health" || path == "/ready" || strings.HasPrefix(path, "/auth") {
				next.ServeHTTP(w, r)
				return
			}

			if ak := r.Header.Get("X-Access-Key"); ak != "" {
				if ak == service.AccessKey() {
					next.ServeHTTP(w, r)
					return
				}
				writeError(w, http.StatusForbidden, "invalid access key")
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "authorization required: provide token or access key")
				return
			}
			token := strings.TrimPrefix(auth, "Bearer ")
			node, err := service.ResolveToken(token)
			if err != nil {
				writeServiceError(w, err)
				return
			}

			if r.Header.Get("X-Node-ID") == "" {
				r.Header.Set("X-Node-ID", node.ID)
			}
			next.ServeHTTP(w, r)
		})
	}
}
