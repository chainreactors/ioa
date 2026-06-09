package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chainreactors/ioa/api"
	"github.com/chainreactors/ioa/protocols"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Service struct {
	store     Store
	hub       *Hub
	accessKey string
}

func NewService(store Store, accessKey string) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store, hub: NewHub(), accessKey: accessKey}
}

func (s *Service) AccessKey() string {
	return s.accessKey
}

func (s *Service) Store() Store {
	return s.store
}

func (s *Service) Hub() *Hub {
	return s.hub
}

func (s *Service) Ready(ctx context.Context) error {
	_, err := s.ListSpaces(ctx)
	return err
}

func (s *Service) ListSpaces(ctx context.Context) ([]protocols.SpaceInfo, error) {
	spaces, err := s.store.ListSpaces()
	if err != nil {
		return nil, err
	}
	result := make([]protocols.SpaceInfo, 0, len(spaces))
	for _, space := range spaces {
		info, err := s.spaceInfo(space)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}

func (s *Service) ListNodes(ctx context.Context) ([]protocols.Node, error) {
	return s.store.ListNodes()
}

func (s *Service) RegisterNode(ctx context.Context, body api.NodeCreate) (protocols.Node, error) {
	if strings.TrimSpace(body.Name) == "" {
		return protocols.Node{}, protocols.ProtocolError(http.StatusUnprocessableEntity, "name is required")
	}
	node := protocols.Node{
		ID:          protocols.NewID(),
		Name:        body.Name,
		Description: body.Description,
		Meta:        defaultMeta(body.Meta),
	}
	if err := s.store.PutNode(node); err != nil {
		return protocols.Node{}, err
	}
	return node, nil
}

func (s *Service) GetNode(ctx context.Context, nodeID string) (protocols.Node, error) {
	node, ok, err := s.store.GetNode(nodeID)
	if err != nil {
		return protocols.Node{}, err
	}
	if !ok {
		return protocols.Node{}, protocols.ProtocolError(http.StatusNotFound, "Node '%s' not found", nodeID)
	}
	return node, nil
}

func (s *Service) CreateSpace(ctx context.Context, callerNodeID string, body api.SpaceCreate) (protocols.SpaceInfo, error) {
	nodeID, err := s.callerNodeID(callerNodeID)
	if err != nil {
		return protocols.SpaceInfo{}, err
	}
	if strings.TrimSpace(body.Name) == "" {
		return protocols.SpaceInfo{}, protocols.ProtocolError(http.StatusUnprocessableEntity, "Space name is required")
	}
	description := body.Description
	if strings.TrimSpace(description) == "" {
		node, ok, _ := s.store.GetNode(nodeID)
		if ok && node.Description != "" {
			description = node.Description
		}
	}
	if strings.TrimSpace(description) == "" {
		return protocols.SpaceInfo{}, protocols.ProtocolError(http.StatusUnprocessableEntity, "Space description is required")
	}
	space, err := s.store.PutSpaceIfAbsent(protocols.Space{ID: protocols.NewID(), Name: body.Name, Tags: body.Tags})
	if err != nil {
		return protocols.SpaceInfo{}, err
	}
	if err := s.store.JoinSpace(space.ID, nodeID, description); err != nil {
		return protocols.SpaceInfo{}, err
	}
	return s.spaceInfo(space)
}

func (s *Service) GetSpace(ctx context.Context, spaceID string) (protocols.SpaceInfo, error) {
	space, err := s.requireSpace(spaceID)
	if err != nil {
		return protocols.SpaceInfo{}, err
	}
	return s.spaceInfo(space)
}

func (s *Service) SendMessage(ctx context.Context, spaceID, callerNodeID string, body protocols.SendMessage) (protocols.Message, error) {
	if _, err := s.requireSpace(spaceID); err != nil {
		return protocols.Message{}, err
	}
	sender, err := s.callerNodeID(callerNodeID)
	if err != nil {
		return protocols.Message{}, err
	}
	if body.Content == nil {
		return protocols.Message{}, protocols.ProtocolError(http.StatusUnprocessableEntity, "content is required")
	}
	refs := emptyRef()
	if body.Refs != nil {
		refs = completeRef(*body.Refs)
	}
	if err := s.validateRefs(refs, spaceID); err != nil {
		return protocols.Message{}, err
	}

	if body.ContentSchema != nil {
		if err := compileContentSchema(body.ContentSchema); err != nil {
			return protocols.Message{}, protocols.ProtocolError(http.StatusUnprocessableEntity, "content_schema is not valid JSON Schema: %s", err)
		}
	}

	record := protocols.Message{
		ID:            protocols.NewID(),
		SpaceID:       spaceID,
		Sender:        sender,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		ContentType:   body.ContentType,
		Content:       body.Content,
		Refs:          refs,
		Meta:          body.Meta,
		ContentSchema: body.ContentSchema,
	}
	if err := s.store.AppendMessage(record); err != nil {
		return protocols.Message{}, err
	}
	message := record
	s.hub.Broadcast(spaceID, message)
	for _, nid := range refs.Nodes {
		s.hub.BroadcastToNode(nid, message)
	}
	return message, nil
}

func (s *Service) ReadMessages(ctx context.Context, spaceID, callerNodeID string, opts protocols.ReadOptions) ([]protocols.Message, error) {
	if _, err := s.requireSpace(spaceID); err != nil {
		return nil, err
	}
	if opts.Limit < 0 {
		return nil, protocols.ProtocolError(http.StatusUnprocessableEntity, "limit must be greater than 0")
	}
	if opts.After != "" {
		if _, ok, err := s.store.GetMessage(spaceID, opts.After); err != nil {
			return nil, err
		} else if !ok {
			return nil, protocols.ProtocolError(http.StatusUnprocessableEntity, "after: '%s' not found in space '%s'", opts.After, spaceID)
		}
	}

	var records []protocols.Message
	var err error
	if opts.MessageID != "" {
		if _, ok, err := s.store.GetMessage(spaceID, opts.MessageID); err != nil {
			return nil, err
		} else if !ok {
			return nil, protocols.ProtocolError(http.StatusNotFound, "Message '%s' not found in space '%s'", opts.MessageID, spaceID)
		}
		if opts.Direction != "" && opts.Direction != "upstream" && opts.Direction != "downstream" {
			return nil, protocols.ProtocolError(http.StatusUnprocessableEntity, "direction must be 'upstream' or 'downstream'")
		}
		records, err = s.store.GetRelatedMessages(spaceID, opts.MessageID, opts.Direction, opts.After, opts.Limit)
	} else if opts.All {
		records, err = s.store.GetMessages(spaceID, opts.After, opts.Limit)
	} else if callerNodeID != "" {
		if _, ok, err := s.store.GetNode(callerNodeID); err != nil {
			return nil, err
		} else if !ok {
			return nil, protocols.ProtocolError(http.StatusUnprocessableEntity, "caller node '%s' not found", callerNodeID)
		}
		records, err = s.store.GetMessagesForNode(spaceID, callerNodeID, opts.After, opts.Limit)
	} else {
		records, err = s.store.GetStartMessages(spaceID, opts.After, opts.Limit)
	}
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Service) GetInbox(ctx context.Context, nodeID string, opts protocols.ReadOptions) ([]protocols.Message, error) {
	if _, err := s.callerNodeID(nodeID); err != nil {
		return nil, err
	}
	records, err := s.store.GetInboxMessages(nodeID, opts.After, opts.Limit)
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []protocols.Message{}
	}
	return records, nil
}

