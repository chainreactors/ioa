package swarm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/protocols"
)

func init() {
	protocols.Register(&protocols.Protocol{
		Name:        "swarm",
		Description: "Autonomous multi-agent coordination",
		Send: []protocols.SubcommandDef{{
			Name:        "swarm",
			Description: "Send a swarm message or broadcast a task",
			Data:        &SendFlags{},
			Execute:     execSend,
		}},
		Read: []protocols.SubcommandDef{{
			Name:        "swarm",
			Description: "Read swarm messages",
			Data:        &ReadFlags{},
			Execute:     execRead,
		}},
	})
}

type SendFlags struct {
	Content string `long:"content" json:"content" required:"yes" description:"Message content"`
	Targets string `long:"targets" json:"targets" description:"Comma-separated operational targets"`
	Task    bool   `long:"task" json:"task" description:"Mark as task broadcast (all idle nodes pick up)"`
}

type ReadFlags struct {
	After string `long:"after" json:"after" description:"Read messages after this ID"`
	Limit int    `long:"limit" json:"limit" description:"Maximum number of messages"`
	Kind  string `long:"kind" json:"kind" description:"Filter by message kind (e.g. task_dispatch)"`
}

func execSend(ctx context.Context, env *protocols.Env) (string, error) {
	flags := protocols.FlagsFrom[SendFlags](env)

	msg := SwarmMessage{Content: flags.Content}
	if flags.Targets != "" {
		for _, t := range splitComma(flags.Targets) {
			msg.Targets = append(msg.Targets, t)
		}
	}

	body := ioa.SendMessage{ContentType: "swarm", Content: SwarmContent(msg)}
	if flags.Task {
		body.Content["task"] = true
	}

	sent, err := env.Client.Send(ctx, env.SpaceID, body)
	if err != nil {
		return "", err
	}
	return encodeResult(sent)
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

	var swarmMsgs []ioa.Message
	for _, m := range messages {
		if _, ok := ParseSwarm(m.Content); ok {
			swarmMsgs = append(swarmMsgs, m)
		} else if _, ok := ParseLegacyMessage(m.Content); ok {
			swarmMsgs = append(swarmMsgs, m)
		}
	}

	if flags.Kind != "" {
		var filtered []ioa.Message
		for _, m := range swarmMsgs {
			sm, _ := SwarmFromIOA(m)
			if MessageKind(m, sm) == flags.Kind {
				filtered = append(filtered, m)
			}
		}
		swarmMsgs = filtered
	}

	return encodeResult(swarmMsgs)
}

func splitComma(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func encodeResult(v interface{}) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
