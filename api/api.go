package api

import "github.com/chainreactors/ioa/protocols"

type NodeCreate struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta"`
}

type SpaceCreate struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
}

type MessageFilter struct {
	SpaceID    string
	MessageID  string
	NodeID     string
	Sender     string
	RefMessage string
	RefNode    string
	After      string
	Limit      int
}

type GraphOptions struct {
	MessageFilter
	Include []string
}

type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

type GraphStats struct {
	SpaceCount   int `json:"space_count"`
	NodeCount    int `json:"node_count"`
	MessageCount int `json:"message_count"`
	EdgeCount    int `json:"edge_count"`
}

type GraphView struct {
	Spaces   []protocols.SpaceInfo `json:"spaces"`
	Nodes    []protocols.Node      `json:"nodes"`
	Messages []protocols.Message   `json:"messages"`
	Edges    []GraphEdge           `json:"edges"`
	Stats    GraphStats            `json:"stats"`
}

type AuthRegister struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	AccessKey   string         `json:"access_key"`
	Meta        map[string]any `json:"meta"`
}

type AuthResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

type ErrorResponse struct {
	Detail string `json:"detail"`
}
