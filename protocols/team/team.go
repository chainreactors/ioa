package team

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/protocols"
	"github.com/chainreactors/ioa/skills"
)

func init() {
	protocols.Register(&protocols.Protocol{
		Name:        "team",
		Description: "Named-group communication within a workspace",
		Send: []protocols.SubcommandDef{{
			Name:        "team",
			Description: "Broadcast a message to a named group",
			Data:        &SendFlags{},
			Execute:     execSend,
		}},
		Read: []protocols.SubcommandDef{{
			Name:        "team",
			Description: "Read team messages",
			Data:        &ReadFlags{},
			Execute:     execRead,
		}},
	})
}

type SendFlags struct {
	Team string `long:"team" json:"team" required:"yes" description:"Group name"`
	Text string `long:"text" json:"text" required:"yes" description:"Message body"`
}

type ReadFlags struct {
	Team  string `long:"team" json:"team" description:"Filter by team name"`
	After string `long:"after" json:"after" description:"Read messages after this ID"`
	Limit int    `long:"limit" json:"limit" description:"Maximum number of messages"`
}

func execSend(ctx context.Context, env *protocols.Env) (string, error) {
	flags := protocols.FlagsFrom[SendFlags](env)
	if flags.Team == "" || flags.Text == "" {
		return "", fmt.Errorf("team: --team and --text are required")
	}

	content := map[string]interface{}{
		"team": flags.Team,
		"text": flags.Text,
	}

	msg, err := env.Client.Send(ctx, env.SpaceID, ioa.SendMessage{ContentType: "team", Content: content})
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

	var teamMsgs []ioa.Message
	for _, m := range messages {
		if ioa.MessageContentType(m) != "team" {
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
