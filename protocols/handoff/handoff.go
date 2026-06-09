package handoff

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chainreactors/ioa/protocols"
	"github.com/chainreactors/ioa/skills"
)

func init() {
	protocols.Register(&protocols.Protocol{
		Name:        "handoff",
		Description: "Fire-and-forget work delegation",
		Send: &protocols.Handler{
			Description: "Delegate work to another agent",
			Flags:       &SendFlags{},
			Execute:     execSend,
		},
		Read: &protocols.Handler{
			Description: "Read handoff messages",
			Flags:       &ReadFlags{},
			Execute:     execRead,
		},
	})
}

type SendFlags struct {
	Title    string `long:"title" json:"title" description:"Handoff title"`
	Message  string `long:"message" json:"message" description:"Handoff message body"`
	RefNodes string `long:"ref_nodes" json:"ref_nodes" description:"Comma-separated reference node IDs"`
}

type ReadFlags struct {
	After string `long:"after" json:"after" description:"Read messages after this ID"`
	Limit int    `long:"limit" json:"limit" description:"Max number of messages to read"`
}

func execSend(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
	var flags SendFlags
	protocols.ParseArgs(args, &flags)
	if flags.Title == "" {
		return "", fmt.Errorf("handoff: --title is required")
	}

	content := map[string]interface{}{
		"title": flags.Title,
	}
	if flags.Message != "" {
		content["message"] = flags.Message
	}

	body := protocols.SendMessage{ContentType: "handoff", Content: content}
	if flags.RefNodes != "" {
		var nodes []string
		for _, n := range strings.Split(flags.RefNodes, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				nodes = append(nodes, n)
			}
		}
		if len(nodes) > 0 {
			body.Refs = &protocols.Ref{Nodes: nodes}
		}
	}

	msg, err := env.Client.Send(ctx, env.SpaceID, body)
	if err != nil {
		return "", err
	}
	return encodeResult(msg)
}

func execRead(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
	var flags ReadFlags
	protocols.ParseArgs(args, &flags)

	opts := protocols.ReadOptions{All: true}
	if flags.After != "" {
		opts.After = flags.After
	}
	if flags.Limit > 0 {
		opts.Limit = flags.Limit
	}

	messages, err := env.Client.Read(ctx, env.SpaceID, opts)
	if err != nil {
		return "", err
	}

	var handoffs []protocols.Message
	for _, m := range messages {
		if protocols.MessageContentType(m) == "handoff" {
			handoffs = append(handoffs, m)
		}
	}
	return encodeResult(handoffs)
}

func Schema() map[string]any {
	s, err := skills.ReadSchema("handoff")
	if err != nil {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
			},
		}
	}
	return s
}

func encodeResult(v interface{}) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
