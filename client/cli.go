package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chainreactors/ioa/protocols"
	goflags "github.com/jessevdk/go-flags"
)

// CommandOptions holds the client-side command tree (space/send/read),
// including per-protocol subcommands registered from protocols.All().
type CommandOptions struct {
	Space SpaceCommand `command:"space" description:"Create or join a space"`
	Send  SendCommand  `command:"send" description:"Send a message to a space"`
	Read  ReadCommand  `command:"read" description:"Read messages from a space"`
}

type SpaceCommand struct {
	Tags []string `long:"tag" description:"Space tag (repeatable)"`

	Positional struct {
		Name        string `positional-arg-name:"name" required:"yes"`
		Description string `positional-arg-name:"description" required:"yes"`
	} `positional-args:"yes"`
}

type SendCommand struct {
	SpaceID       string `long:"space" short:"s" description:"Space ID" required:"yes"`
	ContentType   string `long:"content-type" short:"t" description:"Message content type (e.g. checkpoint, handoff, team, swarm)"`
	Content       string `long:"content" short:"c" description:"Message content JSON"`
	RefMsgs       string `long:"ref-messages" description:"Comma-separated message IDs to reference"`
	RefNodes      string `long:"ref-nodes" description:"Comma-separated node IDs to target"`
	Meta          string `long:"meta" description:"Message metadata JSON"`
	ContentSchema string `long:"content-schema" description:"JSON Schema for content (declarative, per-message)"`
}

type ReadCommand struct {
	SpaceID   string `long:"space" short:"s" description:"Space ID" required:"yes"`
	MessageID string `long:"message" short:"m" description:"Message ID for context retrieval"`
	Direction string `long:"direction" short:"d" description:"Traversal direction: upstream, downstream (requires --message)"`
	After     string `long:"after" description:"Cursor: read messages after this ID"`
	Limit     int    `long:"limit" short:"l" description:"Maximum number of messages"`
	All       bool   `long:"all" short:"a" description:"Read all messages (not just addressed to this node)"`
	Listen    bool   `long:"listen" description:"Stream new messages via SSE (use with --message for thread-scoped)"`
}

// NewCommandParser builds a go-flags parser for the client command tree and
// attaches the protocol subcommands (send <protocol>, read <protocol>).
func NewCommandParser(opts *CommandOptions) *goflags.Parser {
	parser := goflags.NewParser(opts, goflags.Default&^goflags.PrintErrors)
	RegisterProtocolCommands(parser)
	return parser
}

// RegisterProtocolCommands attaches send/read protocol subcommands to any
// go-flags parser that has "send" and "read" commands.
func RegisterProtocolCommands(parser *goflags.Parser) {
	sendCommand := parser.Find("send")
	readCommand := parser.Find("read")

	for _, p := range protocols.All() {
		if p.Send != nil && sendCommand != nil {
			flags := p.Send.Flags
			if flags == nil {
				flags = &struct{}{}
			}
			_, _ = sendCommand.AddCommand(p.Name, p.Send.Description, "", flags)
		}
		if p.Read != nil && readCommand != nil {
			flags := p.Read.Flags
			if flags == nil {
				flags = &struct{}{}
			}
			_, _ = readCommand.AddCommand(p.Name, p.Read.Description, "", flags)
		}
	}

	if sendCommand != nil {
		sendCommand.SubcommandsOptional = true
	}
	if readCommand != nil {
		readCommand.SubcommandsOptional = true
	}
}

// Dispatch executes the parsed client command against the API client, writing
// results to stdout (os.Stdout when nil).
func Dispatch(ctx context.Context, c protocols.ClientAPI, nodeName string, opts *CommandOptions, active *goflags.Command, stdout io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if active == nil {
		return fmt.Errorf("no command specified")
	}
	switch active.Name {
	case "space":
		info, startMsgs, err := JoinSpace(ctx, c, nodeName, opts.Space.Positional.Name, opts.Space.Positional.Description, opts.Space.Tags...)
		if err != nil {
			return err
		}
		return writeJSON(stdout, struct {
			protocols.SpaceInfo
			StartMessages []protocols.Message `json:"start_messages"`
		}{SpaceInfo: info, StartMessages: startMsgs})
	case "send":
		return dispatchSend(ctx, c, nodeName, &opts.Send, active, stdout)
	case "read":
		return dispatchRead(ctx, c, nodeName, &opts.Read, active, stdout)
	}
	return fmt.Errorf("unknown command %q", active.Name)
}

// JoinSpace registers the node when needed, creates or joins the named space,
// and returns the space info with its root (start) messages.
func JoinSpace(ctx context.Context, c protocols.ClientAPI, nodeName, name, description string, tags ...string) (protocols.SpaceInfo, []protocols.Message, error) {
	if err := EnsureNode(ctx, c, nodeName); err != nil {
		return protocols.SpaceInfo{}, nil, err
	}
	info, err := c.Space(ctx, name, description, tags...)
	if err != nil {
		return protocols.SpaceInfo{}, nil, err
	}
	msgs, _ := c.Read(ctx, info.ID, protocols.ReadOptions{All: true})
	var startMsgs []protocols.Message
	for _, m := range msgs {
		if len(m.Refs.Messages) == 0 && len(m.Refs.Nodes) == 0 {
			startMsgs = append(startMsgs, m)
		}
	}
	return info, startMsgs, nil
}

