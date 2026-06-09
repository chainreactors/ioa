package server

import (
	"testing"

	"github.com/chainreactors/ioa/protocols"
)

func seedMessages(store Store, spaceID string, msgs ...protocols.Message) {
	for _, m := range msgs {
		_ = store.AppendMessage(m)
	}
}

func msg(id, spaceID string, parents ...string) protocols.Message {
	return protocols.Message{
		ID:      id,
		SpaceID: spaceID,
		Sender:  "test",
		Refs:    protocols.Ref{Messages: parents},
		Content: map[string]interface{}{"text": id},
	}
}

func TestHeadTracker_LinearThread(t *testing.T) {
	// A → B → C, head=C
	// New message D (parent=C) should extend branch
	// New message E (parent=A) should be unrelated (beyond depth 1)
	store := NewMemoryStore()
	space := protocols.Space{ID: "s1", Name: "test"}
	store.PutSpaceIfAbsent(space)
	seedMessages(store, "s1",
		msg("A", "s1"),
		msg("B", "s1", "A"),
		msg("C", "s1", "B"),
	)

	ht, err := NewHeadTracker(store, "s1", "C", 1)
	if err != nil {
		t.Fatal(err)
	}
	if ht.Head() != "C" {
		t.Fatalf("head = %q, want C", ht.Head())
	}

	// D extends C
	deliver, fork := ht.Accept(protocols.Message{ID: "D", Refs: protocols.Ref{Messages: []string{"C"}}})
	if !deliver || fork {
		t.Errorf("D: deliver=%v fork=%v, want deliver=true fork=false", deliver, fork)
	}
	if ht.Head() != "D" {
		t.Fatalf("after D: head = %q, want D", ht.Head())
	}

	// E references A (2 levels back, beyond depth=1)
	deliver, fork = ht.Accept(protocols.Message{ID: "E", Refs: protocols.Ref{Messages: []string{"A"}}})
	if deliver {
		t.Errorf("E: deliver=%v, want false (beyond depth)", deliver)
	}
}

func TestHeadTracker_Fork(t *testing.T) {
	// A → B → C, head=C
	// D (parent=B) should be detected as fork (depth=1 from C)
	store := NewMemoryStore()
	space := protocols.Space{ID: "s1", Name: "test"}
	store.PutSpaceIfAbsent(space)
	seedMessages(store, "s1",
		msg("A", "s1"),
		msg("B", "s1", "A"),
		msg("C", "s1", "B"),
	)

	ht, err := NewHeadTracker(store, "s1", "C", 1)
	if err != nil {
		t.Fatal(err)
	}

	// D forks from B (parent of C, within depth=1)
	deliver, fork := ht.Accept(protocols.Message{ID: "D", Refs: protocols.Ref{Messages: []string{"B"}}})
	if !deliver || !fork {
		t.Errorf("D: deliver=%v fork=%v, want deliver=true fork=true", deliver, fork)
	}
}

func TestHeadTracker_ForkDepth2(t *testing.T) {
	// A → B → C → D, head=D, forkDepth=2
	// E (parent=B) should be fork (within depth 2)
	// F (parent=A) should be unrelated (depth 3, beyond 2)
	store := NewMemoryStore()
	space := protocols.Space{ID: "s1", Name: "test"}
	store.PutSpaceIfAbsent(space)
	seedMessages(store, "s1",
		msg("A", "s1"),
		msg("B", "s1", "A"),
		msg("C", "s1", "B"),
		msg("D", "s1", "C"),
	)

	ht, err := NewHeadTracker(store, "s1", "D", 2)
	if err != nil {
		t.Fatal(err)
	}

	deliver, fork := ht.Accept(protocols.Message{ID: "E", Refs: protocols.Ref{Messages: []string{"B"}}})
	if !deliver || !fork {
		t.Errorf("E: deliver=%v fork=%v, want deliver=true fork=true", deliver, fork)
	}

	deliver, fork = ht.Accept(protocols.Message{ID: "F", Refs: protocols.Ref{Messages: []string{"A"}}})
	if deliver {
		t.Errorf("F: deliver=%v, want false (beyond depth=2)", deliver)
	}
}

func TestHeadTracker_NoRefs(t *testing.T) {
	store := NewMemoryStore()
	space := protocols.Space{ID: "s1", Name: "test"}
	store.PutSpaceIfAbsent(space)
	seedMessages(store, "s1", msg("A", "s1"))

	ht, err := NewHeadTracker(store, "s1", "A", 1)
	if err != nil {
		t.Fatal(err)
	}

	deliver, _ := ht.Accept(protocols.Message{ID: "B"})
	if deliver {
		t.Errorf("B (no refs): deliver=%v, want false", deliver)
	}
}

func TestHeadTracker_DAGMerge(t *testing.T) {
	// A → B, A → C, head=B
	// D (parents=[B, C]) should extend branch (one parent is HEAD)
	store := NewMemoryStore()
	space := protocols.Space{ID: "s1", Name: "test"}
	store.PutSpaceIfAbsent(space)
	seedMessages(store, "s1",
		msg("A", "s1"),
		msg("B", "s1", "A"),
		msg("C", "s1", "A"),
	)

	ht, err := NewHeadTracker(store, "s1", "B", 1)
	if err != nil {
		t.Fatal(err)
	}

	deliver, fork := ht.Accept(protocols.Message{ID: "D", Refs: protocols.Ref{Messages: []string{"B", "C"}}})
	if !deliver || fork {
		t.Errorf("D (DAG merge): deliver=%v fork=%v, want deliver=true fork=false", deliver, fork)
	}
	if ht.Head() != "D" {
		t.Fatalf("after D: head = %q, want D", ht.Head())
	}
}
