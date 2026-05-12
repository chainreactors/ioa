//go:build sqlite

package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/server"
)

func TestSQLiteStoreProtocol(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "ioa.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	service := server.NewService(store)

	nodeA, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-a"})
	if err != nil {
		t.Fatalf("RegisterNode(a) error = %v", err)
	}
	nodeB, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-b"})
	if err != nil {
		t.Fatalf("RegisterNode(b) error = %v", err)
	}

	space, err := service.CreateSpace(ctx, nodeA.ID, ioa.SpaceCreate{Name: "case", Description: "owner"})
	if err != nil {
		t.Fatalf("CreateSpace() error = %v", err)
	}

	root, err := service.SendMessage(ctx, space.ID, nodeA.ID, ioa.SendMessage{Content: map[string]interface{}{"text": "root"}})
	if err != nil {
		t.Fatalf("SendMessage(root) error = %v", err)
	}
	_, err = service.SendMessage(ctx, space.ID, nodeB.ID, ioa.SendMessage{
		Content: map[string]interface{}{"text": "child"},
		Refs:    &ioa.Ref{Messages: []string{root.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(child) error = %v", err)
	}

	all, err := service.ReadMessages(ctx, space.ID, "", ioa.ReadOptions{All: true})
	if err != nil {
		t.Fatalf("ReadMessages(all) error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d messages, want 2", len(all))
	}
}
