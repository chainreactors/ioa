package protocols

import "strings"

type Ref struct {
	Messages []string `json:"messages"`
	Nodes    []string `json:"nodes"`
}

type Node struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta"`
}

type Space struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags,omitempty"`
}

type SpaceInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Tags         []string `json:"tags,omitempty"`
	Nodes        []Node   `json:"nodes"`
	MessageCount int      `json:"message_count"`
}

type Message struct {
	ID            string         `json:"id"`
	SpaceID       string         `json:"space_id,omitempty"`
	Sender        string         `json:"sender"`
	CreatedAt     string         `json:"created_at"`
	ContentType   string         `json:"content_type,omitempty"`
	Content       map[string]any `json:"content"`
	Refs          Ref            `json:"refs"`
	Meta          map[string]any `json:"meta,omitempty"`
	ContentSchema map[string]any `json:"content_schema,omitempty"`
}

type SendMessage struct {
	ContentType   string         `json:"content_type,omitempty"`
	Content       map[string]any `json:"content"`
	Refs          *Ref           `json:"refs,omitempty"`
	Meta          map[string]any `json:"meta,omitempty"`
	ContentSchema map[string]any `json:"content_schema,omitempty"`
}

type ReadOptions struct {
	MessageID string
	Direction string // "upstream", "downstream", or "" (both)
	After     string
	Limit     int
	All       bool
}

func MessageContentType(msg Message) string {
	if msg.ContentType != "" {
		return msg.ContentType
	}
	if t, ok := msg.Content["type"].(string); ok {
		return t
	}
	return ""
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

// --- request/response types (formerly api package) ---

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
