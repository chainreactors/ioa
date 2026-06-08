package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/protocols"
)

func init() {
	protocols.Register(&protocols.Protocol{
		Name:        "checkpoint",
		Description: "Human-in-the-loop review protocol",
		Send: []protocols.SubcommandDef{{
			Name:        "checkpoint",
			Description: "Submit a checkpoint for human review",
			Data:        &SendFlags{},
			Execute:     execSend,
		}},
		Read: []protocols.SubcommandDef{{
			Name:        "checkpoint",
			Description: "Read checkpoint messages",
			Data:        &ReadFlags{},
			Execute:     execRead,
		}},
	})
}

type SendFlags struct {
	Kind    string `long:"kind" json:"kind" required:"yes" description:"Checkpoint kind (verify, sniper, deep)"`
	Title   string `long:"title" json:"title" required:"yes" description:"Short checkpoint title"`
	Content string `long:"content" json:"content" description:"Markdown body with evidence"`
	Target  string `long:"target" json:"target" description:"Target host:port or URL"`
	Status  string `long:"status" json:"status" description:"Verification status (confirmed, not_confirmed, info, inconclusive)"`
}

type ReadFlags struct {
	After string `long:"after" json:"after" description:"Read checkpoints after this message ID"`
	Limit int    `long:"limit" json:"limit" description:"Maximum number of messages"`
}

func execSend(ctx context.Context, env *protocols.Env) (string, error) {
	flags := protocols.FlagsFrom[SendFlags](env)
	if flags.Kind == "" || flags.Title == "" {
		return "", fmt.Errorf("checkpoint: --kind and --title are required")
	}

	content := map[string]interface{}{
		"id":    ioa.NewID(),
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

	msg, err := env.Client.Send(ctx, env.SpaceID, ioa.SendMessage{ContentType: "checkpoint", Content: content})
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

	var checkpoints []ioa.Message
	for _, m := range messages {
		if ioa.MessageContentType(m) == "checkpoint" {
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
