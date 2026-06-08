package server

import (
	"slices"
	"sort"
	"sync"

	"github.com/chainreactors/ioa"
)

type MemoryStore struct {
	mu           sync.RWMutex
	nodes        map[string]ioa.Node
	spaces       map[string]ioa.Space
	spaceNames   map[string]string
	messages     map[string][]ioa.MessageRecord
	spaceNodes   map[string]map[string]string
	messageSchemas map[string]map[string]interface{}
	tokens       map[string]string // sha256(token) → nodeID
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nodes:        make(map[string]ioa.Node),
		spaces:       make(map[string]ioa.Space),
		spaceNames:   make(map[string]string),
		messages:     make(map[string][]ioa.MessageRecord),
		spaceNodes:   make(map[string]map[string]string),
		messageSchemas: make(map[string]map[string]interface{}),
		tokens:       make(map[string]string),
	}
}

func (s *MemoryStore) PutNode(node ioa.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node
	return nil
}

func (s *MemoryStore) GetNode(nodeID string) (ioa.Node, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, ok := s.nodes[nodeID]
	return node, ok, nil
}

func (s *MemoryStore) GetNodeByName(name string) (ioa.Node, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, node := range s.nodes {
		if node.Name == name {
			return node, true, nil
		}
	}
	return ioa.Node{}, false, nil
}

func (s *MemoryStore) ListNodes() ([]ioa.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ioa.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		result = append(result, node)
	}
	return result, nil
}

func (s *MemoryStore) ListSpaces() ([]ioa.Space, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ioa.Space, 0, len(s.spaces))
	for _, space := range s.spaces {
		result = append(result, space)
	}
	return result, nil
}

func (s *MemoryStore) PutSpaceIfAbsent(space ioa.Space) (ioa.Space, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.spaceNames[space.Name]; ok {
		if existing, ok := s.spaces[existingID]; ok {
			mergedTags := ioa.MergeTags(existing.Tags, space.Tags)
			if !slices.Equal(mergedTags, existing.Tags) {
				existing.Tags = mergedTags
				s.spaces[existingID] = existing
			}
			return existing, nil
		}
	}
	space.Tags = ioa.NormalizeTags(space.Tags)
	s.spaces[space.ID] = space
	s.spaceNames[space.Name] = space.ID
	if _, ok := s.messages[space.ID]; !ok {
		s.messages[space.ID] = []ioa.MessageRecord{}
	}
	if _, ok := s.spaceNodes[space.ID]; !ok {
		s.spaceNodes[space.ID] = make(map[string]string)
	}
	return space, nil
}

func (s *MemoryStore) GetSpace(spaceID string) (ioa.Space, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	space, ok := s.spaces[spaceID]
	return space, ok, nil
}

func (s *MemoryStore) JoinSpace(spaceID, nodeID, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.spaceNodes[spaceID]; !ok {
		s.spaceNodes[spaceID] = make(map[string]string)
	}
	s.spaceNodes[spaceID][nodeID] = description
	return nil
}

func (s *MemoryStore) GetSpaceNodes(spaceID string) ([]ioa.SpaceNodeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	members := s.spaceNodes[spaceID]
	result := make([]ioa.SpaceNodeRecord, 0, len(members))
	for nodeID, description := range members {
		node, ok := s.nodes[nodeID]
		if !ok {
			continue
		}
		result = append(result, ioa.SpaceNodeRecord{Node: node, Description: description})
	}
	return result, nil
}

func (s *MemoryStore) SetContentSchema(spaceID, rootMessageID string, schema map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := spaceID + "\x00" + rootMessageID
	if schema == nil {
		delete(s.messageSchemas, key)
	} else {
		s.messageSchemas[key] = schema
	}
	return nil
}

func (s *MemoryStore) GetContentSchema(spaceID, rootMessageID string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.messageSchemas[spaceID+"\x00"+rootMessageID], nil
}

func (s *MemoryStore) AppendMessage(message ioa.MessageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[message.SpaceID] = append(s.messages[message.SpaceID], message)
	return nil
}

func (s *MemoryStore) GetMessage(spaceID, messageID string) (ioa.MessageRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, message := range s.messages[spaceID] {
		if message.ID == messageID {
			return message, true, nil
		}
	}
	return ioa.MessageRecord{}, false, nil
}

func (s *MemoryStore) GetMessagesForNode(spaceID, nodeID, after string, limit int) ([]ioa.MessageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := cloneMessages(s.messages[spaceID])
	messages := make([]ioa.MessageRecord, 0, len(all))
	for _, message := range all {
		if ContainsString(message.Refs.Nodes, nodeID) {
			messages = append(messages, message)
		}
	}
	return WindowMessages(messages, all, after, limit), nil
}

