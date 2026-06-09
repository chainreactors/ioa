package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/chainreactors/ioa/client"
	"github.com/chainreactors/ioa/protocols"
	"github.com/chainreactors/ioa/server"
	"github.com/chainreactors/ioa/skills"
	goflags "github.com/jessevdk/go-flags"

	_ "github.com/chainreactors/ioa/protocols/checkpoint"
	_ "github.com/chainreactors/ioa/protocols/handoff"
	_ "github.com/chainreactors/ioa/protocols/swarm"
	_ "github.com/chainreactors/ioa/protocols/team"
)

type options struct {
	URL      string `long:"url" env:"IOA_URL" description:"Server listen URL (serve) or target URL" default:"http://127.0.0.1:8765"`
	Token    string `long:"token" env:"IOA_TOKEN" description:"Auth token for authenticated requests"`
	NodeName string `long:"name" env:"IOA_NODE_NAME" description:"Node name for auto-registration" default:"ioa-client"`
	DB       string `long:"db" description:"SQLite database path (empty or :memory: for in-memory)" default:"./ioa.db"`
	Timeout  int    `long:"timeout" description:"Overall timeout in seconds" default:"3600"`
	Debug    bool   `long:"debug" description:"Enable debug logging"`
	Quiet    bool   `short:"q" long:"quiet" description:"Quiet mode"`
	JSON     bool   `long:"json" description:"Output results in JSON format"`

	// Client commands
	Init     initCmd     `command:"init" description:"Export protocol skills and schemas to .agent/skills/"`
	Register registerCmd `command:"register" description:"Register a new node and obtain a token"`
	Space    spaceCmd    `command:"space" description:"Create or join a space"`
	Send     sendCmd     `command:"send" description:"Send a message to a space"`
	Read     readCmd     `command:"read" description:"Read messages from a space"`

	// Server commands
	Serve    serveCmd    `command:"serve" description:"Start the IOA HTTP server"`
	Spaces   spacesCmd   `command:"spaces" description:"List all spaces"`
	Messages messagesCmd `command:"messages" description:"List start messages in a space"`
	Context  contextCmd  `command:"context" description:"View message thread/context"`
	Nodes    nodesCmd    `command:"nodes" description:"List nodes"`
}

// Client command structs

type initCmd struct {
	Output string `long:"output" short:"o" description:"Output directory" default:".agent/skills"`

	Positional struct {
		Skills []string `positional-arg-name:"skill" description:"Skill names to export (default: all)"`
	} `positional-args:"yes"`
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
	ContentType   string `long:"content-type" short:"t" description:"Message content type (e.g. checkpoint, handoff, team, swarm)"`
	Content       string `long:"content" short:"c" description:"Message content JSON"`
	RefMsgs       string `long:"ref-messages" description:"Comma-separated message IDs to reference"`
	RefNodes      string `long:"ref-nodes" description:"Comma-separated node IDs to target"`
	Meta          string `long:"meta" description:"Message metadata JSON"`
	ContentSchema string `long:"content-schema" description:"JSON Schema for content (declarative, per-message)"`
}

type readCmd struct {
	SpaceID   string `long:"space" short:"s" description:"Space ID" required:"yes"`
	MessageID string `long:"message" short:"m" description:"Message ID for context retrieval"`
	Direction string `long:"direction" short:"d" description:"Traversal direction: upstream, downstream (requires --message)"`
	After     string `long:"after" description:"Cursor: read messages after this ID"`
	Limit     int    `long:"limit" short:"l" description:"Maximum number of messages"`
	All       bool   `long:"all" short:"a" description:"Read all messages (not just addressed to this node)"`
	Listen    bool   `long:"listen" description:"Stream new messages via SSE (use with --message for thread-scoped)"`
}

// Server command structs

type serveCmd struct {
	AccessKey string `long:"access-key" env:"IOA_ACCESS_KEY" description:"Access key for client registration (enables auth when set)"`
}

type spacesCmd struct{}

type messagesCmd struct {
	Positional struct {
		Space string `positional-arg-name:"space" required:"yes"`
	} `positional-args:"yes"`
}

