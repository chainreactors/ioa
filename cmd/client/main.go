package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/client"
	"github.com/chainreactors/ioa/protocols"
	goflags "github.com/jessevdk/go-flags"
)

type options struct {
	URL      string `long:"url" env:"IOA_URL" description:"IOA server URL" default:"http://127.0.0.1:8765"`
	Token    string `long:"token" env:"IOA_TOKEN" description:"Auth token for authenticated requests"`
	NodeName string `long:"name" env:"IOA_NODE_NAME" description:"Node name for auto-registration" default:"ioa-client"`

	Register registerCmd `command:"register" description:"Register a new node and obtain a token"`
	Space    spaceCmd    `command:"space" description:"Create or join a space"`
	Send     sendCmd     `command:"send" description:"Send a message to a space"`
	Read     readCmd     `command:"read" description:"Read messages from a space"`
}

type registerCmd struct {
	AccessKey string `long:"access-key" env:"IOA_ACCESS_KEY" description:"Server access key" required:"yes"`
}

type spaceCmd struct {
	Tags []string `long:"tag" description:"Space tag (repeatable)"`

	Positional struct {
		Name        string `positional-arg-name:"name" required:"yes"`
		Description string `positional-arg-name:"description" required:"yes"`
	} `positional-args:"yes"`
}

type sendCmd struct {
	SpaceID       string `long:"space" short:"s" description:"Space ID" required:"yes"`
	Content       string `long:"content" short:"c" description:"Message content JSON"`
	RefMsgs       string `long:"ref-messages" description:"Comma-separated message IDs to reference"`
	RefNodes      string `long:"ref-nodes" description:"Comma-separated node IDs to target"`
	Meta          string `long:"meta" description:"Message metadata JSON"`
	ContentSchema string `long:"content-schema" description:"JSON Schema for thread content validation"`
}

type readCmd struct {
	SpaceID   string `long:"space" short:"s" description:"Space ID" required:"yes"`
	MessageID string `long:"message" short:"m" description:"Message ID for context retrieval"`
	After     string `long:"after" description:"Cursor: read messages after this ID"`
	Limit     int    `long:"limit" short:"l" description:"Maximum number of messages"`
	All       bool   `long:"all" short:"a" description:"Read all messages (not just addressed to this node)"`
}