func (s *MemoryStore) GetMessageCount(spaceID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages[spaceID]), nil
}

func (s *MemoryStore) GetMessages(spaceID, after string, limit int) ([]ioa.MessageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := cloneMessages(s.messages[spaceID])
	return WindowMessages(all, all, after, limit), nil
}

func (s *MemoryStore) GetStartMessages(spaceID, after string, limit int) ([]ioa.MessageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := cloneMessages(s.messages[spaceID])
	messages := make([]ioa.MessageRecord, 0, len(all))
	for _, message := range all {
		if len(message.Refs.Messages) == 0 && len(message.Refs.Nodes) == 0 {
			messages = append(messages, message)
		}
	}
	return WindowMessages(messages, all, after, limit), nil
}

func (s *MemoryStore) GetRelatedMessages(spaceID, messageID, after string, limit int) ([]ioa.MessageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := cloneMessages(s.messages[spaceID])
	return RelatedMessages(all, messageID, after, limit), nil
}

func (s *MemoryStore) GetInboxMessages(nodeID, after string, limit int) ([]ioa.MessageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spaceIDs := make([]string, 0)
	for spaceID, members := range s.spaceNodes {
		if _, ok := members[nodeID]; ok {
			spaceIDs = append(spaceIDs, spaceID)
		}
	}
	sort.Strings(spaceIDs)
	var allMessages []ioa.MessageRecord
	for _, spaceID := range spaceIDs {
		allMessages = append(allMessages, cloneMessages(s.messages[spaceID])...)
	}
	filtered := make([]ioa.MessageRecord, 0, len(allMessages))
	for _, record := range allMessages {
		if ContainsString(record.Refs.Nodes, nodeID) {
			filtered = append(filtered, record)
		}
	}
	return WindowMessages(filtered, allMessages, after, limit), nil
}

func (s *MemoryStore) ListMessages(filter ioa.MessageFilter) ([]ioa.MessageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.messagesInScope(filter.SpaceID)
	filtered := FilterMessages(all, filter)
	return WindowMessages(filtered, all, filter.After, filter.Limit), nil
}

func (s *MemoryStore) messagesInScope(spaceID string) []ioa.MessageRecord {
	if spaceID != "" {
		return cloneMessages(s.messages[spaceID])
	}
	spaceIDs := make([]string, 0, len(s.messages))
	for id := range s.messages {
		spaceIDs = append(spaceIDs, id)
	}
	sort.Strings(spaceIDs)
	var messages []ioa.MessageRecord
	for _, id := range spaceIDs {
		messages = append(messages, cloneMessages(s.messages[id])...)
	}
	return messages
}

func (s *MemoryStore) PutToken(tokenHash string, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[tokenHash] = nodeID
	return nil
}

func (s *MemoryStore) GetNodeByTokenHash(tokenHash string) (ioa.Node, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodeID, ok := s.tokens[tokenHash]
	if !ok {
		return ioa.Node{}, false, nil
	}
	node, ok := s.nodes[nodeID]
	return node, ok, nil
}

func cloneMessages(messages []ioa.MessageRecord) []ioa.MessageRecord {
	cloned := make([]ioa.MessageRecord, len(messages))
	for i, message := range messages {
		cloned[i] = message
	}
	return cloned
}

func WindowMessages(messages, allMessages []ioa.MessageRecord, after string, limit int) []ioa.MessageRecord {
	if after != "" {
		order := make(map[string]int, len(allMessages))
		for i, message := range allMessages {
			order[message.ID] = i
		}
		afterPosition, ok := order[after]
		if !ok {
			messages = nil
		} else {
			filtered := make([]ioa.MessageRecord, 0, len(messages))
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

func RelatedMessages(allMessages []ioa.MessageRecord, messageID, after string, limit int) []ioa.MessageRecord {
	index := make(map[string]ioa.MessageRecord, len(allMessages))
	children := make(map[string][]string)
	for _, message := range allMessages {
		index[message.ID] = message
		for _, parentID := range message.Refs.Messages {
			children[parentID] = append(children[parentID], message.ID)
		}
	}

	related := make(map[string]struct{})
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

	stack = []string{messageID}
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

	messages := make([]ioa.MessageRecord, 0, len(related))
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

func FilterMessages(messages []ioa.MessageRecord, filter ioa.MessageFilter) []ioa.MessageRecord {
	filtered := make([]ioa.MessageRecord, 0, len(messages))
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
