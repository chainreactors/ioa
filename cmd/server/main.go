package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/client"
	"github.com/chainreactors/ioa/server"
	goflags "github.com/jessevdk/go-flags"
)

type options struct {
	URL     string `long:"url" description:"Server listen URL (serve) or target URL (query)" default:"http://127.0.0.1:8765"`
	DB      string `long:"db" description:"SQLite database path when built with -tags sqlite" default:"./ioa.db"`
	Timeout int    `long:"timeout" description:"Overall timeout in seconds" default:"3600"`
	Debug   bool   `long:"debug" description:"Enable debug logging"`
	Quiet   bool   `short:"q" long:"quiet" description:"Quiet mode"`
	JSON    bool   `long:"json" description:"Output results in JSON format"`

	Serve    serveCmd    `command:"serve" description:"Start the IOA HTTP server"`
	Spaces   spacesCmd   `command:"spaces" description:"List all spaces"`
	Messages messagesCmd `command:"messages" description:"List start messages in a space"`
	Context  contextCmd  `command:"context" description:"View message thread/context"`
	Nodes    nodesCmd    `command:"nodes" description:"List nodes"`
}

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

var (
	openStore              = openMemoryStore
	withOptionalMiddleware = passThroughMiddleware
)

func openMemoryStore(opts options) (server.Store, func() error, string, error) {
	return server.NewMemoryStore(), func() error { return nil }, "memory", nil
}

func passThroughMiddleware(handler http.Handler, service *server.Service) http.Handler {
	return handler
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

ioa-server - IOA (Internet of Agent) server and management CLI

Commands:
  serve              Start the IOA HTTP server
  spaces             List all spaces
  messages <space>   List start messages in a space
  context <space> <message-id>   View message thread
  nodes [space]      List nodes (optionally scoped to a space)

Examples:
  ioa-server serve --url http://127.0.0.1:8765 --db ./ioa.db
  ioa-server spaces
  ioa-server messages default
  ioa-server nodes`

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
		fmt.Fprintln(os.Stderr, "error: missing subcommand: use serve, spaces, messages, context, or nodes")
		os.Exit(1)
	}

	logger := &stdLogger{debug: opts.Debug, quiet: opts.Quiet}

	var err error
	switch active.Name {
	case "serve":
		err = runServe(opts, logger)
	case "spaces":
		err = runSpaces(opts)
	case "messages":
		err = runMessages(opts)
	case "context":
		err = runCtx(opts)
	case "nodes":
		err = runNodes(opts)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func runServe(opts options, logger *stdLogger) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.Timeout)*time.Second)
	defer cancel()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		count := 0
		for range sigChan {
			count++
			if count == 1 {
				logger.Warnf("signal=shutdown action=graceful")
				cancel()
			} else {
				logger.Warnf("signal=shutdown action=force_exit")
				os.Exit(1)
			}
		}
	}()

	store, closeStore, storeDescription, err := openStore(opts)
	if err != nil {
		return err
	}
	defer func() { _ = closeStore() }()

	logger.Importantf("ioa_server store=%s", storeDescription)

	if opts.Serve.AccessKey != "" {
		logger.Importantf("ioa_server auth=enabled")
	}

	return server.RunServer(ctx, server.ServerOptions{
		URL:        opts.URL,
		AccessKey:  opts.Serve.AccessKey,
		Store:      store,
		Middleware: withOptionalMiddleware,
		Logger:     logger,
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
		return writeJSONOut(spaces)
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
	messages, err := c.ReadPublic(context.Background(), space.ID, ioa.ReadOptions{})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSONOut(messages)
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
	messages, err := c.ReadPublic(ctx, space.ID, ioa.ReadOptions{MessageID: msgID})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSONOut(messages)
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
			return writeJSONOut(space.Nodes)
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
		return writeJSONOut(nodes)
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

func writeJSONOut(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type stdLogger struct {
	debug bool
	quiet bool
}

func (l *stdLogger) Debugf(format string, args ...interface{}) {
	if l.debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

func (l *stdLogger) Infof(format string, args ...interface{}) {
	if !l.quiet {
		fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
	}
}

func (l *stdLogger) Warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
}

func (l *stdLogger) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}

func (l *stdLogger) Importantf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[*] "+format+"\n", args...)
}
