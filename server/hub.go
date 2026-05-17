package server

import (
	"sync"

	"github.com/chainreactors/ioa"
)

type Hub struct {
	mu              sync.Mutex
	subscribers     map[string]map[chan ioa.Message]struct{}
	nodeSubscribers map[string]map[chan ioa.Message]struct{}
}

func NewHub() *Hub {
	return &Hub{
		subscribers:     make(map[string]map[chan ioa.Message]struct{}),
		nodeSubscribers: make(map[string]map[chan ioa.Message]struct{}),
	}
}

func (h *Hub) Subscribe(spaceID string) (<-chan ioa.Message, func()) {
	return h.subscribe(h.subscribers, spaceID)
}

func (h *Hub) Broadcast(spaceID string, message ioa.Message) {
	h.broadcast(h.subscribers, spaceID, message)
}

func (h *Hub) SubscribeNode(nodeID string) (<-chan ioa.Message, func()) {
	return h.subscribe(h.nodeSubscribers, nodeID)
}

func (h *Hub) BroadcastToNode(nodeID string, message ioa.Message) {
	h.broadcast(h.nodeSubscribers, nodeID, message)
}

func (h *Hub) subscribe(buckets map[string]map[chan ioa.Message]struct{}, key string) (<-chan ioa.Message, func()) {
	ch := make(chan ioa.Message, 16)
	h.mu.Lock()
	if _, ok := buckets[key]; !ok {
		buckets[key] = make(map[chan ioa.Message]struct{})
	}
	buckets[key][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if bucket, ok := buckets[key]; ok {
			delete(bucket, ch)
			if len(bucket) == 0 {
				delete(buckets, key)
			}
		}
		close(ch)
		h.mu.Unlock()
	}
}

func (h *Hub) broadcast(buckets map[string]map[chan ioa.Message]struct{}, key string, message ioa.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range buckets[key] {
		select {
		case ch <- message:
		default:
		}
	}
}
