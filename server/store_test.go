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
	service := NewService(store, "")

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

	space, err := service.CreateSpace(ctx, nodeA.ID, ioa.SpaceCreate{
		Name:        "case",
		Description: "owner",
		Tags:        []string{"workspace:aide", "aide", "workspace:aide"},
	})
	if err != nil {
		t.Fatalf("CreateSpace() error = %v", err)
	}
	same, err := service.CreateSpace(ctx, nodeB.ID, ioa.SpaceCreate{
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

func runContentSchemaTest(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	service := NewService(store, "")

	node, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent"})
	if err != nil {
		t.Fatalf("RegisterNode error = %v", err)
	}
	space, err := service.CreateSpace(ctx, node.ID, ioa.SpaceCreate{Name: "schema-test", Description: "tester"})
	if err != nil {
		t.Fatalf("CreateSpace error = %v", err)
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"type": map[string]interface{}{"type": "string"},
			"body": map[string]interface{}{"type": "string"},
		},
		"required": []interface{}{"type", "body"},
	}

	// Setting schema — the message itself is not validated
	_, err = service.SendMessage(ctx, space.ID, node.ID, ioa.SendMessage{
		Content:       map[string]interface{}{"text": "I set the schema"},
		ContentSchema: schema,
	})
	if err != nil {
		t.Fatalf("SendMessage(set schema) error = %v", err)
	}

	// SpaceInfo should include the schema
	info, err := service.GetSpace(ctx, space.ID)
	if err != nil {
		t.Fatalf("GetSpace error = %v", err)
	}
	if info.ContentSchema == nil {
		t.Fatalf("SpaceInfo.ContentSchema = nil, want schema")
	}

	// Compliant message passes
	_, err = service.SendMessage(ctx, space.ID, node.ID, ioa.SendMessage{
		Content: map[string]interface{}{"type": "task", "body": "do something"},
	})
	if err != nil {
		t.Fatalf("SendMessage(compliant) error = %v", err)
	}

	// Non-compliant message fails with 422
	_, err = service.SendMessage(ctx, space.ID, node.ID, ioa.SendMessage{
		Content: map[string]interface{}{"wrong": "fields"},
	})
	if err == nil || statusOf(err) != 422 {
		t.Fatalf("SendMessage(non-compliant) error = %v, want 422", err)
	}

	// Update schema to something different
	newSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"status": map[string]interface{}{"type": "string"},
		},
		"required": []interface{}{"status"},
	}
	_, err = service.SendMessage(ctx, space.ID, node.ID, ioa.SendMessage{
		Content:       map[string]interface{}{"text": "updating schema"},
		ContentSchema: newSchema,
	})
	if err != nil {
		t.Fatalf("SendMessage(update schema) error = %v", err)
	}

	// Old-format message now fails
	_, err = service.SendMessage(ctx, space.ID, node.ID, ioa.SendMessage{
		Content: map[string]interface{}{"type": "task", "body": "do something"},
	})
	if err == nil || statusOf(err) != 422 {
		t.Fatalf("SendMessage(old format) error = %v, want 422", err)
	}

	// New-format message passes
	_, err = service.SendMessage(ctx, space.ID, node.ID, ioa.SendMessage{
		Content: map[string]interface{}{"status": "done"},
	})
	if err != nil {
		t.Fatalf("SendMessage(new format) error = %v", err)
	}

	// Invalid schema is rejected
	_, err = service.SendMessage(ctx, space.ID, node.ID, ioa.SendMessage{
		Content:       map[string]interface{}{"text": "bad schema"},
		ContentSchema: map[string]interface{}{"type": 123},
	})
	if err == nil || statusOf(err) != 422 {
		t.Fatalf("SendMessage(invalid schema) error = %v, want 422", err)
	}
}