// EnsureNode registers the node on first use.
func EnsureNode(ctx context.Context, c protocols.ClientAPI, nodeName string) error {
	if c.NodeID() != "" {
		return nil
	}
	if nodeName == "" {
		nodeName = "ioa-client"
	}
	type autoRegisterer interface {
		EnsureRegistered(ctx context.Context, name, description string, meta map[string]interface{}) error
	}
	if ar, ok := c.(autoRegisterer); ok {
		return ar.EnsureRegistered(ctx, nodeName, "", map[string]interface{}{})
	}
	_, err := c.RegisterNode(ctx, nodeName, "", map[string]interface{}{})
	return err
}

func dispatchSend(ctx context.Context, c protocols.ClientAPI, nodeName string, cmd *SendCommand, active *goflags.Command, stdout io.Writer) error {
	if sub := active.Active; sub != nil {
		return execProtocolSend(ctx, c, nodeName, cmd.SpaceID, sub, stdout)
	}
	if cmd.Content == "" {
		return fmt.Errorf("send: --content is required")
	}
	if err := EnsureNode(ctx, c, nodeName); err != nil {
		return err
	}
	var content map[string]interface{}
	if err := json.Unmarshal([]byte(cmd.Content), &content); err != nil {
		return fmt.Errorf("send: invalid content JSON: %s", err)
	}
	body := protocols.SendMessage{ContentType: cmd.ContentType, Content: content}
	if cmd.RefMsgs != "" {
		if body.Refs == nil {
			body.Refs = &protocols.Ref{}
		}
		body.Refs.Messages = splitComma(cmd.RefMsgs)
	}
	if cmd.RefNodes != "" {
		if body.Refs == nil {
			body.Refs = &protocols.Ref{}
		}
		body.Refs.Nodes = splitComma(cmd.RefNodes)
	}
	if cmd.Meta != "" {
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(cmd.Meta), &meta); err != nil {
			return fmt.Errorf("send: invalid meta JSON: %s", err)
		}
		body.Meta = meta
	}
	if cmd.ContentSchema != "" {
		var schema map[string]interface{}
		if err := json.Unmarshal([]byte(cmd.ContentSchema), &schema); err != nil {
			return fmt.Errorf("send: invalid content-schema JSON: %s", err)
		}
		body.ContentSchema = schema
	}
	msg, err := c.Send(ctx, cmd.SpaceID, body)
	if err != nil {
		return err
	}
	return writeJSON(stdout, msg)
}

func dispatchRead(ctx context.Context, c protocols.ClientAPI, nodeName string, cmd *ReadCommand, active *goflags.Command, stdout io.Writer) error {
	if sub := active.Active; sub != nil {
		return execProtocolRead(ctx, c, nodeName, cmd.SpaceID, sub, stdout)
	}
	if err := EnsureNode(ctx, c, nodeName); err != nil {
		return err
	}
	if cmd.Listen {
		return listen(ctx, c, cmd, stdout)
	}
	msgs, err := c.Read(ctx, cmd.SpaceID, protocols.ReadOptions{
		MessageID: cmd.MessageID,
		Direction: cmd.Direction,
		After:     cmd.After,
		Limit:     cmd.Limit,
		All:       cmd.All,
	})
	if err != nil {
		return err
	}
	return writeJSON(stdout, msgs)
}

func listen(ctx context.Context, c protocols.ClientAPI, cmd *ReadCommand, stdout io.Writer) error {
	stream, ok := c.(StreamAPI)
	if !ok {
		return fmt.Errorf("read: --listen is not supported by this client")
	}
	var opts []SubscribeOption
	if cmd.MessageID != "" {
		opts = append(opts, WithMessage(cmd.MessageID))
	}
	messages, errs, cancel, err := stream.Subscribe(ctx, cmd.SpaceID, opts...)
	if err != nil {
		return err
	}
	defer cancel()

	enc := json.NewEncoder(stdout)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errs:
			if ok && err != nil {
				return err
			}
		case msg, ok := <-messages:
			if !ok {
				return nil
			}
			_ = enc.Encode(msg)
		}
	}
}

func execProtocolSend(ctx context.Context, c protocols.ClientAPI, nodeName, spaceID string, sub *goflags.Command, stdout io.Writer) error {
	if err := EnsureNode(ctx, c, nodeName); err != nil {
		return err
	}
	p := protocols.Get(sub.Name)
	if p == nil || p.Send == nil {
		return fmt.Errorf("send: unknown subcommand %q", sub.Name)
	}
	env := &protocols.Env{Client: c, SpaceID: spaceID, NodeName: nodeName}
	result, err := p.Send.Execute(ctx, env, p.Send.Flags)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, result)
	return err
}

func execProtocolRead(ctx context.Context, c protocols.ClientAPI, nodeName, spaceID string, sub *goflags.Command, stdout io.Writer) error {
	if err := EnsureNode(ctx, c, nodeName); err != nil {
		return err
	}
	p := protocols.Get(sub.Name)
	if p == nil || p.Read == nil {
		return fmt.Errorf("read: unknown subcommand %q", sub.Name)
	}
	env := &protocols.Env{Client: c, SpaceID: spaceID, NodeName: nodeName}
	result, err := p.Read.Execute(ctx, env, p.Read.Flags)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, result)
	return err
}

func writeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
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
