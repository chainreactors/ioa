package ioa

import (
	"strings"
)

type Ref struct {
	Messages []string `json:"messages"`
	Nodes    []string `json:"nodes"`
}

type Node struct {
	ID   string                 `json:"id"`
	Name string                 `json:"name"`
	Meta map[string]interface{} `json:"meta"`
}

type Message struct {
	ID        string                 `json:"id"`
	Sender    string                 `json:"sender"`
	CreatedAt string                 `json:"created_at"`
	Content   map[string]interface{} `json:"content"`
	Refs      Ref                    `json:"refs"`
}

type MessageRecord struct {
	ID        string                 `json:"id"`
	SpaceID   string                 `json:"space_id"`
	Sender    string                 `json:"sender"`
	CreatedAt string                 `json:"created_at"`
	Content   map[string]interface{} `json:"content"`
	Refs      Ref                    `json:"refs"`
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
	Spaces   []SpaceInfo     `json:"spaces"`
	Nodes    []Node          `json:"nodes"`
	Messages []MessageRecord `json:"messages"`
	Edges    []GraphEdge     `json:"edges"`
	Stats    GraphStats      `json:"stats"`
}

type Space struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags,omitempty"`
}

type SpaceNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SpaceInfo struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Tags          []string               `json:"tags,omitempty"`
	Nodes         []SpaceNode            `json:"nodes"`
	MessageCount  int                    `json:"message_count"`
	ContentSchema map[string]interface{} `json:"content_schema,omitempty"`
}

type NodeCreate struct {
	Name string                 `json:"name"`
	Meta map[string]interface{} `json:"meta"`
}

type SpaceCreate struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
}

type SendMessage struct {
	Content       map[string]interface{} `json:"content"`
	Refs          *Ref                   `json:"refs,omitempty"`
	ContentSchema map[string]interface{} `json:"content_schema,omitempty"`
}

type ReadOptions struct {
	MessageID string
	After     string
	Limit     int
	All       bool
}

type AuthRegister struct {
	Name      string                 `json:"name"`
	AccessKey string                 `json:"access_key"`
	Meta      map[string]interface{} `json:"meta"`
}

type AuthResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

type ErrorResponse struct {
	Detail string `json:"detail"`
}

type SpaceNodeRecord struct {
	Node        Node
	Description string
}

func ExposeMessage(record MessageRecord) Message {
	return Message{
		ID:        record.ID,
		Sender:    record.Sender,
		CreatedAt: record.CreatedAt,
		Content:   record.Content,
		Refs:      record.Refs,
	}
}

func NormalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func MergeTags(existing []string, incoming []string) []string {
	return NormalizeTags(append(append([]string{}, existing...), incoming...))
}
