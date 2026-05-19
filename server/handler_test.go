package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chainreactors/ioa"
)

func TestHandlerHealth(t *testing.T) {
	srv := httptest.NewServer(NewHandler(NewService(NewMemoryStore(), "")))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("health status = %q, want ok", body["status"])
	}

	req, err := http.NewRequest(http.MethodHead, srv.URL+"/ready", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD /ready status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestHandlerHealthReportsStoreFailure(t *testing.T) {
	srv := httptest.NewServer(NewHandler(NewService(failingListSpacesStore{
		MemoryStore: NewMemoryStore(),
		err:         errors.New("store unavailable"),
	}, "")))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /health status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	var body ioa.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Detail != "store unavailable" {
		t.Fatalf("health detail = %q, want store unavailable", body.Detail)
	}
}

func TestAuthMiddlewareScopesAccessKeyToBootstrap(t *testing.T) {
	const accessKey = "test-access-key"
	service := NewService(NewMemoryStore(), accessKey)
	srv := httptest.NewServer(AuthMiddleware(service)(NewHandler(service)))
	defer srv.Close()

	getStatus(t, srv.URL+"/health", nil, http.StatusOK)
	getStatus(t, srv.URL+"/spaces", nil, http.StatusUnauthorized)
	getStatus(t, srv.URL+"/spaces", map[string]string{"X-Access-Key": accessKey}, http.StatusUnauthorized)

	auth := postJSONAuth(t, srv.URL+"/auth/register", map[string]interface{}{
		"name":       "agent",
		"access_key": accessKey,
		"meta":       map[string]interface{}{"role": "test"},
	}, http.StatusCreated)
	tokenHeaders := map[string]string{"Authorization": "Bearer " + auth.Token}
	getStatus(t, srv.URL+"/spaces", tokenHeaders, http.StatusOK)

	postJSONStatusHeaders(t, srv.URL+"/spaces", tokenHeaders, map[string]interface{}{
		"name": "case", "description": "token-only",
	}, http.StatusUnauthorized)

	space := postJSONSpaceInfoHeaders(t, srv.URL+"/spaces", map[string]string{
		"X-Access-Key": accessKey,
		"X-Node-ID":    auth.ID,
	}, map[string]interface{}{"name": "case", "description": "bootstrap"}, http.StatusOK)
	if space.Name != "case" {
		t.Fatalf("space name = %q, want case", space.Name)
	}

	msg := postJSONMessageHeaders(t, srv.URL+"/spaces/"+space.ID+"/messages", tokenHeaders, map[string]interface{}{
		"content": map[string]interface{}{"text": "hello"},
	}, http.StatusCreated)
	if msg.Sender != auth.ID {
		t.Fatalf("message sender = %q, want token node %q", msg.Sender, auth.ID)
	}
}

type failingListSpacesStore struct {
	*MemoryStore
	err error
}

func (s failingListSpacesStore) ListSpaces() ([]ioa.Space, error) {
	return nil, s.err
}

func TestHandlerHTTPAndSSE(t *testing.T) {
	srv := httptest.NewServer(NewHandler(NewService(NewMemoryStore(), "")))
	defer srv.Close()

	node := postJSONNode(t, srv.URL+"/nodes", "", map[string]interface{}{"name": "agent", "meta": map[string]interface{}{"role": "test"}}, http.StatusCreated)
	space := postJSONSpaceInfo(t, srv.URL+"/spaces", node.ID, map[string]interface{}{"name": "case", "description": "tester", "tags": []string{"workspace:aide", "aide"}}, http.StatusOK)
	if !reflect.DeepEqual(space.Tags, []string{"workspace:aide", "aide"}) {
		t.Fatalf("space tags = %#v, want normalized tags", space.Tags)
	}

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
	srv := httptest.NewServer(NewHandler(NewService(NewMemoryStore(), "")))
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

func TestHandlerMessagesAndGraph(t *testing.T) {
	srv := httptest.NewServer(NewHandler(NewService(NewMemoryStore(), "")))
	defer srv.Close()

	nodeA := postJSONNode(t, srv.URL+"/nodes", "", map[string]interface{}{"name": "agent-a"}, http.StatusCreated)
	nodeB := postJSONNode(t, srv.URL+"/nodes", "", map[string]interface{}{"name": "agent-b"}, http.StatusCreated)
	space := postJSONSpaceInfo(t, srv.URL+"/spaces", nodeA.ID, map[string]interface{}{"name": "case", "description": "owner"}, http.StatusOK)
	_ = postJSONSpaceInfo(t, srv.URL+"/spaces", nodeB.ID, map[string]interface{}{"name": "case", "description": "reviewer"}, http.StatusOK)
	root := postJSONMessage(t, srv.URL+"/spaces/"+space.ID+"/messages", nodeA.ID, map[string]interface{}{"content": map[string]interface{}{"text": "root"}}, http.StatusCreated)
	child := postJSONMessage(t, srv.URL+"/spaces/"+space.ID+"/messages", nodeB.ID, map[string]interface{}{
		"content": map[string]interface{}{"text": "child"},
		"refs":    map[string]interface{}{"messages": []string{root.ID}},
	}, http.StatusCreated)
	directed := postJSONMessage(t, srv.URL+"/spaces/"+space.ID+"/messages", nodeA.ID, map[string]interface{}{
		"content": map[string]interface{}{"text": "to-b"},
		"refs":    map[string]interface{}{"nodes": []string{nodeB.ID}},
	}, http.StatusCreated)

	records := getJSONMessageRecords(t, srv.URL+"/messages?space_id="+space.ID, http.StatusOK)
	if got := handlerRecordIDs(records); len(got) != 3 || got[0] != root.ID || got[1] != child.ID || got[2] != directed.ID {
		t.Fatalf("GET /messages ids = %#v, want root,child,directed", got)
	}
	refRecords := getJSONMessageRecords(t, srv.URL+"/messages?space_id="+space.ID+"&ref_message="+root.ID, http.StatusOK)
	if got := handlerRecordIDs(refRecords); len(got) != 1 || got[0] != child.ID {
		t.Fatalf("GET /messages ref ids = %#v, want child", got)
	}

	graph := getJSONGraph(t, srv.URL+"/spaces/"+space.ID+"/graph", http.StatusOK)
	if graph.Stats.SpaceCount != 1 || graph.Stats.MessageCount != 3 {
		t.Fatalf("GET /spaces/{id}/graph stats = %#v, want one space and three messages", graph.Stats)
	}
	if !hasHandlerGraphEdge(graph, ioa.GraphEdge{Source: "message:" + child.ID, Target: "message:" + root.ID, Kind: "refs.messages"}) {
		t.Fatalf("space graph missing refs.messages edge: %#v", graph.Edges)
	}
	if !hasHandlerGraphEdge(graph, ioa.GraphEdge{Source: "message:" + directed.ID, Target: "node:" + nodeB.ID, Kind: "refs.nodes"}) {
		t.Fatalf("space graph missing refs.nodes edge: %#v", graph.Edges)
	}

	related := getJSONGraph(t, srv.URL+"/graph?message_id="+root.ID, http.StatusOK)
	if got := handlerRecordIDs(related.Messages); len(got) != 2 || got[0] != root.ID || got[1] != child.ID {
		t.Fatalf("GET /graph message_id ids = %#v, want root,child", got)
	}
}

func doPost(t *testing.T, url, nodeID string, body interface{}, wantStatus int) []byte {
	t.Helper()
	headers := map[string]string{}
	if nodeID != "" {
		headers["X-Node-ID"] = nodeID
	}
	return doPostHeaders(t, url, headers, body, wantStatus)
}

func doPostHeaders(t *testing.T, url string, headers map[string]string, body interface{}, wantStatus int) []byte {
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
	for key, value := range headers {
		req.Header.Set(key, value)
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

func doGet(t *testing.T, url string, wantStatus int) []byte {
	t.Helper()
	return getStatus(t, url, nil, wantStatus)
}

func getStatus(t *testing.T, url string, headers map[string]string, wantStatus int) []byte {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d, body=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(data)))
	}
	return data
}

func postJSONAuth(t *testing.T, url string, body interface{}, wantStatus int) ioa.AuthResponse {
	t.Helper()
	data := doPostHeaders(t, url, nil, body, wantStatus)
	var out ioa.AuthResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
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

func postJSONSpaceInfoHeaders(t *testing.T, url string, headers map[string]string, body interface{}, wantStatus int) ioa.SpaceInfo {
	t.Helper()
	data := doPostHeaders(t, url, headers, body, wantStatus)
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

func postJSONMessageHeaders(t *testing.T, url string, headers map[string]string, body interface{}, wantStatus int) ioa.Message {
	t.Helper()
	data := doPostHeaders(t, url, headers, body, wantStatus)
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

func postJSONStatusHeaders(t *testing.T, url string, headers map[string]string, body interface{}, wantStatus int) {
	t.Helper()
	doPostHeaders(t, url, headers, body, wantStatus)
}

func getJSONMessageRecords(t *testing.T, url string, wantStatus int) []ioa.MessageRecord {
	t.Helper()
	data := doGet(t, url, wantStatus)
	var out []ioa.MessageRecord
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func getJSONGraph(t *testing.T, url string, wantStatus int) ioa.GraphView {
	t.Helper()
	data := doGet(t, url, wantStatus)
	var out ioa.GraphView
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func handlerRecordIDs(messages []ioa.MessageRecord) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func hasHandlerGraphEdge(graph ioa.GraphView, want ioa.GraphEdge) bool {
	for _, edge := range graph.Edges {
		if edge == want {
			return true
		}
	}
	return false
}