type contextCmd struct {
	Positional struct {
		Space     string `positional-arg-name:"space" required:"yes"`
		MessageID string `positional-arg-name:"message-id" required:"yes"`
	} `positional-args:"yes"`
}

type nodesCmd struct {
	Positional struct {
		Space string `positional-arg-name:"space"`
	} `positional-args:"yes"`
}

func openStore(opts options) (server.Store, func() error, string, error) {
	dbPath := opts.DB
	if dbPath == "" || dbPath == ":memory:" {
		store := server.NewMemoryStore()
		return store, store.Close, "memory", nil
	}
	if !filepath.IsAbs(dbPath) {
		if wd, err := os.Getwd(); err == nil {
			dbPath = filepath.Join(wd, dbPath)
		}
	}
	store, err := server.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open database: %s", err)
	}
	return store, store.Close, "sqlite:" + dbPath, nil
}

//	@title			IOA (Internet of Agent) API
//	@version		1.0
//	@description	IOA is a multi-agent communication protocol server. Nodes register themselves, create or join spaces, and exchange messages in real time via HTTP and Server-Sent Events.
//
//	@host		127.0.0.1:8765
//	@BasePath	/
func main() {
	var opts options
	parser := goflags.NewParser(&opts, goflags.Default&^goflags.PrintErrors)
	parser.Usage = `[OPTIONS] <command>

ioa - IOA (Internet of Agent) unified CLI

Client commands:
  init                 Export protocol skills and schemas to .agent/skills/
  register             Register a new node and obtain a token
  space <name> <desc>  Create or join a space
  send                 Send a message to a space
  read                 Read messages from a space

Server commands:
  serve                Start the IOA HTTP server
  spaces               List all spaces
  messages <space>     List start messages in a space
  context <space> <message-id>   View message thread
  nodes [space]        List nodes (optionally scoped to a space)

Examples:
  ioa serve --url http://127.0.0.1:8765 --db ./ioa.db
  ioa register --access-key mykey
  ioa space myspace "A new space"
  ioa send --space <id> --content '{"text":"hello"}'
  ioa read --space <id>
  ioa spaces
  ioa messages default
  ioa nodes`

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

	// Handle init (no client needed)
	if active.Name == "init" {
		if err := runInit(opts.Init); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	var err error
	switch active.Name {
	// Server commands
	case "serve":
		err = runServe(opts)
	case "spaces":
		err = runSpaces(opts)
	case "messages":
		err = runMessages(opts)
	case "context":
		err = runCtx(opts)
	case "nodes":
		err = runNodes(opts)
	// Client commands
	case "register":
		c, cerr := newAuthClient(opts)
		if cerr != nil {
			err = cerr
		} else {
			err = runRegister(context.Background(), c, opts.NodeName, opts.Register)
		}
	case "space":
		c, cerr := newAuthClient(opts)
		if cerr != nil {
			err = cerr
		} else {
			err = runSpace(context.Background(), c, opts.NodeName, opts.Space)
		}
	case "send":
		c, cerr := newAuthClient(opts)
		if cerr != nil {
			err = cerr
		} else {
			err = runSendDispatch(context.Background(), c, opts.NodeName, opts.Send, active)
		}
	case "read":
		c, cerr := newAuthClient(opts)
		if cerr != nil {
			err = cerr
		} else {
			err = runReadDispatch(context.Background(), c, opts.NodeName, opts.Read, active)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

// ──────────────────────────────────────────────────────────────────
// Protocol registration (client)
// ──────────────────────────────────────────────────────────────────

func registerProtocols(parser *goflags.Parser) {
	sendCommand := parser.Find("send")
	readCommand := parser.Find("read")

	for _, p := range protocols.All() {
		if p.Send != nil && sendCommand != nil {
			flags := p.Send.Flags
			if flags == nil {
				flags = &struct{}{}
			}
			sendCommand.AddCommand(p.Name, p.Send.Description, "", flags)
		}
		if p.Read != nil && readCommand != nil {
			flags := p.Read.Flags
			if flags == nil {
				flags = &struct{}{}
			}
			readCommand.AddCommand(p.Name, p.Read.Description, "", flags)
		}
	}

	if sendCommand != nil {
		sendCommand.SubcommandsOptional = true
	}
	if readCommand != nil {
		readCommand.SubcommandsOptional = true
	}
}

// ──────────────────────────────────────────────────────────────────
// Client helpers
// ──────────────────────────────────────────────────────────────────

func newAuthClient(opts options) (*client.Client, error) {
	if opts.Token != "" {
		return client.NewClientWithToken(opts.URL, opts.Token)
	}
	return client.NewClient(opts.URL, "")
}

func ensureNode(ctx context.Context, c *client.Client, name string) error {
	if c.NodeID() != "" {
		return nil
	}
	_, err := c.RegisterNode(ctx, name, "", map[string]interface{}{})
	return err
}

// ──────────────────────────────────────────────────────────────────
// Client command handlers
// ──────────────────────────────────────────────────────────────────

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
	msgs, _ := c.Read(ctx, info.ID, protocols.ReadOptions{All: true})
	var startMsgs []protocols.Message
	for _, m := range msgs {
		if len(m.Refs.Messages) == 0 && len(m.Refs.Nodes) == 0 {
			startMsgs = append(startMsgs, m)
		}
	}
	return writeJSON(struct {
		protocols.SpaceInfo
		StartMessages []protocols.Message `json:"start_messages"`
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
	return writeJSON(msg)
}

func runReadDispatch(ctx context.Context, c *client.Client, nodeName string, cmd readCmd, active *goflags.Command) error {
	if sub := active.Active; sub != nil {
		return execProtocolRead(ctx, c, nodeName, cmd.SpaceID, sub)
	}
	if err := ensureNode(ctx, c, nodeName); err != nil {
		return err
	}
	if cmd.Listen {
		return runReadListen(ctx, c, cmd)
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
	return writeJSON(msgs)
}

func runReadListen(ctx context.Context, c *client.Client, cmd readCmd) error {
	var opts []client.SubscribeOption
	if cmd.MessageID != "" {
		opts = append(opts, client.WithMessage(cmd.MessageID))
	}
	messages, errs, cancel, err := c.Subscribe(ctx, cmd.SpaceID, opts...)
	if err != nil {
		return err
	}
	defer cancel()

	enc := json.NewEncoder(os.Stdout)
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

func execProtocolSend(ctx context.Context, c *client.Client, nodeName, spaceID string, sub *goflags.Command) error {
	if err := ensureNode(ctx, c, nodeName); err != nil {
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
	fmt.Println(result)
	return nil
}

func execProtocolRead(ctx context.Context, c *client.Client, nodeName, spaceID string, sub *goflags.Command) error {
	if err := ensureNode(ctx, c, nodeName); err != nil {
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
	fmt.Println(result)
	return nil
}


func runInit(cmd initCmd) error {
	all, err := skills.LoadAll()
	if err != nil {
		return err
	}

	requested := cmd.Positional.Skills
	var toExport []skills.Skill
	if len(requested) == 0 {
		toExport = all
	} else {
		byName := make(map[string]skills.Skill, len(all))
		for _, s := range all {
			byName[s.Name] = s
		}
		for _, name := range requested {
			s, ok := byName[name]
			if !ok {
				return fmt.Errorf("unknown skill: %s", name)
			}
			toExport = append(toExport, s)
		}
	}

	for _, s := range toExport {
		dir := filepath.Join(cmd.Output, s.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}

		body, err := skills.ReadSkill(s.Name)
		if err == nil && body != "" {
			raw, _ := skills.ReadSkillRaw(s.Name)
			if len(raw) > 0 {
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), raw, 0o644); err != nil {
					return err
				}
			}
		}

		schemaRaw, err := skills.ReadSchemaRaw(s.Name)
		if err == nil {
			if err := os.WriteFile(filepath.Join(dir, "schema.json"), schemaRaw, 0o644); err != nil {
				return err
			}
		}

		fmt.Printf("  %s/ -> SKILL.md + schema.json\n", filepath.Join(cmd.Output, s.Name))
	}

	fmt.Printf("exported %d skills to %s\n", len(toExport), cmd.Output)
	return nil
}

// ──────────────────────────────────────────────────────────────────
// Server command handlers
// ──────────────────────────────────────────────────────────────────

func runServe(opts options) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.Timeout)*time.Second)
	defer cancel()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		count := 0
		for range sigChan {
			count++
			if count == 1 {
				log.Printf("[!] signal=shutdown action=graceful")
				cancel()
			} else {
				log.Printf("[!] signal=shutdown action=force_exit")
				os.Exit(1)
			}
		}
	}()

	store, closeStore, storeDescription, err := openStore(opts)
	if err != nil {
		return err
	}
	defer func() { _ = closeStore() }()

	log.Printf("[*] ioa_server store=%s", storeDescription)

	if opts.Serve.AccessKey != "" {
		log.Printf("[*] ioa_server auth=enabled")
	}

	return server.RunServer(ctx, server.ServerOptions{
		URL:       opts.URL,
		AccessKey: opts.Serve.AccessKey,
		Store:     store,
	})
}

func newClient(opts options) (*client.Client, error) {
	return client.NewClient(opts.URL, "")
}

func runSpaces(opts options) error {
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	spaces, err := c.ListSpaces(context.Background())
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(spaces)
	}
	if len(spaces) == 0 {
		fmt.Fprintln(os.Stderr, "no spaces found")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tNAME\tNODES\tMESSAGES\n")
	for _, s := range spaces {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", s.ID, s.Name, len(s.Nodes), s.MessageCount)
	}
	return w.Flush()
}

func runMessages(opts options) error {
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	space, err := c.ResolveSpace(context.Background(), opts.Messages.Positional.Space)
	if err != nil {
		return err
	}
	messages, err := c.ReadPublic(context.Background(), space.ID, protocols.ReadOptions{})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(messages)
	}
	if len(messages) == 0 {
		fmt.Fprintf(os.Stderr, "no start messages in space %q\n", space.Name)
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tSENDER\tCONTENT\n")
	for _, m := range messages {
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.ID, m.Sender, contentPreview(m.Content, 80))
	}
	return w.Flush()
}

func runCtx(opts options) error {
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	ctx := context.Background()
	space, err := c.ResolveSpace(ctx, opts.Context.Positional.Space)
	if err != nil {
		return err
	}
	msgID := opts.Context.Positional.MessageID
	messages, err := c.ReadPublic(ctx, space.ID, protocols.ReadOptions{MessageID: msgID})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(messages)
	}
	if len(messages) == 0 {
		fmt.Fprintf(os.Stderr, "no messages in context of %s\n", msgID)
		return nil
	}
	for _, m := range messages {
		marker := " "
		if m.ID == msgID {
			marker = "*"
		}
		fmt.Printf("%s [%s] %s:\n  %s\n", marker, m.ID, m.Sender, contentPreview(m.Content, 120))
	}
	return nil
}

func runNodes(opts options) error {
	c, err := newClient(opts)
	if err != nil {
		return err
	}
	ctx := context.Background()
	spaceName := opts.Nodes.Positional.Space

	if spaceName != "" {
		space, err := c.ResolveSpace(ctx, spaceName)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writeJSON(space.Nodes)
		}
		if len(space.Nodes) == 0 {
			fmt.Fprintf(os.Stderr, "no nodes in space %q\n", space.Name)
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "ID\tNAME\tDESCRIPTION\n")
		for _, n := range space.Nodes {
			fmt.Fprintf(w, "%s\t%s\t%s\n", n.ID, n.Name, n.Description)
		}
		return w.Flush()
	}

	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(nodes)
	}
	if len(nodes) == 0 {
		fmt.Fprintln(os.Stderr, "no nodes found")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tNAME\n")
	for _, n := range nodes {
		fmt.Fprintf(w, "%s\t%s\n", n.ID, n.Name)
	}
	return w.Flush()
}

// ──────────────────────────────────────────────────────────────────
// Shared helpers
// ──────────────────────────────────────────────────────────────────

func contentPreview(content map[string]interface{}, maxLen int) string {
	if text, ok := content["text"].(string); ok {
		if len(text) > maxLen {
			return text[:maxLen] + "..."
		}
		return text
	}
	data, _ := json.Marshal(content)
	s := string(data)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
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

