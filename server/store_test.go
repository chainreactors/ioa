package server

import (
	"context"
	"reflect"
	"testing"

	"github.com/chainreactors/ioa/protocols"
)

func runStoreProtocolTest(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	service := NewService(store, "")

	nodeA, err := service.RegisterNode(ctx, protocols.NodeCreate{Name: "agent-a"})
	if err != nil {
		t.Fatalf("RegisterNode(a) error = %v", err)
	}
	if nodeA.Meta == nil || len(nodeA.Meta) != 0 {
		t.Fatalf("nodeA meta = %#v, want empty map", nodeA.Meta)
	}
	nodeB, err := service.RegisterNode(ctx, protocols.NodeCreate{Name: "agent-b"})
	if err != nil {
		t.Fatalf("RegisterNode(b) error = %v", err)
	}

	space, err := service.CreateSpace(ctx, nodeA.ID, protocols.SpaceCreate{
		Name:        "case",
		Description: "owner",
		Tags:        []string{"workspace:aide", "aide", "workspace:aide"},
	})
	if err != nil {
		t.Fatalf("CreateSpace() error = %v", err)
	}
	same, err := service.CreateSpace(ctx, nodeB.ID, protocols.SpaceCreate{
		Name:        "case",
		Description: "reviewer",
		Tags:        []string{"checkpoint"},
	})
	if err != nil {
		t.Fatalf("CreateSpace(second) error = %v", err)
	}
	if same.ID != space.ID {
		t.Fatalf("space id = %s, want %s", same.ID, space.ID)
	}
	if len(same.Nodes) != 2 {
		t.Fatalf("space nodes = %#v, want 2 nodes", same.Nodes)
	}
	if !reflect.DeepEqual(same.Tags, []string{"workspace:aide", "aide", "checkpoint"}) {
		t.Fatalf("space tags = %#v, want normalized merged tags", same.Tags)
	}

	root, err := service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{Content: map[string]interface{}{"text": "root"}})
	if err != nil {
		t.Fatalf("SendMessage(root) error = %v", err)
	}
	if root.Refs.Messages == nil || root.Refs.Nodes == nil || len(root.Refs.Messages) != 0 || len(root.Refs.Nodes) != 0 {
		t.Fatalf("root refs = %#v, want empty slices", root.Refs)
	}
	directed, err := service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "to-b"},
		Refs:    &protocols.Ref{Nodes: []string{nodeB.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(directed) error = %v", err)
	}
	child, err := service.SendMessage(ctx, space.ID, nodeB.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "child"},
		Refs:    &protocols.Ref{Messages: []string{root.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(child) error = %v", err)
	}

	start, err := service.ReadMessages(ctx, space.ID, "", protocols.ReadOptions{})
	if err != nil {
		t.Fatalf("ReadMessages(start) error = %v", err)
	}
	if got := messageIDs(start); !reflect.DeepEqual(got, []string{root.ID}) {
		t.Fatalf("start ids = %#v, want root only", got)
	}

	forNode, err := service.ReadMessages(ctx, space.ID, nodeB.ID, protocols.ReadOptions{})
	if err != nil {
		t.Fatalf("ReadMessages(node) error = %v", err)
	}
	if got := messageIDs(forNode); !reflect.DeepEqual(got, []string{root.ID, directed.ID}) {
		t.Fatalf("node ids = %#v, want root(broadcast)+directed", got)
	}

	related, err := service.ReadMessages(ctx, space.ID, "", protocols.ReadOptions{MessageID: root.ID})
	if err != nil {
		t.Fatalf("ReadMessages(related) error = %v", err)
	}
	if got := messageIDs(related); !reflect.DeepEqual(got, []string{root.ID, child.ID}) {
		t.Fatalf("related ids = %#v, want root+child", got)
	}

	allAfter, err := service.ReadMessages(ctx, space.ID, "", protocols.ReadOptions{All: true, After: root.ID, Limit: 1})
	if err != nil {
		t.Fatalf("ReadMessages(all after) error = %v", err)
	}
	if got := messageIDs(allAfter); !reflect.DeepEqual(got, []string{directed.ID}) {
		t.Fatalf("all after ids = %#v, want directed only", got)
	}

	emptyContent, err := service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{Content: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("SendMessage(empty content) error = %v", err)
	}
	if emptyContent.Content == nil || len(emptyContent.Content) != 0 {
		t.Fatalf("empty content = %#v, want empty map", emptyContent.Content)
	}
	nilRef, err := service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "nil-ref-fields"},
		Refs:    &protocols.Ref{},
	})
	if err != nil {
		t.Fatalf("SendMessage(nil ref fields) error = %v", err)
	}
	if nilRef.Refs.Messages == nil || nilRef.Refs.Nodes == nil {
		t.Fatalf("nilRef refs = %#v, want non-nil empty slices", nilRef.Refs)
	}

	_, err = service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "bad"},
		Refs:    &protocols.Ref{Messages: []string{"missing"}},
	})
	if err == nil || statusOf(err) != 422 {
		t.Fatalf("missing ref error = %v, want 422", err)
	}
	_, err = service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{})
	if err == nil || statusOf(err) != 422 {
		t.Fatalf("nil content error = %v, want 422", err)
	}
	all, err := service.ReadMessages(ctx, space.ID, "", protocols.ReadOptions{All: true})
	if err != nil {
		t.Fatalf("ReadMessages(all) error = %v", err)
	}
	if containsMessageID(all, emptyContent.ID) == false || containsMessageID(all, nilRef.ID) == false {
		t.Fatalf("expected explicit default messages in all messages: %#v", all)
	}

	// Meta round-trip
	metaMsg, err := service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "with-meta"},
		Meta:    map[string]interface{}{"kind": "plan", "labels": []interface{}{"security", "review"}},
	})
	if err != nil {
		t.Fatalf("SendMessage(meta) error = %v", err)
	}
	if metaMsg.Meta == nil {
		t.Fatalf("meta = nil, want non-nil")
	}
	if metaMsg.Meta["kind"] != "plan" {
		t.Fatalf("meta.kind = %v, want plan", metaMsg.Meta["kind"])
	}
	readAll, err := service.ReadMessages(ctx, space.ID, "", protocols.ReadOptions{All: true})
	if err != nil {
		t.Fatalf("ReadMessages(all after meta) error = %v", err)
	}
	var found bool
	for _, msg := range readAll {
		if msg.ID == metaMsg.ID && msg.Meta != nil && msg.Meta["kind"] == "plan" {
			found = true
		}
	}
	if !found {
		t.Fatalf("meta not preserved in read messages")
	}

	// No meta — omitted
	noMeta, err := service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "no-meta"},
	})
	if err != nil {
		t.Fatalf("SendMessage(no meta) error = %v", err)
	}
	if noMeta.Meta != nil {
		t.Fatalf("no-meta msg meta = %#v, want nil", noMeta.Meta)
	}
}

