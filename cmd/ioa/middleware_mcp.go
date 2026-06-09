//go:build mcp

package main

import (
	"net/http"

	ioamcp "github.com/chainreactors/ioa/mcp"
	"github.com/chainreactors/ioa/server"
)

func init() {
	withOptionalMiddleware = withMCPMiddleware
}

func withMCPMiddleware(handler http.Handler, service *server.Service) http.Handler {
	return ioamcp.WithMCP(handler, service)
}
