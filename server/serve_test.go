package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewHTTPHandlerComposesRESTAuthAndMCP(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()
	handler := NewHTTPHandler(NewService(store, "test-key"))
	for _, test := range []struct {
		path   string
		status int
	}{{"/health", 200}, {"/spaces", 401}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
		if response.Code != test.status {
			t.Fatalf("%s = %d, want %d", test.path, response.Code, test.status)
		}
	}
	request := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "protocolVersion") {
		t.Fatalf("MCP = %d %s", response.Code, response.Body.String())
	}
}
