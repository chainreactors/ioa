package protocols

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"

	"github.com/chainreactors/ioa"
)

type ClientAPI interface {
	NodeID() string
	RegisterNode(ctx context.Context, name, description string, meta map[string]interface{}) (ioa.Node, error)
	Space(ctx context.Context, name, description string, tags ...string) (ioa.SpaceInfo, error)
	Send(ctx context.Context, spaceID string, body ioa.SendMessage) (ioa.Message, error)
	Read(ctx context.Context, spaceID string, opts ioa.ReadOptions) ([]ioa.Message, error)
}

type Env struct {
	Client   ClientAPI
	SpaceID  string
	NodeName string
	Data     interface{}
}

func FlagsFrom[T any](env *Env) *T {
	if v, ok := env.Data.(*T); ok {
		return v
	}
	var zero T
	return &zero
}

type SubcommandDef struct {
	Name        string
	Description string
	Data        interface{}
	Execute     func(ctx context.Context, env *Env) (string, error)
}

type Protocol struct {
	Name        string
	Description string
	Send        []SubcommandDef
	Read        []SubcommandDef
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

func SendHandler(subcommand string) func(ctx context.Context, env *Env, args map[string]interface{}) (string, error) {
	mu.RLock()
	defer mu.RUnlock()
	for _, p := range registry {
		for i := range p.Send {
			if p.Send[i].Name == subcommand {
				def := &p.Send[i]
				return func(ctx context.Context, env *Env, args map[string]interface{}) (string, error) {
					data := newFlags(def.Data)
					populateFlags(data, args)
					env.Data = data
					return def.Execute(ctx, env)
				}
			}
		}
	}
	return nil
}

func ReadHandler(subcommand string) func(ctx context.Context, env *Env, args map[string]interface{}) (string, error) {
	mu.RLock()
	defer mu.RUnlock()
	for _, p := range registry {
		for i := range p.Read {
			if p.Read[i].Name == subcommand {
				def := &p.Read[i]
				return func(ctx context.Context, env *Env, args map[string]interface{}) (string, error) {
					data := newFlags(def.Data)
					populateFlags(data, args)
					env.Data = data
					return def.Execute(ctx, env)
				}
			}
		}
	}
	return nil
}

func newFlags(proto interface{}) interface{} {
	return reflect.New(reflect.TypeOf(proto).Elem()).Interface()
}

func populateFlags(data interface{}, args map[string]interface{}) {
	b, _ := json.Marshal(args)
	_ = json.Unmarshal(b, data)
}
