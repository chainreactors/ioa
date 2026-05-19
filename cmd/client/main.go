package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/client"
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
	Tags []string `long:"tag" description:"Space tag. Repeat for multiple tags."`

	Positional struct {
		Name        string `positional-arg-name:"name" required:"yes"`
		Description string `positional-arg-name:"description" required:"yes"`
	} `positional-args:"yes"`
}

type sendCmd struct {
	SpaceID       string `long:"space" short:"s" description:"Space ID" required:"yes"`
	Content       string `long:"content" short:"c" description:"Message content JSON" required:"yes"`
	RefMsgs       string `long:"ref-messages" description:"Comma-separated message IDs to reference"`
	RefNodes      string `long:"ref-nodes" description:"Comma-separated node IDs to target"`
	ContentSchema string `long:"content-schema" description:"JSON Schema for space content validation"`
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
	parser.Usage = `[OPTIONS] <command>

ioa - IOA (Internet of Agent) client

Commands:
  register --access-key KEY    Register a new node (returns token)
  space <name> <description>   Create or join a space
  send  --space ID -c JSON     Send a message
  read  --space ID             Read messages

Environment:
  IOA_URL          Server URL (default http://127.0.0.1:8765)
  IOA_TOKEN        Auth token for authenticated requests
  IOA_NODE_NAME    Node name (default ioa-client)
  IOA_ACCESS_KEY   Access key for registration

Examples:
  ioa register --access-key mykey --name alice
  ioa --token TOKEN space my-task "code reviewer"
  IOA_TOKEN=TOKEN ioa send -s SPACE_ID -c '{"text":"hello"}'`

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
		fmt.Fprintln(os.Stderr, "error: missing subcommand: use register, space, send, or read")
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
	if nodeName == "" {
		nodeName = "ioa-client"
	}

	switch active.Name {
	case "register":
		err = runRegister(ctx, c, nodeName, opts.Register)
	case "space":
		err = runSpace(ctx, c, nodeName, opts.Space)
	case "send":
		err = runSend(ctx, c, nodeName, opts.Send)
	case "read":
		err = runRead(ctx, c, nodeName, opts.Read)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func runRegister(ctx context.Context, c *client.Client, nodeName string, cmd registerCmd) error {
	resp, err := c.Register(ctx, cmd.AccessKey, nodeName, map[string]interface{}{})
	if err != nil {
		return err
	}
	return writeJSON(resp)
}

func ensureNode(ctx context.Context, c *client.Client, name string) error {
	if c.NodeID() != "" {
		return nil
	}
	_, err := c.RegisterNode(ctx, name, map[string]interface{}{})
	return err
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
	result := struct {
		ioa.SpaceInfo
		StartMessages []ioa.Message `json:"start_messages"`
	}{SpaceInfo: info, StartMessages: startMsgs}
	return writeJSON(result)
}

func runSend(ctx context.Context, c *client.Client, nodeName string, cmd sendCmd) error {
	if err := ensureNode(ctx, c, nodeName); err != nil {
		return err
	}
	var content map[string]interface{}
	if err := json.Unmarshal([]byte(cmd.Content), &content); err != nil {
		return fmt.Errorf("invalid content JSON: %s", err)
	}
	var refs *ioa.Ref
	if cmd.RefMsgs != "" || cmd.RefNodes != "" {
		refs = &ioa.Ref{}
		if cmd.RefMsgs != "" {
			refs.Messages = splitComma(cmd.RefMsgs)
		}
		if cmd.RefNodes != "" {
			refs.Nodes = splitComma(cmd.RefNodes)
		}
	}
	body := ioa.SendMessage{Content: content, Refs: refs}
	if cmd.ContentSchema != "" {
		var schema map[string]interface{}
		if err := json.Unmarshal([]byte(cmd.ContentSchema), &schema); err != nil {
			return fmt.Errorf("invalid content-schema JSON: %s", err)
		}
		body.ContentSchema = schema
	}
	msg, err := c.Send(ctx, cmd.SpaceID, body)
	if err != nil {
		return err
	}
	return writeJSON(msg)
}

func runRead(ctx context.Context, c *client.Client, nodeName string, cmd readCmd) error {
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
