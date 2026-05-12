package client

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/server"
)

func TestClientAndTools(t *testing.T) {
	httpServer := httptest.NewServer(server.NewHandler(server.NewService(server.NewMemoryStore())))
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
	spaceOut, err := tools[0].Execute(ctx, `{"name":"case","description":"tester"}`)
	if err != nil {
		t.Fatalf("acp_space error = %v", err)
	}
	if client.NodeID() == "" {
		t.Fatal("client node id was not auto-registered")
	}
	var space ioa.SpaceInfo
	if err := json.Unmarshal([]byte(spaceOut), &space); err != nil {
		t.Fatalf("decode space: %v", err)
	}

	sendOut, err := tools[1].Execute(ctx, `{"space_id":"`+space.ID+`","content":{"text":"hello"}}`)
	if err != nil {
		t.Fatalf("acp_send error = %v", err)
	}
	var message ioa.Message
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

func TestSendToolRejectsMissingContent(t *testing.T) {
	httpServer := httptest.NewServer(server.NewHandler(server.NewService(server.NewMemoryStore())))
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
	var space ioa.SpaceInfo
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
