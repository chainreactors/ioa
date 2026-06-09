package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ServerOptions struct {
	URL       string
	AccessKey string
	Store     Store
}

func RunServer(ctx context.Context, opts ServerOptions) error {
	store := opts.Store
	if store == nil {
		store = NewMemoryStore()
	}

	listenURL := opts.URL
	if listenURL == "" {
		listenURL = "http://127.0.0.1:8765"
	}
	parsed, err := url.Parse(listenURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	host := parsed.Host
	if host == "" {
		host = "127.0.0.1:8765"
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (use http or https)", scheme)
	}

	service := NewService(store, opts.AccessKey)

	mux := http.NewServeMux()
	mux.Handle("/mcp", newMCPHandler(service))
	mux.Handle("/", NewHandler(service))

	var handler http.Handler = mux
	if opts.AccessKey != "" {
		handler = AuthMiddleware(service)(mux)
	}

	srv := &http.Server{Addr: host, Handler: handler}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("[*] ioa_server status=starting url=%s://%s", scheme, host)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
