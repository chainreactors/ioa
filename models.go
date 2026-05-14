package ioa

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
	ID      string                 `json:"id"`
	Sender  string                 `json:"sender"`
	Content map[string]interface{} `json:"content"`
	Refs    Ref                    `json:"refs"`
}

type MessageRecord struct {
	ID      string                 `json:"id"`
	SpaceID string                 `json:"space_id"`
	Sender  string                 `json:"sender"`
	Content map[string]interface{} `json:"content"`
	Refs    Ref                    `json:"refs"`
}

type Space struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SpaceNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SpaceInfo struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Nodes         []SpaceNode            `json:"nodes"`
	MessageCount  int                    `json:"message_count"`
	ContentSchema map[string]interface{} `json:"content_schema,omitempty"`
}

type NodeCreate struct {
	Name string                 `json:"name"`
	Meta map[string]interface{} `json:"meta"`
}

type SpaceCreate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
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

type ErrorResponse struct {
	Detail string `json:"detail"`
}

type SpaceNodeRecord struct {
	Node        Node
	Description string
}

func ExposeMessage(record MessageRecord) Message {
	return Message{
		ID:      record.ID,
		Sender:  record.Sender,
		Content: record.Content,
		Refs:    record.Refs,
	}
}
