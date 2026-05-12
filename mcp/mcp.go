//go:build mcp

package ioamcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/server"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type mcpBridge struct {
	service *server.Service
	nodeID  string
	mu      sync.Mutex
}

func (b *mcpBridge) ensureNode(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.nodeID != "" {
		return b.nodeID, nil
	}
	node, err := b.service.RegisterNode(ctx, ioa.NodeCreate{
		Name: "mcp-client",
		Meta: map[string]interface{}{"transport": "mcp"},
	})
	if err != nil {
		return "", err
	}
	b.nodeID = node.ID
	return b.nodeID, nil
}

func WithMCP(handler http.Handler, service *server.Service) http.Handler {
	bridge := &mcpBridge{service: service}
	s := mcpserver.NewMCPServer("ioa", "1.0.0")
	registerMCPTools(s, bridge)

	mcpHandler := mcpserver.NewStreamableHTTPServer(s)

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/", handler)
	return mux
}

func registerMCPTools(s *mcpserver.MCPServer, bridge *mcpBridge) {
	spaceTool := mcp.NewTool("ioa_space",
		mcp.WithDescription("Create or join an IOA message space for collaboration with other nodes. Returns space info with id, name, nodes, and message count."),
		mcp.WithString("name", mcp.Required(), mcp.Description("IOA space name")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Your role or intent in this space")),
	)
	s.AddTool(spaceTool, bridge.handleSpace)

	sendTool := mcp.NewTool("ioa_send",
		mcp.WithDescription("Send a structured IOA message to a space. Use refs.messages and refs.nodes to target context or recipients."),
		mcp.WithString("space_id", mcp.Required(), mcp.Description("IOA space id")),
		mcp.WithObject("content", mcp.Required(), mcp.Description("Structured message content")),
		mcp.WithObject("refs", mcp.Description("Optional references: {\"messages\": [\"msg-id\"], \"nodes\": [\"node-id\"]}")),
	)
	s.AddTool(sendTool, bridge.handleSend)

	readTool := mcp.NewTool("ioa_read",
		mcp.WithDescription("Read IOA messages from a space, optionally by related message context, after cursor, limit, or all messages."),
		mcp.WithString("space_id", mcp.Required(), mcp.Description("IOA space id")),
		mcp.WithString("message_id", mcp.Description("Optional message id to read its ancestor and descendant context")),
		mcp.WithString("after", mcp.Description("Optional message id cursor for pagination")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of messages to return")),
		mcp.WithBoolean("all", mcp.Description("Read all messages instead of only messages addressed to this node")),
	)
	s.AddTool(readTool, bridge.handleRead)
}

type mcpSpaceResult struct {
	ioa.SpaceInfo
	StartMessages []ioa.Message `json:"start_messages"`
}

func (b *mcpBridge) handleSpace(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	description, err := request.RequireString("description")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	nodeID, err := b.ensureNode(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	info, err := b.service.CreateSpace(ctx, nodeID, ioa.SpaceCreate{
		Name:        name,
		Description: description,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	startMessages, err := b.service.ReadMessages(ctx, info.ID, "", ioa.ReadOptions{})
	if err != nil {
		return marshalToolResult(info)
	}

	return marshalToolResult(mcpSpaceResult{SpaceInfo: info, StartMessages: startMessages})
}

func (b *mcpBridge) handleSend(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spaceID, err := request.RequireString("space_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	args := request.GetArguments()
	content, ok := args["content"].(map[string]interface{})
	if !ok || content == nil {
		return mcp.NewToolResultError("content is required and must be a JSON object"), nil
	}

	var refs *ioa.Ref
	if refsRaw, hasRefs := args["refs"]; hasRefs && refsRaw != nil {
		data, _ := json.Marshal(refsRaw)
		refs = &ioa.Ref{}
		_ = json.Unmarshal(data, refs)
	}

	nodeID, err := b.ensureNode(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	message, err := b.service.SendMessage(ctx, spaceID, nodeID, ioa.SendMessage{
		Content: content,
		Refs:    refs,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return marshalToolResult(message)
}

func (b *mcpBridge) handleRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	spaceID, err := request.RequireString("space_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	messageID := request.GetString("message_id", "")
	after := request.GetString("after", "")
	limit := request.GetInt("limit", 0)

	args := request.GetArguments()
	all, _ := args["all"].(bool)

	nodeID, err := b.ensureNode(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	messages, err := b.service.ReadMessages(ctx, spaceID, nodeID, ioa.ReadOptions{
		MessageID: messageID,
		After:     after,
		Limit:     limit,
		All:       all,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return marshalToolResult(messages)
}

func marshalToolResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}
