package swarm

import (
	"encoding/json"

	"github.com/chainreactors/ioa/skills"
	"github.com/chainreactors/ioa/protocols"
)

type SwarmMessage struct {
	Content string         `json:"content"`
	Targets []string       `json:"targets,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

func SwarmSchema() map[string]any {
	s, err := skills.ReadSchema("swarm")
	if err != nil {
		return map[string]any{
			"type":                 "object",
			"properties":          map[string]any{"content": map[string]any{"type": "string"}},
			"required":            []string{"content"},
			"additionalProperties": true,
		}
	}
	return s
}

func ParseSwarm(content map[string]any) (SwarmMessage, bool) {
	c, ok := content["content"].(string)
	if !ok || c == "" {
		return SwarmMessage{}, false
	}
	msg := SwarmMessage{Content: c}
	if raw, ok := content["targets"]; ok {
		if data, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(data, &msg.Targets)
		}
	}
	if raw, ok := content["meta"].(map[string]any); ok {
		msg.Meta = raw
	}
	for k, v := range content {
		if k == "content" || k == "targets" || k == "meta" {
			continue
		}
		if msg.Meta == nil {
			msg.Meta = make(map[string]any)
		}
		msg.Meta[k] = v
	}
	return msg, true
}

func ParseLegacyMessage(content map[string]any) (SwarmMessage, bool) {
	if task, ok := content["task"].(string); ok && task != "" {
		return SwarmMessage{Content: task}, true
	}
	if prompt, ok := content["prompt"].(string); ok && prompt != "" {
		return SwarmMessage{Content: prompt}, true
	}
	return SwarmMessage{}, false
}

func SwarmContent(msg SwarmMessage) map[string]any {
	m := map[string]any{"content": msg.Content}
	if len(msg.Targets) > 0 {
		m["targets"] = msg.Targets
	}
	if len(msg.Meta) > 0 {
		m["meta"] = msg.Meta
	}
	return m
}

func SwarmFromIOA(msg protocols.Message) (SwarmMessage, bool) {
	if sm, ok := ParseSwarm(msg.Content); ok {
		return sm, true
	}
	return ParseLegacyMessage(msg.Content)
}

func MessageKind(msg protocols.Message, sm SwarmMessage) string {
	if kind, _ := sm.Meta["kind"].(string); kind != "" {
		return kind
	}
	if kind, _ := msg.Content["kind"].(string); kind != "" {
		return kind
	}
	if meta, ok := msg.Content["meta"].(map[string]any); ok {
		kind, _ := meta["kind"].(string)
		return kind
	}
	return ""
}

