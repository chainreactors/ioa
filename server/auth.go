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

			if path == "/spaces" && r.Method == http.MethodPost {
				if !authorizeAccessKey(w, r, service) {
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if !authorizeToken(w, r, service) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func authorizeAccessKey(w http.ResponseWriter, r *http.Request, service *Service) bool {
	ak := r.Header.Get("X-Access-Key")
	if ak == "" {
		writeError(w, http.StatusUnauthorized, "access key required")
		return false
	}
	if ak != service.AccessKey() {
		writeError(w, http.StatusForbidden, "invalid access key")
		return false
	}

	// If the caller also supplied a bearer token, bind the caller node for
	// handlers that need a sender/member identity, while keeping access-key
	// authorization scoped to this bootstrap endpoint.
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return true
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "invalid authorization header")
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	node, err := service.ResolveToken(token)
	if err != nil {
		writeServiceError(w, err)
		return false
	}
	if r.Header.Get("X-Node-ID") == "" {
		r.Header.Set("X-Node-ID", node.ID)
	}
	return true
}

func authorizeToken(w http.ResponseWriter, r *http.Request, service *Service) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "authorization token required")
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	node, err := service.ResolveToken(token)
	if err != nil {
		writeServiceError(w, err)
		return false
	}

	if r.Header.Get("X-Node-ID") == "" {
		r.Header.Set("X-Node-ID", node.ID)
	}
	return true
}
