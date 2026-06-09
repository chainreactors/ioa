package server

import (
	"github.com/chainreactors/ioa/protocols"
)

type Store interface {
	PutNode(node protocols.Node) error
	GetNode(nodeID string) (protocols.Node, bool, error)
	GetNodeByName(name string) (protocols.Node, bool, error)
	ListNodes() ([]protocols.Node, error)

	PutSpaceIfAbsent(space protocols.Space) (protocols.Space, error)
	GetSpace(spaceID string) (protocols.Space, bool, error)
	ListSpaces() ([]protocols.Space, error)
	JoinSpace(spaceID, nodeID, description string) error
	GetSpaceNodes(spaceID string) ([]protocols.Node, error)

	AppendMessage(message protocols.Message) error
	GetMessage(spaceID, messageID string) (protocols.Message, bool, error)
	GetMessagesForNode(spaceID, nodeID, after string, limit int) ([]protocols.Message, error)
	GetMessageCount(spaceID string) (int, error)
	GetMessages(spaceID, after string, limit int) ([]protocols.Message, error)
	GetStartMessages(spaceID, after string, limit int) ([]protocols.Message, error)
	GetRelatedMessages(spaceID, messageID, direction, after string, limit int) ([]protocols.Message, error)
	GetInboxMessages(nodeID, after string, limit int) ([]protocols.Message, error)
	ListMessages(filter protocols.MessageFilter) ([]protocols.Message, error)

	PutToken(tokenHash string, nodeID string) error
	GetNodeByTokenHash(tokenHash string) (protocols.Node, bool, error)
}

// --- query helpers (shared by all Store implementations) ---

func WindowMessages(messages, allMessages []protocols.Message, after string, limit int) []protocols.Message {
	if after != "" {
		order := make(map[string]int, len(allMessages))
		for i, message := range allMessages {
			order[message.ID] = i
		}
		afterPosition, ok := order[after]
		if !ok {
			messages = nil
		} else {
			filtered := make([]protocols.Message, 0, len(messages))
			for _, message := range messages {
				if order[message.ID] > afterPosition {
					filtered = append(filtered, message)
				}
			}
			messages = filtered
		}
	}
	if limit > 0 {
		if after == "" {
			if len(messages) > limit {
				messages = messages[len(messages)-limit:]
			}
		} else if len(messages) > limit {
			messages = messages[:limit]
		}
	}
	return messages
}

func RelatedMessages(allMessages []protocols.Message, messageID, direction, after string, limit int) []protocols.Message {
	index := make(map[string]protocols.Message, len(allMessages))
	children := make(map[string][]string)
	for _, message := range allMessages {
		index[message.ID] = message
		for _, parentID := range message.Refs.Messages {
			children[parentID] = append(children[parentID], message.ID)
		}
	}

	related := make(map[string]struct{})
	if _, ok := index[messageID]; ok {
		related[messageID] = struct{}{}
	}

	if direction != "downstream" {
		stack := []string{messageID}
		for len(stack) > 0 {
			mid := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, seen := related[mid]; seen {
				continue
			}
			message, ok := index[mid]
			if !ok {
				continue
			}
			related[mid] = struct{}{}
			stack = append(stack, message.Refs.Messages...)
		}
	}

	if direction != "upstream" {
		stack := []string{messageID}
		for len(stack) > 0 {
			mid := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, childID := range children[mid] {
				if _, seen := related[childID]; seen {
					continue
				}
				related[childID] = struct{}{}
				stack = append(stack, childID)
			}
		}
	}

	messages := make([]protocols.Message, 0, len(related))
	for _, message := range allMessages {
		if _, ok := related[message.ID]; ok {
			messages = append(messages, message)
		}
	}
	return WindowMessages(messages, allMessages, after, limit)
}

func ContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func FilterMessages(messages []protocols.Message, filter protocols.MessageFilter) []protocols.Message {
	filtered := make([]protocols.Message, 0, len(messages))
	for _, message := range messages {
		if filter.SpaceID != "" && message.SpaceID != filter.SpaceID {
			continue
		}
		if filter.MessageID != "" && message.ID != filter.MessageID {
			continue
		}
		if filter.Sender != "" && message.Sender != filter.Sender {
			continue
		}
		if filter.NodeID != "" && message.Sender != filter.NodeID && !ContainsString(message.Refs.Nodes, filter.NodeID) {
			continue
		}
		if filter.RefMessage != "" && !ContainsString(message.Refs.Messages, filter.RefMessage) {
			continue
		}
		if filter.RefNode != "" && !ContainsString(message.Refs.Nodes, filter.RefNode) {
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered
}
