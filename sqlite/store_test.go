//go:build sqlite

package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
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

	nodes, err := service.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes(empty) error = %v", err)
	}
	if nodes == nil || len(nodes) != 0 {
		t.Fatalf("ListNodes(empty) = %#v, want non-nil empty slice", nodes)
	}

	nodeA, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-a"})
	if err != nil {
		t.Fatalf("RegisterNode(a) error = %v", err)
	}
	nodeB, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-b"})
	if err != nil {
		t.Fatalf("RegisterNode(b) error = %v", err)
	}

	space, err := service.CreateSpace(ctx, nodeA.ID, ioa.SpaceCreate{
		Name:        "case",
		Description: "owner",
		Tags:        []string{"workspace:aide", "aide"},
	})
	if err != nil {
		t.Fatalf("CreateSpace() error = %v", err)
	}
	space, err = service.CreateSpace(ctx, nodeB.ID, ioa.SpaceCreate{
		Name:        "case",
		Description: "reviewer",
		Tags:        []string{"checkpoint", "aide"},
	})
	if err != nil {
		t.Fatalf("CreateSpace(join) error = %v", err)
	}
	if !reflect.DeepEqual(space.Tags, []string{"workspace:aide", "aide", "checkpoint"}) {
		t.Fatalf("space tags = %#v, want normalized merged tags", space.Tags)
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

	records, err := service.ListMessages(ctx, ioa.MessageFilter{SpaceID: space.ID, RefMessage: root.ID})
	if err != nil {
		t.Fatalf("ListMessages(ref_message) error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListMessages(ref_message) got %d messages, want 1", len(records))
	}
	graph, err := service.GetSpaceGraph(ctx, space.ID, ioa.GraphOptions{})
	if err != nil {
		t.Fatalf("GetSpaceGraph() error = %v", err)
	}
	if graph.Stats.SpaceCount != 1 || graph.Stats.MessageCount != 2 || graph.Stats.EdgeCount == 0 {
		t.Fatalf("graph stats = %#v, want one space, two messages, and edges", graph.Stats)
	}
}
