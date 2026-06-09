package swarm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/chainreactors/ioa/protocols"
)

func init() {
	protocols.Register(&protocols.Protocol{
		Name:        "swarm",
		Description: "Autonomous multi-agent coordination",
		Send: &protocols.Handler{
			Description: "Send a swarm message or broadcast a task",
			Flags:       &SendFlags{},
			Execute:     execSend,
		},
		Read: &protocols.Handler{
			Description: "Read swarm messages",
			Flags:       &ReadFlags{},
			Execute:     execRead,
		},
	})
}

type SendFlags struct {
	Content string `long:"content" json:"content" description:"Message content"`
	Targets string `long:"targets" json:"targets" description:"Comma-separated targets"`
	Task    bool   `long:"task" json:"task" description:"Mark as task broadcast"`
}

type ReadFlags struct {
	After string `long:"after" json:"after" description:"Read after message ID"`
	Limit int    `long:"limit" json:"limit" description:"Max messages"`
	Kind  string `long:"kind" json:"kind" description:"Filter by kind"`
}

func execSend(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
	var flags SendFlags
	protocols.ParseArgs(args, &flags)

	msg := SwarmMessage{Content: flags.Content}
	if flags.Targets != "" {
		for _, t := range splitComma(flags.Targets) {
			msg.Targets = append(msg.Targets, t)
		}
	}

	body := protocols.SendMessage{ContentType: "swarm", Content: SwarmContent(msg)}
	if flags.Task {
		body.Content["task"] = true
	}

	sent, err := env.Client.Send(ctx, env.SpaceID, body)
	if err != nil {
		return "", err
	}
	return encodeResult(sent)
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

	var swarmMsgs []protocols.Message
	for _, m := range messages {
		if _, ok := ParseSwarm(m.Content); ok {
			swarmMsgs = append(swarmMsgs, m)
		} else if _, ok := ParseLegacyMessage(m.Content); ok {
			swarmMsgs = append(swarmMsgs, m)
		}
	}

	if flags.Kind != "" {
		var filtered []protocols.Message
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
