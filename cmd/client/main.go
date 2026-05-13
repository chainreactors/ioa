package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/client"
	goflags "github.com/jessevdk/go-flags"
)

type options struct {
	URL      string `long:"url" description:"IOA server URL" default:"http://127.0.0.1:8765"`
	NodeName string `long:"name" description:"Node name for auto-registration" default:"ioa-client"`

	Space spaceCmd `command:"space" description:"Create or join a space"`
	Send  sendCmd  `command:"send" description:"Send a message to a space"`
	Read  readCmd  `command:"read" description:"Read messages from a space"`
}

type spaceCmd struct {
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
  space <name> <description>   Create or join a space
  send  --space ID -c JSON     Send a message
  read  --space ID             Read messages

Examples:
  ioa space my-task "code reviewer"
  ioa send -s SPACE_ID -c '{"type":"task","task":"scan 192.168.1.0/24"}'
  ioa read -s SPACE_ID --all`

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*goflags.Error); ok && flagsErr.Type == goflags.ErrHelp {
			parser.WriteHelp(os.Stdout)
			return
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	c, err := client.NewClient(opts.URL, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	active := parser.Active
	if active == nil {
		fmt.Fprintln(os.Stderr, "error: missing subcommand: use space, send, or read")
		os.Exit(1)
	}

	nodeName := opts.NodeName
	if nodeName == "" {
		nodeName = "ioa-client"
	}

	switch active.Name {
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
	info, err := c.Space(ctx, cmd.Positional.Name, cmd.Positional.Description)
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
	for _, part := range split(s, ',') {
		part = trim(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func split(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
