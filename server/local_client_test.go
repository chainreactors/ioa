package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chainreactors/ioa/client"
	"github.com/chainreactors/ioa/protocols"
)

func TestClientTransports(t *testing.T) {
	for _, transport := range []string{"local", "http"} {
		t.Run(transport, func(t *testing.T) {
			store := NewMemoryStore()
			defer store.Close()
			svc := NewService(store, "")
			var a, b client.StreamAPI
			if transport == "local" {
				a, b = NewLocalClient(svc, ""), NewLocalClient(svc, "")
			} else {
				h := httptest.NewServer(NewHandler(svc))
				defer h.Close()
				var err error
				a, err = client.NewClient(h.URL, "")
				if err != nil {
					t.Fatal(err)
				}
				b, err = client.NewClient(h.URL, "")
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for _, c := range []client.StreamAPI{a, b} {
				if _, err := c.RegisterNode(ctx, "agent", "agent", nil); err != nil {
					t.Fatal(err)
				}
			}
			space, err := a.Space(ctx, "work", "work")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = b.Space(ctx, "work", "work"); err != nil {
				t.Fatal(err)
			}
			feed, errs, stop, err := b.Subscribe(ctx, space.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			first, err := a.Send(ctx, space.ID, protocols.SendMessage{Content: map[string]any{"text": "work"}, Refs: &protocols.Ref{Nodes: []string{b.NodeID()}}})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case msg := <-feed:
				if msg.ID != first.ID || msg.Sender != a.NodeID() {
					t.Fatalf("message = %#v", msg)
				}
			case err := <-errs:
				t.Fatal(err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			reply, err := b.Send(ctx, space.ID, protocols.SendMessage{Content: map[string]any{"text": "result"}, Refs: &protocols.Ref{Messages: []string{first.ID}}})
			if err != nil {
				t.Fatal(err)
			}
			history, err := a.Read(ctx, space.ID, protocols.ReadOptions{MessageID: first.ID, Direction: "downstream", All: true})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, msg := range history {
				if msg.ID == reply.ID {
					found = true
				}
			}
			if !found {
				t.Fatalf("reply not in history: %#v", history)
			}
			if _, err := a.Send(ctx, space.ID, protocols.SendMessage{Content: map[string]any{"text": "bad"}, Refs: &protocols.Ref{Messages: []string{"missing"}}}); err == nil {
				t.Fatal("invalid ref accepted")
			}
			stop()
			stop()
		})
	}
}