func (s *Service) ListMessages(ctx context.Context, filter api.MessageFilter) ([]protocols.Message, error) {
	if err := s.validateMessageFilter(filter); err != nil {
		return nil, err
	}
	records, err := s.store.ListMessages(filter)
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []protocols.Message{}
	}
	return records, nil
}

func (s *Service) IsRelated(ctx context.Context, spaceID, rootMessageID, messageID string) (bool, error) {
	records, err := s.store.GetRelatedMessages(spaceID, rootMessageID, "", "", 0)
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

func (s *Service) AuthRegister(ctx context.Context, body api.AuthRegister) (api.AuthResponse, error) {
	if body.AccessKey != s.accessKey {
		return api.AuthResponse{}, protocols.ProtocolError(http.StatusForbidden, "invalid access key")
	}
	if strings.TrimSpace(body.Name) == "" {
		return api.AuthResponse{}, protocols.ProtocolError(http.StatusUnprocessableEntity, "name is required")
	}
	node, ok, err := s.store.GetNodeByName(body.Name)
	if err != nil {
		return api.AuthResponse{}, err
	}
	if !ok {
		node, err = s.RegisterNode(ctx, api.NodeCreate{Name: body.Name, Description: body.Description, Meta: body.Meta})
		if err != nil {
			return api.AuthResponse{}, err
		}
	}
	token := protocols.NewToken()
	if err := s.store.PutToken(protocols.TokenHash(token), node.ID); err != nil {
		return api.AuthResponse{}, err
	}
	return api.AuthResponse{ID: node.ID, Name: node.Name, Token: token}, nil
}

func (s *Service) ResolveToken(token string) (protocols.Node, error) {
	hash := protocols.TokenHash(token)
	node, ok, err := s.store.GetNodeByTokenHash(hash)
	if err != nil {
		return protocols.Node{}, err
	}
	if !ok {
		return protocols.Node{}, protocols.ProtocolError(http.StatusUnauthorized, "invalid token")
	}
	return node, nil
}

func (s *Service) validateMessageFilter(filter api.MessageFilter) error {
	if filter.Limit < 0 {
		return protocols.ProtocolError(http.StatusUnprocessableEntity, "limit must be greater than 0")
	}
	if filter.SpaceID != "" {
		if _, err := s.requireSpace(filter.SpaceID); err != nil {
			return err
		}
	}
	for field, nodeID := range map[string]string{
		"node_id":  filter.NodeID,
		"sender":   filter.Sender,
		"ref_node": filter.RefNode,
	} {
		if nodeID == "" {
			continue
		}
		if _, ok, err := s.store.GetNode(nodeID); err != nil {
			return err
		} else if !ok {
			return protocols.ProtocolError(http.StatusNotFound, "%s: node '%s' not found", field, nodeID)
		}
	}
	if filter.MessageID != "" {
		if _, ok, err := s.findMessage(filter.SpaceID, filter.MessageID); err != nil {
			return err
		} else if !ok {
			return protocols.ProtocolError(http.StatusNotFound, "Message '%s' not found", filter.MessageID)
		}
	}
	if filter.RefMessage != "" {
		if _, ok, err := s.findMessage(filter.SpaceID, filter.RefMessage); err != nil {
			return err
		} else if !ok {
			return protocols.ProtocolError(http.StatusUnprocessableEntity, "ref_message: '%s' not found", filter.RefMessage)
		}
	}
	if filter.After != "" {
		if _, ok, err := s.findMessage(filter.SpaceID, filter.After); err != nil {
			return err
		} else if !ok {
			return protocols.ProtocolError(http.StatusUnprocessableEntity, "after: '%s' not found", filter.After)
		}
	}
	return nil
}

func (s *Service) findMessage(spaceID, messageID string) (protocols.Message, bool, error) {
	if spaceID != "" {
		return s.store.GetMessage(spaceID, messageID)
	}
	records, err := s.store.ListMessages(api.MessageFilter{MessageID: messageID})
	if err != nil {
		return protocols.Message{}, false, err
	}
	if len(records) == 0 {
		return protocols.Message{}, false, nil
	}
	return records[0], true, nil
}

func (s *Service) callerNodeID(nodeID string) (string, error) {
	if nodeID == "" {
		return "", protocols.ProtocolError(http.StatusUnprocessableEntity, "caller node identity is required")
	}
	if _, ok, err := s.store.GetNode(nodeID); err != nil {
		return "", err
	} else if !ok {
		return "", protocols.ProtocolError(http.StatusUnprocessableEntity, "caller node '%s' not found", nodeID)
	}
	return nodeID, nil
}

func (s *Service) requireSpace(spaceID string) (protocols.Space, error) {
	space, ok, err := s.store.GetSpace(spaceID)
	if err != nil {
		return protocols.Space{}, err
	}
	if !ok {
		return protocols.Space{}, protocols.ProtocolError(http.StatusNotFound, "Space '%s' not found", spaceID)
	}
	return space, nil
}

func (s *Service) spaceInfo(space protocols.Space) (protocols.SpaceInfo, error) {
	nodes, err := s.store.GetSpaceNodes(space.ID)
	if err != nil {
		return protocols.SpaceInfo{}, err
	}
	count, err := s.store.GetMessageCount(space.ID)
	if err != nil {
		return protocols.SpaceInfo{}, err
	}
	return protocols.SpaceInfo{
		ID:           space.ID,
		Name:         space.Name,
		Tags:         space.Tags,
		Nodes:        nodes,
		MessageCount: count,
	}, nil
}

func (s *Service) validateRefs(refs protocols.Ref, spaceID string) error {
	for _, mid := range refs.Messages {
		if _, ok, err := s.store.GetMessage(spaceID, mid); err != nil {
			return err
		} else if !ok {
			return protocols.ProtocolError(http.StatusUnprocessableEntity, "refs.messages: '%s' not found in space '%s'", mid, spaceID)
		}
	}
	for _, nid := range refs.Nodes {
		if _, ok, err := s.store.GetNode(nid); err != nil {
			return err
		} else if !ok {
			return protocols.ProtocolError(http.StatusUnprocessableEntity, "refs.nodes: '%s' not found", nid)
		}
	}
	return nil
}

func defaultMeta(meta map[string]interface{}) map[string]interface{} {
	if meta == nil {
		return map[string]interface{}{}
	}
	return meta
}

func emptyRef() protocols.Ref {
	return protocols.Ref{
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

func completeRef(ref protocols.Ref) protocols.Ref {
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
	if protocol, ok := err.(*protocols.Error); ok {
		return protocol.Status
	}
	return http.StatusInternalServerError
}

func detailOf(err error) string {
	if err == nil {
		return ""
	}
	if protocol, ok := err.(*protocols.Error); ok {
		return protocol.Detail
	}
	return fmt.Sprintf("%v", err)
}