func runProjectionTest(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	service := NewService(store, "")

	nodeA, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-a"})
	if err != nil {
		t.Fatalf("RegisterNode(a) error = %v", err)
	}
	nodeB, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-b"})
	if err != nil {
		t.Fatalf("RegisterNode(b) error = %v", err)
	}
	nodeC, err := service.RegisterNode(ctx, ioa.NodeCreate{Name: "agent-c"})
	if err != nil {
		t.Fatalf("RegisterNode(c) error = %v", err)
	}

	space, err := service.CreateSpace(ctx, nodeA.ID, ioa.SpaceCreate{Name: "case", Description: "owner"})
	if err != nil {
		t.Fatalf("CreateSpace(case) error = %v", err)
	}
	if _, err := service.CreateSpace(ctx, nodeB.ID, ioa.SpaceCreate{Name: "case", Description: "reviewer"}); err != nil {
		t.Fatalf("CreateSpace(join case) error = %v", err)
	}
	otherSpace, err := service.CreateSpace(ctx, nodeC.ID, ioa.SpaceCreate{Name: "other", Description: "observer"})
	if err != nil {
		t.Fatalf("CreateSpace(other) error = %v", err)
	}

	root, err := service.SendMessage(ctx, space.ID, nodeA.ID, ioa.SendMessage{Content: map[string]interface{}{"text": "root"}})
	if err != nil {
		t.Fatalf("SendMessage(root) error = %v", err)
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
	other, err := service.SendMessage(ctx, otherSpace.ID, nodeC.ID, ioa.SendMessage{Content: map[string]interface{}{"text": "other"}})
	if err != nil {
		t.Fatalf("SendMessage(other) error = %v", err)
	}

	all, err := service.ListMessages(ctx, ioa.MessageFilter{})
	if err != nil {
		t.Fatalf("ListMessages(all) error = %v", err)
	}
	if len(all) != 4 || !containsRecordID(all, other.ID) {
		t.Fatalf("ListMessages(all) = %#v, want four cross-space messages", recordIDs(all))
	}
	scoped, err := service.ListMessages(ctx, ioa.MessageFilter{SpaceID: space.ID})
	if err != nil {
		t.Fatalf("ListMessages(space) error = %v", err)
	}
	if got := recordIDs(scoped); !reflect.DeepEqual(got, []string{root.ID, directed.ID, child.ID}) {
		t.Fatalf("ListMessages(space) ids = %#v, want root,directed,child", got)
	}
	connectedToB, err := service.ListMessages(ctx, ioa.MessageFilter{SpaceID: space.ID, NodeID: nodeB.ID})
	if err != nil {
		t.Fatalf("ListMessages(node_id) error = %v", err)
	}
	if got := recordIDs(connectedToB); !reflect.DeepEqual(got, []string{directed.ID, child.ID}) {
		t.Fatalf("ListMessages(node_id) ids = %#v, want directed,child", got)
	}
	refMessage, err := service.ListMessages(ctx, ioa.MessageFilter{SpaceID: space.ID, RefMessage: root.ID})
	if err != nil {
		t.Fatalf("ListMessages(ref_message) error = %v", err)
	}
	if got := recordIDs(refMessage); !reflect.DeepEqual(got, []string{child.ID}) {
		t.Fatalf("ListMessages(ref_message) ids = %#v, want child", got)
	}
	refNode, err := service.ListMessages(ctx, ioa.MessageFilter{SpaceID: space.ID, RefNode: nodeB.ID})
	if err != nil {
		t.Fatalf("ListMessages(ref_node) error = %v", err)
	}
	if got := recordIDs(refNode); !reflect.DeepEqual(got, []string{directed.ID}) {
		t.Fatalf("ListMessages(ref_node) ids = %#v, want directed", got)
	}

	graph, err := service.GetSpaceGraph(ctx, space.ID, ioa.GraphOptions{})
	if err != nil {
		t.Fatalf("GetSpaceGraph() error = %v", err)
	}
	if graph.Stats.SpaceCount != 1 || graph.Stats.MessageCount != 3 {
		t.Fatalf("graph stats = %#v, want one space and three messages", graph.Stats)
	}
	for _, edge := range []ioa.GraphEdge{
		{Source: "space:" + space.ID, Target: "node:" + nodeA.ID, Kind: "member"},
		{Source: "space:" + space.ID, Target: "node:" + nodeB.ID, Kind: "member"},
		{Source: "node:" + nodeA.ID, Target: "message:" + root.ID, Kind: "sender"},
		{Source: "node:" + nodeB.ID, Target: "message:" + child.ID, Kind: "sender"},
		{Source: "message:" + child.ID, Target: "message:" + root.ID, Kind: "refs.messages"},
		{Source: "message:" + directed.ID, Target: "node:" + nodeB.ID, Kind: "refs.nodes"},
	} {
		if !hasGraphEdge(graph, edge) {
			t.Fatalf("graph missing edge %#v in %#v", edge, graph.Edges)
		}
	}

	related, err := service.GetGraph(ctx, ioa.GraphOptions{MessageFilter: ioa.MessageFilter{MessageID: root.ID}})
	if err != nil {
		t.Fatalf("GetGraph(message_id) error = %v", err)
	}
	if got := recordIDs(related.Messages); !reflect.DeepEqual(got, []string{root.ID, child.ID}) {
		t.Fatalf("related graph ids = %#v, want root,child", got)
	}
}

func TestMemoryStoreProtocol(t *testing.T) {
	runStoreProtocolTest(t, NewMemoryStore())
}

func TestMemoryStoreContentSchema(t *testing.T) {
	runContentSchemaTest(t, NewMemoryStore())
}

func TestMemoryStoreProjections(t *testing.T) {
	runProjectionTest(t, NewMemoryStore())
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

func recordIDs(messages []ioa.MessageRecord) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func containsRecordID(messages []ioa.MessageRecord, want string) bool {
	for _, message := range messages {
		if message.ID == want {
			return true
		}
	}
	return false
}

func hasGraphEdge(graph ioa.GraphView, want ioa.GraphEdge) bool {
	for _, edge := range graph.Edges {
		if edge == want {
			return true
		}
	}
	return false
}
