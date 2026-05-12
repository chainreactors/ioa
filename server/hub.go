package server

import (
	"sync"

	"github.com/chainreactors/ioa"
)

type Hub struct {
	mu          sync.Mutex
	subscribers map[string]map[chan ioa.Message]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[string]map[chan ioa.Message]struct{})}
}

func (h *Hub) Subscribe(spaceID string) (<-chan ioa.Message, func()) {
	ch := make(chan ioa.Message, 16)
	h.mu.Lock()
	if _, ok := h.subscribers[spaceID]; !ok {
		h.subscribers[spaceID] = make(map[chan ioa.Message]struct{})
	}
	h.subscribers[spaceID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if bucket, ok := h.subscribers[spaceID]; ok {
			delete(bucket, ch)
			if len(bucket) == 0 {
				delete(h.subscribers, spaceID)
			}
		}
		close(ch)
		h.mu.Unlock()
	}
}

func (h *Hub) Broadcast(spaceID string, message ioa.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subscribers[spaceID] {
		select {
		case ch <- message:
		default:
		}
	}
}
