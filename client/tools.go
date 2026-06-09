package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/chainreactors/ioa"
)

type API interface {
	NodeID() string
	RegisterNode(ctx context.Context, name, description string, meta map[string]interface{}) (ioa.Node, error)
	Space(ctx context.Context, name, description string, tags ...string) (ioa.SpaceInfo, error)
	Send(ctx context.Context, spaceID string, body ioa.SendMessage) (ioa.Message, error)
	Read(ctx context.Context, spaceID string, opts ioa.ReadOptions) ([]ioa.Message, error)
}

type StreamAPI interface {
	API
	Subscribe(ctx context.Context, spaceID string, opts ...SubscribeOption) (<-chan ioa.Message, <-chan error, func(), error)
}

type subscribeConfig struct {
	Head      string
	ForkDepth int
}

type SubscribeOption func(*subscribeConfig)

func WithHead(messageID string) SubscribeOption {
	return func(c *subscribeConfig) { c.Head = messageID }
}

func WithForkDepth(depth int) SubscribeOption {
	return func(c *subscribeConfig) { c.ForkDepth = depth }
}

type ToolOptions struct {
	NodeName        string
	NodeDescription string
	NodeMeta        map[string]interface{}
}

func NewTools(client API, opts ToolOptions) []Tool {
	base := &toolBase{client: client, opts: opts}
	return []Tool{
		&SpaceTool{base: base},
		&SendTool{base: base},
		&ReadTool{base: base},
	}
}

type Tool interface {
	Name() string
	Description() string
	Definition() ioa.ToolDefinition
	Execute(ctx context.Context, arguments string) (string, error)
}

type toolBase struct {
	client API
	opts   ToolOptions
	mu     sync.Mutex
}

func (b *toolBase) ensureNode(ctx context.Context) error {
	if b.client.NodeID() != "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.client.NodeID() != "" {
		return nil
	}
	name := b.opts.NodeName
	if name == "" {
		name = "ioa-agent"
	}
	meta := b.opts.NodeMeta
	if meta == nil {
		meta = map[string]interface{}{}
	}
	_, err := b.client.RegisterNode(ctx, name, b.opts.NodeDescription, meta)
	return err
}

func encodeToolResult(value interface{}) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type SpaceTool struct {
	base *toolBase
}

func (t *SpaceTool) Name() string { return "ioa_space" }

func (t *SpaceTool) Description() string {
	return "Create or join an IOA message space for collaboration with other nodes. Requires name and description."
}

func (t *SpaceTool) Definition() ioa.ToolDefinition {
	return ioa.ToolDefinition{
		Type: "function",
		Function: ioa.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "IOA space name",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Your role or intent in this space",
					},
					"tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Optional labels for ownership, workspace routing, or domain classification",
					},
				},
				"required": []string{"name", "description"},
			},
		},
	}
}

func (t *SpaceTool) Execute(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if err := t.base.ensureNode(ctx); err != nil {
		return "", err
	}
	info, err := t.base.client.Space(ctx, args.Name, args.Description, args.Tags...)
	if err != nil {
		return "", err
	}
	allMessages, err := t.base.client.Read(ctx, info.ID, ioa.ReadOptions{All: true})
	if err != nil {
		return encodeToolResult(info)
	}
	var startMessages []ioa.Message
	for _, m := range allMessages {
		if len(m.Refs.Messages) == 0 && len(m.Refs.Nodes) == 0 {
			startMessages = append(startMessages, m)
		}
	}
	return encodeToolResult(spaceResult{SpaceInfo: info, StartMessages: startMessages})
}

type spaceResult struct {
	ioa.SpaceInfo
	StartMessages []ioa.Message `json:"start_messages"`
}

type SendTool struct {
	base *toolBase
}

func (t *SendTool) Name() string { return "ioa_send" }

func (t *SendTool) Description() string {
	return "Send a structured IOA message to a space. Use refs.messages and refs.nodes to target context or recipients."
}

func (t *SendTool) Definition() ioa.ToolDefinition {
	return ioa.ToolDefinition{
		Type: "function",
		Function: ioa.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"space_id": map[string]interface{}{
						"type":        "string",
						"description": "IOA space id",
					},
					"content_type": map[string]interface{}{
						"type":        "string",
						"description": "Declares the message protocol type (e.g. checkpoint, handoff, team, swarm). Envelope-level field, not inside content body.",
					},
					"content": map[string]interface{}{
						"type":        "object",
						"description": "Structured message content",
					},
					"refs": refSchema(),
					"meta": map[string]interface{}{
						"type":        "object",
						"description": "Optional metadata for the message (e.g. kind, labels, ttl). Not part of content; used for routing, filtering, and lifecycle.",
					},
					"content_schema": map[string]interface{}{
						"type":        "object",
						"description": "Optional JSON Schema declaring the content structure of this message.",
					},
				},
				"required": []string{"space_id", "content"},
			},
		},
	}
}

func (t *SendTool) Execute(ctx context.Context, arguments string) (string, error) {
	var args struct {
		SpaceID       string                 `json:"space_id"`
		ContentType   string                 `json:"content_type"`
		Content       map[string]interface{} `json:"content"`
		Refs          *ioa.Ref               `json:"refs"`
		Meta          map[string]interface{} `json:"meta"`
		ContentSchema map[string]interface{} `json:"content_schema"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if err := t.base.ensureNode(ctx); err != nil {
		return "", err
	}
	if args.Content == nil {
		return "", fmt.Errorf("content is required")
	}
	message, err := t.base.client.Send(ctx, args.SpaceID, ioa.SendMessage{
		ContentType:   args.ContentType,
		Content:       args.Content,
		Refs:          args.Refs,
		Meta:          args.Meta,
		ContentSchema: args.ContentSchema,
	})
	if err != nil {
		return "", err
	}
	return encodeToolResult(message)
}

type ReadTool struct {
	base *toolBase
}

func (t *ReadTool) Name() string { return "ioa_read" }

func (t *ReadTool) Description() string {
	return "Read IOA messages from a space, optionally by related message context, after cursor, limit, or all messages."
}

func (t *ReadTool) Definition() ioa.ToolDefinition {
	return ioa.ToolDefinition{
		Type: "function",
		Function: ioa.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"space_id": map[string]interface{}{
						"type":        "string",
						"description": "IOA space id",
					},
					"message_id": map[string]interface{}{
						"type":        "string",
						"description": "Optional message id to read its ancestor and descendant context",
					},
					"after": map[string]interface{}{
						"type":        "string",
						"description": "Optional message id cursor",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of messages",
					},
					"all": map[string]interface{}{
						"type":        "boolean",
						"description": "Read all messages instead of only messages addressed to this node",
					},
				},
				"required": []string{"space_id"},
			},
		},
	}
}

func (t *ReadTool) Execute(ctx context.Context, arguments string) (string, error) {
	var args struct {
		SpaceID   string `json:"space_id"`
		MessageID string `json:"message_id"`
		After     string `json:"after"`
		Limit     int    `json:"limit"`
		All       bool   `json:"all"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if err := t.base.ensureNode(ctx); err != nil {
		return "", err
	}
	messages, err := t.base.client.Read(ctx, args.SpaceID, ioa.ReadOptions{
		MessageID: args.MessageID,
		After:     args.After,
		Limit:     args.Limit,
		All:       args.All,
	})
	if err != nil {
		return "", err
	}
	return encodeToolResult(messages)
}

func refSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Optional references to messages or node recipients",
		"properties": map[string]interface{}{
			"messages": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"nodes": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
	}
}