func main() {
	var opts options
	parser := goflags.NewParser(&opts, goflags.Default&^goflags.PrintErrors)

	registerProtocols(parser)

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*goflags.Error); ok && flagsErr.Type == goflags.ErrHelp {
			parser.WriteHelp(os.Stdout)
			return
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	active := parser.Active
	if active == nil {
		parser.WriteHelp(os.Stderr)
		os.Exit(1)
	}

	var (
		c   *client.Client
		err error
	)
	if opts.Token != "" {
		c, err = client.NewClientWithToken(opts.URL, opts.Token)
	} else {
		c, err = client.NewClient(opts.URL, "")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	nodeName := opts.NodeName

	switch active.Name {
	case "register":
		err = runRegister(ctx, c, nodeName, opts.Register)
	case "space":
		err = runSpace(ctx, c, nodeName, opts.Space)
	case "send":
		err = runSendDispatch(ctx, c, nodeName, opts.Send, active)
	case "read":
		err = runReadDispatch(ctx, c, nodeName, opts.Read, active)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func registerProtocols(parser *goflags.Parser) {
	sendCommand := parser.Find("send")
	readCommand := parser.Find("read")

	for _, p := range protocols.All() {
		for i := range p.Send {
			def := &p.Send[i]
			if sendCommand != nil {
				sendCommand.AddCommand(def.Name, def.Description, "", def.Data)
			}
		}
		for i := range p.Read {
			def := &p.Read[i]
			if readCommand != nil {
				readCommand.AddCommand(def.Name, def.Description, "", def.Data)
			}
		}
	}

	if sendCommand != nil {
		sendCommand.SubcommandsOptional = true
	}
	if readCommand != nil {
		readCommand.SubcommandsOptional = true
	}
}

func runRegister(ctx context.Context, c *client.Client, nodeName string, cmd registerCmd) error {
	resp, err := c.Register(ctx, cmd.AccessKey, nodeName, "", map[string]interface{}{})
	if err != nil {
		return err
	}
	return writeJSON(resp)
}

func runSpace(ctx context.Context, c *client.Client, nodeName string, cmd spaceCmd) error {
	if err := ensureNode(ctx, c, nodeName); err != nil {
		return err
	}
	info, err := c.Space(ctx, cmd.Positional.Name, cmd.Positional.Description, cmd.Tags...)
	if err != nil {
		return err
	}
	msgs, _ := c.Read(ctx, info.ID, ioa.ReadOptions{All: true})
	var startMsgs []ioa.Message
	for _, m := range msgs {
		if len(m.Refs.Messages) == 0 && len(m.Refs.Nodes) == 0 {
			startMsgs = append(startMsgs, m)
		}
	}
	return writeJSON(struct {
		ioa.SpaceInfo
		StartMessages []ioa.Message `json:"start_messages"`
	}{SpaceInfo: info, StartMessages: startMsgs})
}

func runSendDispatch(ctx context.Context, c *client.Client, nodeName string, cmd sendCmd, active *goflags.Command) error {
	if sub := active.Active; sub != nil {
		return execProtocolSend(ctx, c, nodeName, cmd.SpaceID, sub)
	}
	if cmd.Content == "" {
		return fmt.Errorf("send: --content is required")
	}
	if err := ensureNode(ctx, c, nodeName); err != nil {
		return err
	}
	var content map[string]interface{}
	if err := json.Unmarshal([]byte(cmd.Content), &content); err != nil {
		return fmt.Errorf("send: invalid content JSON: %s", err)
	}
	body := ioa.SendMessage{Content: content}
	if cmd.RefMsgs != "" {
		if body.Refs == nil {
			body.Refs = &ioa.Ref{}
		}
		body.Refs.Messages = splitComma(cmd.RefMsgs)
	}
	if cmd.RefNodes != "" {
		if body.Refs == nil {
			body.Refs = &ioa.Ref{}
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
	return writeJSON(msg)
}

func runReadDispatch(ctx context.Context, c *client.Client, nodeName string, cmd readCmd, active *goflags.Command) error {
	if sub := active.Active; sub != nil {
		return execProtocolRead(ctx, c, nodeName, cmd.SpaceID, sub)
	}
	if err := ensureNode(ctx, c, nodeName); err != nil {
		return err
	}
	msgs, err := c.Read(ctx, cmd.SpaceID, ioa.ReadOptions{
		MessageID: cmd.MessageID,
		After:     cmd.After,
		Limit:     cmd.Limit,
		All:       cmd.All,
	})
	if err != nil {
		return err
	}
	return writeJSON(msgs)
}

func execProtocolSend(ctx context.Context, c *client.Client, nodeName, spaceID string, sub *goflags.Command) error {
	if err := ensureNode(ctx, c, nodeName); err != nil {
		return err
	}
	def := findSendDef(sub.Name)
	if def == nil {
		return fmt.Errorf("send: unknown subcommand %q", sub.Name)
	}
	env := &protocols.Env{Client: c, SpaceID: spaceID, NodeName: nodeName, Data: def.Data}
	result, err := def.Execute(ctx, env)
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}

func execProtocolRead(ctx context.Context, c *client.Client, nodeName, spaceID string, sub *goflags.Command) error {
	if err := ensureNode(ctx, c, nodeName); err != nil {
		return err
	}
	def := findReadDef(sub.Name)
	if def == nil {
		return fmt.Errorf("read: unknown subcommand %q", sub.Name)
	}
	env := &protocols.Env{Client: c, SpaceID: spaceID, NodeName: nodeName, Data: def.Data}
	result, err := def.Execute(ctx, env)
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}

func findSendDef(name string) *protocols.SubcommandDef {
	for _, p := range protocols.All() {
		for i := range p.Send {
			if p.Send[i].Name == name {
				return &p.Send[i]
			}
		}
	}
	return nil
}

func findReadDef(name string) *protocols.SubcommandDef {
	for _, p := range protocols.All() {
		for i := range p.Read {
			if p.Read[i].Name == name {
				return &p.Read[i]
			}
		}
	}
	return nil
}

func ensureNode(ctx context.Context, c *client.Client, name string) error {
	if c.NodeID() != "" {
		return nil
	}
	_, err := c.RegisterNode(ctx, name, "", map[string]interface{}{})
	return err
}

func writeJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
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