func runContentSchemaTest(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	service := NewService(store, "")

	node, err := service.RegisterNode(ctx, protocols.NodeCreate{Name: "agent"})
	if err != nil {
		t.Fatalf("RegisterNode error = %v", err)
	}
	space, err := service.CreateSpace(ctx, node.ID, protocols.SpaceCreate{Name: "schema-test", Description: "tester"})
	if err != nil {
		t.Fatalf("CreateSpace error = %v", err)
	}

	schemaA := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"type": map[string]interface{}{"type": "string"},
			"body": map[string]interface{}{"type": "string"},
		},
		"required": []interface{}{"type", "body"},
	}

	// Thread 1: root message sets schemaA
	root1, err := service.SendMessage(ctx, space.ID, node.ID, protocols.SendMessage{
		Content:       map[string]interface{}{"text": "thread 1 root"},
		ContentSchema: schemaA,
	})
	if err != nil {
		t.Fatalf("SendMessage(root1) error = %v", err)
	}
	if root1.ContentSchema == nil {
		t.Fatalf("root1.ContentSchema = nil, want schemaA")
	}

	// Declarative schema: replies are NOT constrained by root's schema
	_, err = service.SendMessage(ctx, space.ID, node.ID, protocols.SendMessage{
		Content: map[string]interface{}{"type": "task", "body": "do something"},
		Refs:    &protocols.Ref{Messages: []string{root1.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(thread1 reply) error = %v", err)
	}

	// Any content shape in a reply is accepted (no constraint inheritance)
	_, err = service.SendMessage(ctx, space.ID, node.ID, protocols.SendMessage{
		Content: map[string]interface{}{"arbitrary": "fields"},
		Refs:    &protocols.Ref{Messages: []string{root1.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(thread1 different shape) error = %v", err)
	}

	// Standalone root without schema: any content passes
	_, err = service.SendMessage(ctx, space.ID, node.ID, protocols.SendMessage{
		Content: map[string]interface{}{"arbitrary": "data"},
	})
	if err != nil {
		t.Fatalf("SendMessage(no-schema root) error = %v", err)
	}

	// Invalid schema is still rejected (schema itself must be valid)
	_, err = service.SendMessage(ctx, space.ID, node.ID, protocols.SendMessage{
		Content:       map[string]interface{}{"text": "bad schema"},
		ContentSchema: map[string]interface{}{"type": 123},
	})
	if err == nil || statusOf(err) != 422 {
		t.Fatalf("SendMessage(invalid schema) error = %v, want 422", err)
	}

	// content_type is stored and returned
	typed, err := service.SendMessage(ctx, space.ID, node.ID, protocols.SendMessage{
		ContentType: "checkpoint",
		Content:     map[string]interface{}{"id": "cp-1", "title": "test"},
	})
	if err != nil {
		t.Fatalf("SendMessage(content_type) error = %v", err)
	}
	if typed.ContentType != "checkpoint" {
		t.Fatalf("typed.ContentType = %q, want %q", typed.ContentType, "checkpoint")
	}
	if protocols.MessageContentType(typed) != "checkpoint" {
		t.Fatalf("MessageContentType = %q, want checkpoint", protocols.MessageContentType(typed))
	}
}

func runProjectionTest(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	service := NewService(store, "")

	nodeA, err := service.RegisterNode(ctx, protocols.NodeCreate{Name: "agent-a"})
	if err != nil {
		t.Fatalf("RegisterNode(a) error = %v", err)
	}
	nodeB, err := service.RegisterNode(ctx, protocols.NodeCreate{Name: "agent-b"})
	if err != nil {
		t.Fatalf("RegisterNode(b) error = %v", err)
	}
	nodeC, err := service.RegisterNode(ctx, protocols.NodeCreate{Name: "agent-c"})
	if err != nil {
		t.Fatalf("RegisterNode(c) error = %v", err)
	}

	space, err := service.CreateSpace(ctx, nodeA.ID, protocols.SpaceCreate{Name: "case", Description: "owner"})
	if err != nil {
		t.Fatalf("CreateSpace(case) error = %v", err)
	}
	if _, err := service.CreateSpace(ctx, nodeB.ID, protocols.SpaceCreate{Name: "case", Description: "reviewer"}); err != nil {
		t.Fatalf("CreateSpace(join case) error = %v", err)
	}
	otherSpace, err := service.CreateSpace(ctx, nodeC.ID, protocols.SpaceCreate{Name: "other", Description: "observer"})
	if err != nil {
		t.Fatalf("CreateSpace(other) error = %v", err)
	}

	root, err := service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{Content: map[string]interface{}{"text": "root"}})
	if err != nil {
		t.Fatalf("SendMessage(root) error = %v", err)
	}
	directed, err := service.SendMessage(ctx, space.ID, nodeA.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "to-b"},
		Refs:    &protocols.Ref{Nodes: []string{nodeB.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(directed) error = %v", err)
	}
	child, err := service.SendMessage(ctx, space.ID, nodeB.ID, protocols.SendMessage{
		Content: map[string]interface{}{"text": "child"},
		Refs:    &protocols.Ref{Messages: []string{root.ID}},
	})
	if err != nil {
		t.Fatalf("SendMessage(child) error = %v", err)
	}
	other, err := service.SendMessage(ctx, otherSpace.ID, nodeC.ID, protocols.SendMessage{Content: map[string]interface{}{"text": "other"}})
	if err != nil {
		t.Fatalf("SendMessage(other) error = %v", err)
	}

	all, err := service.ListMessages(ctx, protocols.MessageFilter{})
	if err != nil {
		t.Fatalf("ListMessages(all) error = %v", err)
	}
	if len(all) != 4 || !containsRecordID(all, other.ID) {
		t.Fatalf("ListMessages(all) = %#v, want four cross-space messages", recordIDs(all))
	}
	scoped, err := service.ListMessages(ctx, protocols.MessageFilter{SpaceID: space.ID})
	if err != nil {
		t.Fatalf("ListMessages(space) error = %v", err)
	}
	if got := recordIDs(scoped); !reflect.DeepEqual(got, []string{root.ID, directed.ID, child.ID}) {
		t.Fatalf("ListMessages(space) ids = %#v, want root,directed,child", got)
	}
	connectedToB, err := service.ListMessages(ctx, protocols.MessageFilter{SpaceID: space.ID, NodeID: nodeB.ID})
	if err != nil {
		t.Fatalf("ListMessages(node_id) error = %v", err)
	}
	if got := recordIDs(connectedToB); !reflect.DeepEqual(got, []string{directed.ID, child.ID}) {
		t.Fatalf("ListMessages(node_id) ids = %#v, want directed,child", got)
	}
	refMessage, err := service.ListMessages(ctx, protocols.MessageFilter{SpaceID: space.ID, RefMessage: root.ID})
	if err != nil {
		t.Fatalf("ListMessages(ref_message) error = %v", err)
	}
	if got := recordIDs(refMessage); !reflect.DeepEqual(got, []string{child.ID}) {
		t.Fatalf("ListMessages(ref_message) ids = %#v, want child", got)
	}
	refNode, err := service.ListMessages(ctx, protocols.MessageFilter{SpaceID: space.ID, RefNode: nodeB.ID})
	if err != nil {
		t.Fatalf("ListMessages(ref_node) error = %v", err)
	}
	if got := recordIDs(refNode); !reflect.DeepEqual(got, []string{directed.ID}) {
		t.Fatalf("ListMessages(ref_node) ids = %#v, want directed", got)
	}
}

func TestMemoryStoreProtocol(t *testing.T) {
	store := NewMemoryStore()
	t.Cleanup(func() { store.Close() })
	runStoreProtocolTest(t, store)
}

func TestMemoryStoreContentSchema(t *testing.T) {
	store := NewMemoryStore()
	t.Cleanup(func() { store.Close() })
	runContentSchemaTest(t, store)
}

func TestMemoryStoreProjections(t *testing.T) {
	store := NewMemoryStore()
	t.Cleanup(func() { store.Close() })
	runProjectionTest(t, store)
}

func messageIDs(messages []protocols.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func containsMessageID(messages []protocols.Message, want string) bool {
	for _, message := range messages {
		if message.ID == want {
			return true
		}
	}
	return false
}

func recordIDs(messages []protocols.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func containsRecordID(messages []protocols.Message, want string) bool {
	for _, message := range messages {
		if message.ID == want {
			return true
		}
	}
	return false
}

