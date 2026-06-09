package server

import (
	"slices"

	"github.com/chainreactors/ioa/protocols"
)

// HeadTracker implements branch-aware filtering for SSE connections.
// It tracks a HEAD position in the message graph and classifies incoming
// messages as branch extensions, forks, or unrelated.
//
// This is an SSE-layer observation mechanism, not a protocol concept.
// The underlying message graph (messages + refs) remains unchanged.
type HeadTracker struct {
	store     Store
	spaceID   string
	head      string
	forkDepth int
	ancestors []string // head's ancestor chain up to forkDepth
}

func NewHeadTracker(store Store, spaceID, head string, forkDepth int) (*HeadTracker, error) {
	if forkDepth <= 0 {
		forkDepth = 1
	}
	ht := &HeadTracker{
		store:     store,
		spaceID:   spaceID,
		head:      head,
		forkDepth: forkDepth,
	}
	if err := ht.recomputeAncestors(); err != nil {
		return nil, err
	}
	return ht, nil
}

// Accept classifies an incoming message relative to the current HEAD.
//   - deliver=true, fork=false: message extends the branch (refs HEAD)
//   - deliver=true, fork=true:  message forks from an ancestor within depth
//   - deliver=false:            unrelated message
func (ht *HeadTracker) Accept(msg protocols.Message) (deliver, fork bool) {
	if len(msg.Refs.Messages) == 0 {
		return false, false
	}
	if slices.Contains(msg.Refs.Messages, ht.head) {
		ht.head = msg.ID
		_ = ht.recomputeAncestors()
		return true, false
	}
	for _, ancestor := range ht.ancestors {
		if slices.Contains(msg.Refs.Messages, ancestor) {
			return true, true
		}
	}
	return false, false
}

func (ht *HeadTracker) Head() string {
	return ht.head
}

func (ht *HeadTracker) recomputeAncestors() error {
	ht.ancestors = ht.ancestors[:0]
	current := ht.head
	for i := 0; i < ht.forkDepth; i++ {
		rec, ok, err := ht.store.GetMessage(ht.spaceID, current)
		if err != nil || !ok {
			break
		}
		if len(rec.Refs.Messages) == 0 {
			break
		}
		parent := rec.Refs.Messages[0]
		ht.ancestors = append(ht.ancestors, parent)
		current = parent
	}
	return nil
}
