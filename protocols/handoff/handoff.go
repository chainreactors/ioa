package handoff

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/protocols"
	"github.com/chainreactors/ioa/skills"
)

func init() {
	protocols.Register(&protocols.Protocol{
		Name:        "handoff",
		Description: "Fire-and-forget work delegation",
		Send: []protocols.SubcommandDef{{
			Name:        "handoff",
			Description: "Delegate work to another agent",
			Data:        &SendFlags{},
			Execute:     execSend,
		}},
		Read: []protocols.SubcommandDef{{
			Name:        "handoff",
			Description: "Read handoff messages",
			Data:        &ReadFlags{},
			Execute:     execRead,
		}},
	})
}

type SendFlags struct {
	Title    string `long:"title" json:"title" required:"yes" description:"Short label for the delegated work"`
	Message  string `long:"message" json:"message" description:"Detailed context for the receiver"`
	RefNodes string `long:"ref-nodes" json:"ref_nodes" description:"Comma-separated target node IDs for routing"`
}

type ReadFlags struct {
	After string `long:"after" json:"after" description:"Read handoffs after this message ID"`
	Limit int    `long:"limit" json:"limit" description:"Maximum number of messages"`
}

func execSend(ctx context.Context, env *protocols.Env) (string, error) {
	flags := protocols.FlagsFrom[SendFlags](env)
	if flags.Title == "" {
		return "", fmt.Errorf("handoff: --title is required")
	}

	content := map[string]interface{}{
		"title": flags.Title,
	}
	if flags.Message != "" {
		content["message"] = flags.Message
	}

	body := ioa.SendMessage{ContentType: "handoff", Content: content}
	if flags.RefNodes != "" {
		var nodes []string
		for _, n := range strings.Split(flags.RefNodes, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				nodes = append(nodes, n)
			}
		}
		if len(nodes) > 0 {
			body.Refs = &ioa.Ref{Nodes: nodes}
		}
	}

	msg, err := env.Client.Send(ctx, env.SpaceID, body)
	if err != nil {
		return "", err
	}
	return encodeResult(msg)
}

func execRead(ctx context.Context, env *protocols.Env) (string, error) {
	flags := protocols.FlagsFrom[ReadFlags](env)

	opts := ioa.ReadOptions{All: true}
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

	var handoffs []ioa.Message
	for _, m := range messages {
		if ioa.MessageContentType(m) == "handoff" {
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
