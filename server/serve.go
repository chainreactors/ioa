package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chainreactors/ioa"
)

type ServerOptions struct {
	URL        string
	DB         string
	AccessKey  string
	Store      Store
	Middleware func(http.Handler, *Service) http.Handler
	Logger     ioa.Logger
}

func RunServer(ctx context.Context, opts ServerOptions) error {
	logger := opts.Logger
	if logger == nil {
		logger = ioa.NopLogger()
	}

	store := opts.Store
	if store == nil {
		store = NewMemoryStore()
	}

	listenURL := opts.URL
	if listenURL == "" {
		listenURL = "http://127.0.0.1:8765"
	}
	addr, err := listenAddrFromURL(listenURL)
	if err != nil {
		return err
	}
	if opts.AccessKey == "" {
		return fmt.Errorf("--access-key is required")
	}
	service := NewService(store, opts.AccessKey)
	var handler http.Handler = NewHandler(service)
	handler = AuthMiddleware(service)(handler)
	if opts.Middleware != nil {
		handler = opts.Middleware(handler, service)
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	logger.Importantf("ioa_server status=starting url=%s", listenURL)
	err = srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func listenAddrFromURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "127.0.0.1:8765", nil
	}
	if !strings.Contains(raw, "://") {
		return "", fmt.Errorf("invalid url %q: expected URL", raw)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid url %q: %s", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid url %q: expected http or https", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid url %q: missing host", raw)
	}
	return parsed.Host, nil
}
