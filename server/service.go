package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chainreactors/ioa"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Service struct {
	store Store
	hub   *Hub
}

func NewService(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store, hub: NewHub()}
}

func (s *Service) Store() Store {
	return s.store
}

func (s *Service) Hub() *Hub {
	return s.hub
}

func (s *Service) ListSpaces(ctx context.Context) ([]ioa.SpaceInfo, error) {
	spaces, err := s.store.ListSpaces()
	if err != nil {
		return nil, err
	}
	result := make([]ioa.SpaceInfo, 0, len(spaces))
	for _, space := range spaces {
		info, err := s.spaceInfo(space)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}

func (s *Service) ListNodes(ctx context.Context) ([]ioa.Node, error) {
	return s.store.ListNodes()
}

func (s *Service) RegisterNode(ctx context.Context, body ioa.NodeCreate) (ioa.Node, error) {
	if strings.TrimSpace(body.Name) == "" {
		return ioa.Node{}, ioa.ProtocolError(http.StatusUnprocessableEntity, "name is required")
	}
	node := ioa.Node{
		ID:   ioa.NewID(),
		Name: body.Name,
		Meta: defaultMeta(body.Meta),
	}
	if err := s.store.PutNode(node); err != nil {
		return ioa.Node{}, err
	}
	return node, nil
}

func (s *Service) GetNode(ctx context.Context, nodeID string) (ioa.Node, error) {
	node, ok, err := s.store.GetNode(nodeID)
	if err != nil {
		return ioa.Node{}, err
	}
	if !ok {
		return ioa.Node{}, ioa.ProtocolError(http.StatusNotFound, "Node '%s' not found", nodeID)
	}
	return node, nil
}

func (s *Service) CreateSpace(ctx context.Context, callerNodeID string, body ioa.SpaceCreate) (ioa.SpaceInfo, error) {
	nodeID, err := s.callerNodeID(callerNodeID)
	if err != nil {
		return ioa.SpaceInfo{}, err
	}
	if strings.TrimSpace(body.Name) == "" {
		return ioa.SpaceInfo{}, ioa.ProtocolError(http.StatusUnprocessableEntity, "Space name is required")
	}
	if strings.TrimSpace(body.Description) == "" {
		return ioa.SpaceInfo{}, ioa.ProtocolError(http.StatusUnprocessableEntity, "Space description is required")
	}
	space, err := s.store.PutSpaceIfAbsent(ioa.Space{ID: ioa.NewID(), Name: body.Name})
	if err != nil {
		return ioa.SpaceInfo{}, err
	}
	if err := s.store.JoinSpace(space.ID, nodeID, body.Description); err != nil {
		return ioa.SpaceInfo{}, err
	}
	return s.spaceInfo(space)
}

func (s *Service) GetSpace(ctx context.Context, spaceID string) (ioa.SpaceInfo, error) {
	space, err := s.requireSpace(spaceID)
	if err != nil {
		return ioa.SpaceInfo{}, err
	}
	return s.spaceInfo(space)
}

func (s *Service) SendMessage(ctx context.Context, spaceID, callerNodeID string, body ioa.SendMessage) (ioa.Message, error) {
	if _, err := s.requireSpace(spaceID); err != nil {
		return ioa.Message{}, err
	}
	sender, err := s.callerNodeID(callerNodeID)
	if err != nil {
		return ioa.Message{}, err
	}
	if body.Content == nil {
		return ioa.Message{}, ioa.ProtocolError(http.StatusUnprocessableEntity, "content is required")
	}
	if body.ContentSchema != nil {
		if err := compileContentSchema(body.ContentSchema); err != nil {
			return ioa.Message{}, ioa.ProtocolError(http.StatusUnprocessableEntity, "content_schema is not valid JSON Schema: %s", err)
		}
		if err := s.store.SetContentSchema(spaceID, body.ContentSchema); err != nil {
			return ioa.Message{}, err
		}
	} else {
		schema, err := s.store.GetContentSchema(spaceID)
		if err != nil {
			return ioa.Message{}, err
		}
		if schema != nil {
			if err := validateContent(body.Content, schema); err != nil {
				return ioa.Message{}, ioa.ProtocolError(http.StatusUnprocessableEntity, "content does not match space schema: %s", err)
			}
		}
	}
	refs := emptyRef()
	if body.Refs != nil {
		refs = completeRef(*body.Refs)
	}
	if err := s.validateRefs(refs, spaceID); err != nil {
		return ioa.Message{}, err
	}
	record := ioa.MessageRecord{
		ID:      ioa.NewID(),
		SpaceID: spaceID,
		Sender:  sender,
		Content: body.Content,
		Refs:    refs,
	}
	if err := s.store.AppendMessage(record); err != nil {
		return ioa.Message{}, err
	}
	message := ioa.ExposeMessage(record)
	s.hub.Broadcast(spaceID, message)
	return message, nil
}

func (s *Service) ReadMessages(ctx context.Context, spaceID, callerNodeID string, opts ioa.ReadOptions) ([]ioa.Message, error) {
	if _, err := s.requireSpace(spaceID); err != nil {
		return nil, err
	}
	if opts.Limit < 0 {
		return nil, ioa.ProtocolError(http.StatusUnprocessableEntity, "limit must be greater than 0")
	}
	if opts.Limit == 0 {
		opts.Limit = 0
	}
	if opts.After != "" {
		if _, ok, err := s.store.GetMessage(spaceID, opts.After); err != nil {
			return nil, err
		} else if !ok {
			return nil, ioa.ProtocolError(http.StatusUnprocessableEntity, "after: '%s' not found in space '%s'", opts.After, spaceID)
		}
	}

	var records []ioa.MessageRecord
	var err error
	if opts.MessageID != "" {
		if _, ok, err := s.store.GetMessage(spaceID, opts.MessageID); err != nil {
			return nil, err
		} else if !ok {
			return nil, ioa.ProtocolError(http.StatusNotFound, "Message '%s' not found in space '%s'", opts.MessageID, spaceID)
		}
		records, err = s.store.GetRelatedMessages(spaceID, opts.MessageID, opts.After, opts.Limit)
	} else if opts.All {
		records, err = s.store.GetMessages(spaceID, opts.After, opts.Limit)
	} else if callerNodeID != "" {
		if _, ok, err := s.store.GetNode(callerNodeID); err != nil {
			return nil, err
		} else if !ok {
			return nil, ioa.ProtocolError(http.StatusUnprocessableEntity, "caller node '%s' not found", callerNodeID)
		}
		records, err = s.store.GetMessagesForNode(spaceID, callerNodeID, opts.After, opts.Limit)
	} else {
		records, err = s.store.GetStartMessages(spaceID, opts.After, opts.Limit)
	}
	if err != nil {
		return nil, err
	}
	return exposeMessages(records), nil
}

func (s *Service) IsRelated(ctx context.Context, spaceID, rootMessageID, messageID string) (bool, error) {
	records, err := s.store.GetRelatedMessages(spaceID, rootMessageID, "", 0)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if record.ID == messageID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) callerNodeID(nodeID string) (string, error) {
	if nodeID == "" {
		return "", ioa.ProtocolError(http.StatusUnprocessableEntity, "caller node identity is required")
	}
	if _, ok, err := s.store.GetNode(nodeID); err != nil {
		return "", err
	} else if !ok {
		return "", ioa.ProtocolError(http.StatusUnprocessableEntity, "caller node '%s' not found", nodeID)
	}
	return nodeID, nil
}

func (s *Service) requireSpace(spaceID string) (ioa.Space, error) {
	space, ok, err := s.store.GetSpace(spaceID)
	if err != nil {
		return ioa.Space{}, err
	}
	if !ok {
		return ioa.Space{}, ioa.ProtocolError(http.StatusNotFound, "Space '%s' not found", spaceID)
	}
	return space, nil
}

func (s *Service) spaceInfo(space ioa.Space) (ioa.SpaceInfo, error) {
	nodes, err := s.store.GetSpaceNodes(space.ID)
	if err != nil {
		return ioa.SpaceInfo{}, err
	}
	count, err := s.store.GetMessageCount(space.ID)
	if err != nil {
		return ioa.SpaceInfo{}, err
	}
	schema, err := s.store.GetContentSchema(space.ID)
	if err != nil {
		return ioa.SpaceInfo{}, err
	}
	info := ioa.SpaceInfo{
		ID:            space.ID,
		Name:          space.Name,
		Nodes:         make([]ioa.SpaceNode, 0, len(nodes)),
		MessageCount:  count,
		ContentSchema: schema,
	}
	for _, node := range nodes {
		info.Nodes = append(info.Nodes, ioa.SpaceNode{
			ID:          node.Node.ID,
			Name:        node.Node.Name,
			Description: node.Description,
		})
	}
	return info, nil
}

func (s *Service) validateRefs(refs ioa.Ref, spaceID string) error {
	for _, mid := range refs.Messages {
		if _, ok, err := s.store.GetMessage(spaceID, mid); err != nil {
			return err
		} else if !ok {
			return ioa.ProtocolError(http.StatusUnprocessableEntity, "refs.messages: '%s' not found in space '%s'", mid, spaceID)
		}
	}
	for _, nid := range refs.Nodes {
		if _, ok, err := s.store.GetNode(nid); err != nil {
			return err
		} else if !ok {
			return ioa.ProtocolError(http.StatusUnprocessableEntity, "refs.nodes: '%s' not found", nid)
		}
	}
	return nil
}

func exposeMessages(records []ioa.MessageRecord) []ioa.Message {
	messages := make([]ioa.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, ioa.ExposeMessage(record))
	}
	return messages
}

