package server

import (
	"context"
	"reflect"
	"testing"

	"github.com/chainreactors/ioa"
)

func runStoreProtocolTest(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	service := NewService(store)

	nodeA, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-a"})
	if err != nil {
		t.Fatalf("RegisterNode(a) error = %v", err)
	}
	if nodeA.Meta == nil || len(nodeA.Meta) != 0 {
		t.Fatalf("nodeA meta = %#v, want empty map", nodeA.Meta)
	}
	nodeB, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-b"})
	if err != nil {
		t.Fatalf("RegisterNode(b) error = %v", err)
	}

	space, err := service.CreateSpace(ctx, nodeA.ID, ioa.SpaceCreate{Name: "case", Description: "owner"})
	if err != nil {
		t.Fatalf("CreateSpace() error = %v", err)
	}
	same, err := service.CreateSpace(ctx, nodeB.ID, ioa.SpaceCreate{Name: "case", Description: "reviewer"})
	if err != nil {
		t.Fatalf("CreateSpace(second) error = %v", err)
	}
	if same.ID != space.ID {
		t.Fatalf("space id = %s, want %s", same.ID, space.ID)
	}
	if len(same.Nodes) != 2 {
		t.Fatalf("space nodes = %#v, want 2 nodes", same.Nodes)
	}

	root, err := service.SendMessage(ctx, space.ID, nodeA.ID, ioa.SendMessage{Content: map[string]interface{}{"text": "root"}})
	if err != nil {
		t.Fatalf("SendMessage(root) error = %v", err)
	}
	if root.Refs.Messages == nil || root.Refs.Nodes == nil || len(root.Refs.Messages) != 0 || len(root.Refs.Nodes) != 0 {
		t.Fatalf("root refs = %#v, want empty slices", root.Refs)
	}
	directed, err := service.SendMessage(ctx, space.ID, nodeA.ID, ioa.SendMessage{
		Content: map[string]interface{}{"text": "to-b"},
		Refs:    &ioa.Ref{Nodes: []string{nodeB.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(directed) error = %v", err)
	}
	child, err := service.SendMessage(ctx, space.ID, nodeB.ID, ioa.SendMessage{
		Content: map[string]interface{}{"text": "child"},
		Refs:    &ioa.Ref{Messages: []string{root.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(child) error = %v", err)
	}

	start, err := service.ReadMessages(ctx, space.ID, "", ioa.ReadOptions{})
	if err != nil {
		t.Fatalf("ReadMessages(start) error = %v", err)
	}
	if got := messageIDs(start); !reflect.DeepEqual(got, []string{root.ID}) {
		t.Fatalf("start ids = %#v, want root only", got)
	}

	forNode, err := service.ReadMessages(ctx, space.ID, nodeB.ID, ioa.ReadOptions{})
	if err != nil {
		t.Fatalf("ReadMessages(node) error = %v", err)
	}
	if got := messageIDs(forNode); !reflect.DeepEqual(got, []string{directed.ID}) {
		t.Fatalf("node ids = %#v, want directed", got)
	}

	related, err := service.ReadMessages(ctx, space.ID, "", ioa.ReadOptions{MessageID: root.ID})
	if err != nil {
		t.Fatalf("ReadMessages(related) error = %v", err)
	}
	if got := messageIDs(related); !reflect.DeepEqual(got, []string{root.ID, child.ID}) {
		t.Fatalf("related ids = %#v, want root+child", got)
	}

	allAfter, err := service.ReadMessages(ctx, space.ID, "", ioa.ReadOptions{All: true, After: root.ID, Limit: 1})
	if err != nil {
		t.Fatalf("ReadMessages(all after) error = %v", err)
	}
	if got := messageIDs(allAfter); !reflect.DeepEqual(got, []string{directed.ID}) {
		t.Fatalf("all after ids = %#v, want directed only", got)
	}

	emptyContent, err := service.SendMessage(ctx, space.ID, nodeA.ID, ioa.SendMessage{Content: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("SendMessage(empty content) error = %v", err)
	}
	if emptyContent.Content == nil || len(emptyContent.Content) != 0 {
		t.Fatalf("empty content = %#v, want empty map", emptyContent.Content)
	}
	nilRef, err := service.SendMessage(ctx, space.ID, nodeA.ID, ioa.SendMessage{
		Content: map[string]interface{}{"text": "nil-ref-fields"},
		Refs:    &ioa.Ref{},
	})
	if err != nil {
		t.Fatalf("SendMessage(nil ref fields) error = %v", err)
	}
	if nilRef.Refs.Messages == nil || nilRef.Refs.Nodes == nil {
		t.Fatalf("nilRef refs = %#v, want non-nil empty slices", nilRef.Refs)
	}

	_, err = service.SendMessage(ctx, space.ID, nodeA.ID, ioa.SendMessage{
		Content: map[string]interface{}{"text": "bad"},
		Refs:    &ioa.Ref{Messages: []string{"missing"}},
	})
	if err == nil || statusOf(err) != 422 {
		t.Fatalf("missing ref error = %v, want 422", err)
	}
	_, err = service.SendMessage(ctx, space.ID, nodeA.ID, ioa.SendMessage{})
	if err == nil || statusOf(err) != 422 {
		t.Fatalf("nil content error = %v, want 422", err)
	}
	all, err := service.ReadMessages(ctx, space.ID, "", ioa.ReadOptions{All: true})
	if err != nil {
		t.Fatalf("ReadMessages(all) error = %v", err)
	}
	if containsMessageID(all, emptyContent.ID) == false || containsMessageID(all, nilRef.ID) == false {
		t.Fatalf("expected explicit default messages in all messages: %#v", all)
	}
}

func TestMemoryStoreProtocol(t *testing.T) {
	runStoreProtocolTest(t, NewMemoryStore())
}

func messageIDs(messages []ioa.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func containsMessageID(messages []ioa.Message, want string) bool {
	for _, message := range messages {
		if message.ID == want {
			return true
		}
	}
	return false
}
