package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chainreactors/ioa"
)

func TestHandlerHTTPAndSSE(t *testing.T) {
	srv := httptest.NewServer(NewHandler(NewService(NewMemoryStore())))
	defer srv.Close()

	node := postJSONNode(t, srv.URL+"/nodes", "", map[string]interface{}{"name": "agent", "meta": map[string]interface{}{"role": "test"}}, http.StatusCreated)
	space := postJSONSpaceInfo(t, srv.URL+"/spaces", node.ID, map[string]interface{}{"name": "case", "description": "tester"}, http.StatusOK)

	sseReq, err := http.NewRequest(http.MethodGet, srv.URL+"/spaces/"+space.ID+"/sse", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(sseReq)
	if err != nil {
		t.Fatalf("open sse: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type = %q", got)
	}

	done := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				done <- strings.TrimPrefix(line, "data: ")
				return
			}
		}
		done <- ""
	}()

	message := postJSONMessage(t, srv.URL+"/spaces/"+space.ID+"/messages", node.ID, map[string]interface{}{"content": map[string]interface{}{"text": "hello"}}, http.StatusCreated)

	select {
	case data := <-done:
		if data == "" {
			t.Fatal("sse closed without data")
		}
		var got ioa.Message
		if err := json.Unmarshal([]byte(data), &got); err != nil {
			t.Fatalf("decode sse data: %v", err)
		}
		if got.ID != message.ID {
			t.Fatalf("sse message id = %s, want %s", got.ID, message.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for sse message")
	}
}

func TestHandlerDefaultsAndValidation(t *testing.T) {
	srv := httptest.NewServer(NewHandler(NewService(NewMemoryStore())))
	defer srv.Close()

	node := postJSONNode(t, srv.URL+"/nodes", "", map[string]interface{}{"name": "agent"}, http.StatusCreated)
	if node.Meta == nil || len(node.Meta) != 0 {
		t.Fatalf("node meta = %#v, want empty map", node.Meta)
	}
	space := postJSONSpaceInfo(t, srv.URL+"/spaces", node.ID, map[string]interface{}{"name": "case", "description": "tester"}, http.StatusOK)
	message := postJSONMessage(t, srv.URL+"/spaces/"+space.ID+"/messages", node.ID, map[string]interface{}{"content": map[string]interface{}{"text": "hello"}}, http.StatusCreated)
	if message.Refs.Messages == nil || message.Refs.Nodes == nil || len(message.Refs.Messages) != 0 || len(message.Refs.Nodes) != 0 {
		t.Fatalf("message refs = %#v, want empty slices", message.Refs)
	}

	postJSONStatus(t, srv.URL+"/spaces/"+space.ID+"/messages", node.ID, map[string]interface{}{"content": nil}, http.StatusUnprocessableEntity)
	postJSONStatus(t, srv.URL+"/spaces/"+space.ID+"/messages", node.ID, map[string]interface{}{}, http.StatusUnprocessableEntity)
}

func doPost(t *testing.T, url, nodeID string, body interface{}, wantStatus int) []byte {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if nodeID != "" {
		req.Header.Set("X-Node-ID", nodeID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	respData, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d, body=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(respData)))
	}
	return respData
}

func postJSONNode(t *testing.T, url, nodeID string, body interface{}, wantStatus int) ioa.Node {
	t.Helper()
	data := doPost(t, url, nodeID, body, wantStatus)
	var out ioa.Node
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postJSONSpaceInfo(t *testing.T, url, nodeID string, body interface{}, wantStatus int) ioa.SpaceInfo {
	t.Helper()
	data := doPost(t, url, nodeID, body, wantStatus)
	var out ioa.SpaceInfo
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postJSONMessage(t *testing.T, url, nodeID string, body interface{}, wantStatus int) ioa.Message {
	t.Helper()
	data := doPost(t, url, nodeID, body, wantStatus)
	var out ioa.Message
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postJSONStatus(t *testing.T, url, nodeID string, body interface{}, wantStatus int) {
	t.Helper()
	doPost(t, url, nodeID, body, wantStatus)
}