func defaultMeta(meta map[string]interface{}) map[string]interface{} {
	if meta == nil {
		return map[string]interface{}{}
	}
	return meta
}

func emptyRef() ioa.Ref {
	return ioa.Ref{
		Messages: []string{},
		Nodes:    []string{},
	}
}

func compileContentSchema(schema map[string]interface{}) error {
	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", v); err != nil {
		return err
	}
	_, err = c.Compile("schema.json")
	return err
}

func validateContent(content, schema map[string]interface{}) error {
	schemaData, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	var schemaValue interface{}
	if err := json.Unmarshal(schemaData, &schemaValue); err != nil {
		return err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", schemaValue); err != nil {
		return err
	}
	compiled, err := c.Compile("schema.json")
	if err != nil {
		return err
	}
	contentData, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var contentValue interface{}
	if err := json.Unmarshal(contentData, &contentValue); err != nil {
		return err
	}
	return compiled.Validate(contentValue)
}

func completeRef(ref ioa.Ref) ioa.Ref {
	if ref.Messages == nil {
		ref.Messages = []string{}
	}
	if ref.Nodes == nil {
		ref.Nodes = []string{}
	}
	return ref
}

func statusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if protocol, ok := err.(*ioa.Error); ok {
		return protocol.Status
	}
	return http.StatusInternalServerError
}

func detailOf(err error) string {
	if err == nil {
		return ""
	}
	if protocol, ok := err.(*ioa.Error); ok {
		return protocol.Detail
	}
	return fmt.Sprintf("%v", err)
}
