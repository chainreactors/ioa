package client

import (
	"context"

	"github.com/chainreactors/ioa/protocols"
)

type StreamAPI interface {
	protocols.ClientAPI
	Subscribe(ctx context.Context, spaceID string, opts ...SubscribeOption) (<-chan protocols.Message, <-chan error, func(), error)
}

type subscribeConfig struct {
	Head      string
	ForkDepth int
}

type SubscribeOption func(*subscribeConfig)

func WithHead(messageID string) SubscribeOption {
	return func(c *subscribeConfig) { c.Head = messageID }
}

func WithForkDepth(depth int) SubscribeOption {
	return func(c *subscribeConfig) { c.ForkDepth = depth }
}
