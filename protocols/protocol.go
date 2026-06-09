package protocols

import (
	"context"
	"encoding/json"
	"sync"
)

type ClientAPI interface {
	NodeID() string
	RegisterNode(ctx context.Context, name, description string, meta map[string]any) (Node, error)
	Space(ctx context.Context, name, description string, tags ...string) (SpaceInfo, error)
	Send(ctx context.Context, spaceID string, body SendMessage) (Message, error)
	Read(ctx context.Context, spaceID string, opts ReadOptions) ([]Message, error)
}

type Env struct {
	Client   ClientAPI
	SpaceID  string
	NodeName string
}

type Handler struct {
	Description string
	Flags       interface{}
	Execute     func(ctx context.Context, env *Env, args interface{}) (string, error)
}

type Protocol struct {
	Name        string
	Description string
	Send        *Handler
	Read        *Handler
}

var (
	mu       sync.RWMutex
	registry []*Protocol
)

func Register(p *Protocol) {
	mu.Lock()
	defer mu.Unlock()
	registry = append(registry, p)
}

func All() []*Protocol {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]*Protocol, len(registry))
	copy(result, registry)
	return result
}

func Get(name string) *Protocol {
	mu.RLock()
	defer mu.RUnlock()
	for _, p := range registry {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func SendHandler(name string) func(ctx context.Context, env *Env, args interface{}) (string, error) {
	p := Get(name)
	if p == nil || p.Send == nil {
		return nil
	}
	return p.Send.Execute
}

func ReadHandler(name string) func(ctx context.Context, env *Env, args interface{}) (string, error) {
	p := Get(name)
	if p == nil || p.Read == nil {
		return nil
	}
	return p.Read.Execute
}

func ParseArgs(args interface{}, dst interface{}) {
	b, _ := json.Marshal(args)
	_ = json.Unmarshal(b, dst)
}
