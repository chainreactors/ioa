package team

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chainreactors/ioa/protocols"
	"github.com/chainreactors/ioa/skills"
)

func init() {
	protocols.Register(&protocols.Protocol{
		Name:        "team",
		Description: "Named-group communication within a workspace",
		Send: &protocols.Handler{
			Description: "Broadcast a message to a named group",
			Flags:       &SendFlags{},
			Execute:     execSend,
		},
		Read: &protocols.Handler{
			Description: "Read team messages",
			Flags:       &ReadFlags{},
			Execute:     execRead,
		},
	})
}

type SendFlags struct {
	Team string `long:"team" json:"team" description:"Target team name"`
	Text string `long:"text" json:"text" description:"Message text"`
}

type ReadFlags struct {
	Team  string `long:"team" json:"team" description:"Filter by team name"`
	After string `long:"after" json:"after" description:"Read messages after this ID"`
	Limit int    `long:"limit" json:"limit" description:"Max number of messages to read"`
}

func execSend(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
	var flags SendFlags
	protocols.ParseArgs(args, &flags)
	if flags.Team == "" || flags.Text == "" {
		return "", fmt.Errorf("team: --team and --text are required")
	}

	content := map[string]interface{}{
		"team": flags.Team,
		"text": flags.Text,
	}

	msg, err := env.Client.Send(ctx, env.SpaceID, protocols.SendMessage{ContentType: "team", Content: content})
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

	var teamMsgs []protocols.Message
	for _, m := range messages {
		if protocols.MessageContentType(m) != "team" {
			continue
		}
		if flags.Team != "" {
			if name, _ := m.Content["team"].(string); name != flags.Team {
				continue
			}
		}
		teamMsgs = append(teamMsgs, m)
	}
	return encodeResult(teamMsgs)
}

func Schema() map[string]any {
	s, err := skills.ReadSchema("team")
	if err != nil {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"team": map[string]any{"type": "string"},
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"team", "text"},
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
