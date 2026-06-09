package client

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chainreactors/ioa/api"
	"github.com/chainreactors/ioa/protocols"
	"github.com/chainreactors/ioa/server"
)

func TestClientAndTools(t *testing.T) {
	httpServer := httptest.NewServer(server.NewHandler(server.NewService(server.NewMemoryStore(), "")))
	defer httpServer.Close()

	client, err := NewClient(httpServer.URL, "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	tools := NewTools(client, ToolOptions{NodeName: "agent"})
	if len(tools) != 3 {
		t.Fatalf("tools = %d, want 3", len(tools))
	}

	ctx := context.Background()
	spaceOut, err := tools[0].Execute(ctx, `{"name":"case","description":"tester","tags":["workspace:aide","aide"]}`)
	if err != nil {
		t.Fatalf("acp_space error = %v", err)
	}
	if client.NodeID() == "" {
		t.Fatal("client node id was not auto-registered")
	}
	var space protocols.SpaceInfo
	if err := json.Unmarshal([]byte(spaceOut), &space); err != nil {
		t.Fatalf("decode space: %v", err)
	}
	if got := strings.Join(space.Tags, ","); got != "workspace:aide,aide" {
		t.Fatalf("space tags = %q", got)
	}

	sendOut, err := tools[1].Execute(ctx, `{"space_id":"`+space.ID+`","content":{"text":"hello"}}`)
	if err != nil {
		t.Fatalf("acp_send error = %v", err)
	}
	var message protocols.Message
	if err := json.Unmarshal([]byte(sendOut), &message); err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if message.Content["text"] != "hello" {
		t.Fatalf("message content = %#v", message.Content)
	}
	if message.Refs.Messages == nil || message.Refs.Nodes == nil {
		t.Fatalf("message refs = %#v, want non-nil empty slices", message.Refs)
	}

	readOut, err := tools[2].Execute(ctx, `{"space_id":"`+space.ID+`","all":true}`)
	if err != nil {
		t.Fatalf("acp_read error = %v", err)
	}
	if !strings.Contains(readOut, message.ID) {
		t.Fatalf("read output missing message id: %s", readOut)
	}
}

func TestClientProjections(t *testing.T) {
	httpServer := httptest.NewServer(server.NewHandler(server.NewService(server.NewMemoryStore(), "")))
	defer httpServer.Close()

	ctx := context.Background()
	clientA, err := NewClient(httpServer.URL, "")
	if err != nil {
		t.Fatalf("NewClient(a) error = %v", err)
	}
	clientB, err := NewClient(httpServer.URL, "")
	if err != nil {
		t.Fatalf("NewClient(b) error = %v", err)
	}
	nodeA, err := clientA.RegisterNode(ctx, "agent-a", "", nil)
	if err != nil {
		t.Fatalf("RegisterNode(a) error = %v", err)
	}
	nodeB, err := clientB.RegisterNode(ctx, "agent-b", "", nil)
	if err != nil {
		t.Fatalf("RegisterNode(b) error = %v", err)
	}
	space, err := clientA.Space(ctx, "case", "owner")
	if err != nil {
		t.Fatalf("Space(a) error = %v", err)
	}
	if _, err := clientB.Space(ctx, "case", "reviewer"); err != nil {
		t.Fatalf("Space(b) error = %v", err)
	}
	root, err := clientA.Send(ctx, space.ID, protocols.SendMessage{Content: map[string]interface{}{"text": "root"}})
	if err != nil {
		t.Fatalf("Send(root) error = %v", err)
	}
	child, err := clientB.Send(ctx, space.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "child"},
		Refs:    &protocols.Ref{Messages: []string{root.ID}, Nodes: []string{nodeA.ID}},
	})
	if err != nil {
		t.Fatalf("Send(child) error = %v", err)
	}

	records, err := clientA.ListMessages(ctx, api.MessageFilter{SpaceID: space.ID, RefMessage: root.ID})
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != child.ID || records[0].SpaceID != space.ID {
		t.Fatalf("ListMessages() = %#v, want child with space_id", records)
	}
	graph, err := clientA.GetSpaceGraph(ctx, space.ID, api.GraphOptions{})
	if err != nil {
		t.Fatalf("GetSpaceGraph() error = %v", err)
	}
	if !hasClientGraphEdge(graph, api.GraphEdge{Source: "message:" + child.ID, Target: "message:" + root.ID, Kind: "refs.messages"}) {
		t.Fatalf("GetSpaceGraph() missing refs.messages edge: %#v", graph.Edges)
	}
	global, err := clientA.GetGraph(ctx, api.GraphOptions{
		MessageFilter: api.MessageFilter{NodeID: nodeB.ID},
		Include:       []string{"messages", "edges"},
	})
	if err != nil {
		t.Fatalf("GetGraph() error = %v", err)
	}
	if len(global.Spaces) != 0 || len(global.Nodes) != 0 {
		t.Fatalf("GetGraph(include) returned spaces/nodes: spaces=%d nodes=%d", len(global.Spaces), len(global.Nodes))
	}
	if len(global.Messages) != 1 || global.Messages[0].ID != child.ID {
		t.Fatalf("GetGraph(node_id) messages = %#v, want child", global.Messages)
	}
}

func TestSendToolRejectsMissingContent(t *testing.T) {
	httpServer := httptest.NewServer(server.NewHandler(server.NewService(server.NewMemoryStore(), "")))
	defer httpServer.Close()

	client, err := NewClient(httpServer.URL, "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	tools := NewTools(client, ToolOptions{NodeName: "agent"})
	ctx := context.Background()
	spaceOut, err := tools[0].Execute(ctx, `{"name":"case","description":"tester"}`)
	if err != nil {
		t.Fatalf("acp_space error = %v", err)
	}
	var space protocols.SpaceInfo
	if err := json.Unmarshal([]byte(spaceOut), &space); err != nil {
		t.Fatalf("decode space: %v", err)
	}
	if _, err := tools[1].Execute(ctx, `{"space_id":"`+space.ID+`"}`); err == nil {
		t.Fatal("acp_send without content succeeded, want error")
	}
	if _, err := tools[1].Execute(ctx, `{"space_id":"`+space.ID+`","content":null}`); err == nil {
		t.Fatal("acp_send with null content succeeded, want error")
	}
}

func hasClientGraphEdge(graph api.GraphView, want api.GraphEdge) bool {
	for _, edge := range graph.Edges {
		if edge == want {
			return true
		}
	}
	return false
}
