package api

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
