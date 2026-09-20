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
	MessageID string
}

// SubscriptionOptions resolves the options shared by HTTP and in-process clients.
func SubscriptionOptions(opts ...SubscribeOption) (head, messageID string, forkDepth int) {
	var cfg subscribeConfig
	for _, option := range opts {
		option(&cfg)
	}
	if cfg.ForkDepth <= 0 {
		cfg.ForkDepth = 1
	}
	return cfg.Head, cfg.MessageID, cfg.ForkDepth
}

type SubscribeOption func(*subscribeConfig)

func WithHead(messageID string) SubscribeOption {
	return func(c *subscribeConfig) { c.Head = messageID }
}

func WithForkDepth(depth int) SubscribeOption {
	return func(c *subscribeConfig) { c.ForkDepth = depth }
}

func WithMessage(messageID string) SubscribeOption {
	return func(c *subscribeConfig) { c.MessageID = messageID }
}
