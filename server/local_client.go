package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/chainreactors/ioa/client"
	"github.com/chainreactors/ioa/protocols"
)

// LocalClient borrows a Service and speaks the same protocol as the HTTP client.
// The caller owns the service/store and must cancel subscriptions before closing it.
type LocalClient struct {
	service *Service
	mu      sync.RWMutex
	nodeID  string
	bound   bool
}

func NewLocalClient(service *Service, nodeID string) *LocalClient {
	if nodeID == "" {
		nodeID = protocols.NewID()
	}
	return &LocalClient{service: service, nodeID: nodeID}
}
func (c *LocalClient) NodeID() string { c.mu.RLock(); defer c.mu.RUnlock(); return c.nodeID }
func (c *LocalClient) Bound() bool    { c.mu.RLock(); defer c.mu.RUnlock(); return c.bound }
func (c *LocalClient) RegisterNode(ctx context.Context, name, description string, meta map[string]any) (protocols.Node, error) {
	if err := ctx.Err(); err != nil {
		return protocols.Node{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	node, err := c.service.RegisterNode(ctx, protocols.NodeCreate{ID: c.nodeID, Name: name, Description: description, Meta: meta})
	if err == nil {
		c.nodeID, c.bound = node.ID, true
	}
	return node, err
}
func (c *LocalClient) EnsureRegistered(ctx context.Context, name, description string, meta map[string]any) error {
	if c.Bound() {
		return ctx.Err()
	}
	_, err := c.RegisterNode(ctx, name, description, meta)
	return err
}
func (c *LocalClient) Space(ctx context.Context, name, description string, tags ...string) (protocols.SpaceInfo, error) {
	if err := ctx.Err(); err != nil {
		return protocols.SpaceInfo{}, err
	}
	return c.service.CreateSpace(ctx, c.NodeID(), protocols.SpaceCreate{Name: name, Description: description, Tags: tags})
}
func (c *LocalClient) Send(ctx context.Context, space string, body protocols.SendMessage) (protocols.Message, error) {
	if err := ctx.Err(); err != nil {
		return protocols.Message{}, err
	}
	return c.service.SendMessage(ctx, space, c.NodeID(), body)
}
func (c *LocalClient) Read(ctx context.Context, space string, opts protocols.ReadOptions) ([]protocols.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.service.ReadMessages(ctx, space, c.NodeID(), opts)
}
func (c *LocalClient) ReadPublic(ctx context.Context, space string, opts protocols.ReadOptions) ([]protocols.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.service.ReadMessages(ctx, space, "", opts)
}
func (c *LocalClient) ListSpaces(ctx context.Context) ([]protocols.SpaceInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.service.ListSpaces(ctx)
}
func (c *LocalClient) ListNodes(ctx context.Context) ([]protocols.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.service.ListNodes(ctx)
}
func (c *LocalClient) GetSpaceInfo(ctx context.Context, id string) (protocols.SpaceInfo, error) {
	if err := ctx.Err(); err != nil {
		return protocols.SpaceInfo{}, err
	}
	return c.service.GetSpace(ctx, id)
}
func (c *LocalClient) ResolveSpace(ctx context.Context, name string) (protocols.SpaceInfo, error) {
	info, err := c.GetSpaceInfo(ctx, name)
	if err == nil {
		return info, nil
	}
	if pe, ok := err.(*protocols.Error); !ok || pe.Status != http.StatusNotFound {
		return protocols.SpaceInfo{}, err
	}
	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		return protocols.SpaceInfo{}, err
	}
	for _, space := range spaces {
		if space.Name == name {
			return space, nil
		}
	}
	return protocols.SpaceInfo{}, protocols.ProtocolError(http.StatusNotFound, "space %q not found", name)
}

func (c *LocalClient) Subscribe(ctx context.Context, space string, opts ...client.SubscribeOption) (<-chan protocols.Message, <-chan error, func(), error) {
	if _, err := c.GetSpaceInfo(ctx, space); err != nil {
		return nil, nil, nil, err
	}
	head, messageID, depth := client.SubscriptionOptions(opts...)
	if messageID != "" {
		if _, err := c.ReadPublic(ctx, space, protocols.ReadOptions{MessageID: messageID}); err != nil {
			return nil, nil, nil, err
		}
	}
	var tracker *HeadTracker
	if head != "" {
		if _, ok, err := c.service.Store().GetMessage(space, head); err != nil || !ok {
			return nil, nil, nil, fmt.Errorf("head message not found: %s", head)
		}
		var err error
		tracker, err = NewHeadTracker(c.service.Store(), space, head, depth)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	call, cancel := context.WithCancel(ctx)
	feed, unsubscribe := c.service.Hub().Subscribe(space)
	out, errs, done := make(chan protocols.Message), make(chan error, 1), make(chan struct{})
	go func() {
		defer close(done)
		defer close(out)
		defer close(errs)
		defer unsubscribe()
		seen := map[string]bool{}
		deliver := func(msg protocols.Message) bool {
			if tracker != nil {
				accepted, fork := tracker.Accept(msg)
				if !accepted {
					return true
				}
				if fork {
					msg.ContentType = "ioa/fork"
				}
			} else if messageID != "" {
				ok, err := c.service.IsRelated(call, space, messageID, msg.ID)
				if err != nil {
					errs <- err
					return false
				}
				if !ok {
					return true
				}
			}
			select {
			case out <- msg:
				return true
			case <-call.Done():
				return false
			}
		}
		if tracker != nil {
			history, err := c.ReadPublic(call, space, protocols.ReadOptions{All: true, After: head})
			if err != nil {
				errs <- err
				return
			}
			for _, msg := range history {
				seen[msg.ID] = true
				if !deliver(msg) {
					return
				}
			}
		}
		for {
			select {
			case <-call.Done():
				return
			case msg, ok := <-feed:
				if !ok {
					return
				}
				if seen[msg.ID] {
					delete(seen, msg.ID)
					continue
				}
				if !deliver(msg) {
					return
				}
			}
		}
	}()
	return out, errs, func() { cancel(); <-done }, nil
}

var _ client.StreamAPI = (*LocalClient)(nil)
