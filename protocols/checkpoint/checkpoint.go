package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chainreactors/ioa/protocols"
)

func init() {
	protocols.Register(&protocols.Protocol{
		Name:        "checkpoint",
		Description: "Human-in-the-loop review protocol",
		Send: &protocols.Handler{
			Description: "Submit a checkpoint for human review",
			Flags:       &SendFlags{},
			Execute:     execSend,
		},
		Read: &protocols.Handler{
			Description: "Read checkpoint messages",
			Flags:       &ReadFlags{},
			Execute:     execRead,
		},
	})
}

type SendFlags struct {
	Kind    string `long:"kind" json:"kind" description:"Checkpoint kind"`
	Title   string `long:"title" json:"title" description:"Checkpoint title"`
	Content string `long:"content" json:"content" description:"Checkpoint content"`
	Target  string `long:"target" json:"target" description:"Target for the checkpoint"`
	Status  string `long:"status" json:"status" description:"Checkpoint status"`
}

type ReadFlags struct {
	After string `long:"after" json:"after" description:"Read messages after this ID"`
	Limit int    `long:"limit" json:"limit" description:"Max number of messages to read"`
}

func execSend(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
	var flags SendFlags
	protocols.ParseArgs(args, &flags)
	if flags.Kind == "" || flags.Title == "" {
		return "", fmt.Errorf("checkpoint: --kind and --title are required")
	}

	content := map[string]interface{}{
		"id":    protocols.NewID(),
		"kind":  flags.Kind,
		"title": flags.Title,
	}
	if flags.Content != "" {
		content["content"] = flags.Content
	}
	if flags.Target != "" {
		content["target"] = flags.Target
	}
	if flags.Status != "" {
		content["status"] = NormalizeStatus(flags.Status)
	}

	msg, err := env.Client.Send(ctx, env.SpaceID, protocols.SendMessage{ContentType: "checkpoint", Content: content})
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

	var checkpoints []protocols.Message
	for _, m := range messages {
		if protocols.MessageContentType(m) == "checkpoint" {
			checkpoints = append(checkpoints, m)
		}
	}
	return encodeResult(checkpoints)
}

func NormalizeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "confirmed":
		return "confirmed"
	case "not_confirmed", "not confirmed", "false_positive":
		return "not_confirmed"
	case "info", "informational":
		return "info"
	case "inconclusive":
		return "inconclusive"
	default:
		return value
	}
}

func encodeResult(v interface{}) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
