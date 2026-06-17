package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chainreactors/ioa/protocols"
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

	accessKey := opts.AccessKey
	if accessKey == "" {
		accessKey = protocols.NewToken()
	}

	service := NewService(store, accessKey)

	mux := http.NewServeMux()
	mux.Handle("/mcp", newMCPHandler(service))
	mux.Handle("/", NewHandler(service))

	handler := AuthMiddleware(service)(mux)

	srv := &http.Server{Addr: host, Handler: handler}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	clientURL := url.URL{Scheme: scheme, User: url.User(accessKey), Host: host}
	log.Printf("[*] ioa_server status=starting url=%s", clientURL.String())

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
