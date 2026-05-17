package server

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/chainreactors/ioa"
)

func (s *Service) GetGraph(ctx context.Context, opts ioa.GraphOptions) (ioa.GraphView, error) {
	return s.getGraph(ctx, opts)
}

func (s *Service) GetSpaceGraph(ctx context.Context, spaceID string, opts ioa.GraphOptions) (ioa.GraphView, error) {
	opts.SpaceID = spaceID
	return s.getGraph(ctx, opts)
}

func (s *Service) getGraph(ctx context.Context, opts ioa.GraphOptions) (ioa.GraphView, error) {
	include, err := graphIncludeSet(opts.Include)
	if err != nil {
		return ioa.GraphView{}, err
	}
	if err := s.validateMessageFilter(opts.MessageFilter); err != nil {
		return ioa.GraphView{}, err
	}
	messages, err := s.graphMessages(opts.MessageFilter)
	if err != nil {
		return ioa.GraphView{}, err
	}
	return s.buildGraphView(ctx, opts.MessageFilter, messages, include)
}

func (s *Service) graphMessages(filter ioa.MessageFilter) ([]ioa.MessageRecord, error) {
	if filter.MessageID == "" {
		return s.store.ListMessages(filter)
	}
	if filter.SpaceID == "" {
		record, ok, err := s.findMessage("", filter.MessageID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ioa.ProtocolError(http.StatusNotFound, "Message '%s' not found", filter.MessageID)
		}
		filter.SpaceID = record.SpaceID
	}
	records, err := s.store.GetRelatedMessages(filter.SpaceID, filter.MessageID, filter.After, filter.Limit)
	if err != nil {
		return nil, err
	}
	filter.MessageID = ""
	filter.After = ""
	filter.Limit = 0
	return FilterMessages(records, filter), nil
}

func (s *Service) buildGraphView(ctx context.Context, filter ioa.MessageFilter, messages []ioa.MessageRecord, include map[string]bool) (ioa.GraphView, error) {
	spaces, err := s.ListSpaces(ctx)
	if err != nil {
		return ioa.GraphView{}, err
	}
	nodes, err := s.ListNodes(ctx)
	if err != nil {
		return ioa.GraphView{}, err
	}

	spaceIDs := make(map[string]struct{})
	nodeIDs := make(map[string]struct{})
	messageIDs := make(map[string]struct{})
	if graphHasNoStructuralFilter(filter) {
		for _, space := range spaces {
			spaceIDs[space.ID] = struct{}{}
		}
		for _, node := range nodes {
			nodeIDs[node.ID] = struct{}{}
		}
	}
	if filter.SpaceID != "" {
		spaceIDs[filter.SpaceID] = struct{}{}
	}
	if filter.NodeID != "" {
		nodeIDs[filter.NodeID] = struct{}{}
	}
	if filter.Sender != "" {
		nodeIDs[filter.Sender] = struct{}{}
	}
	if filter.RefNode != "" {
		nodeIDs[filter.RefNode] = struct{}{}
	}
	for _, message := range messages {
		messageIDs[message.ID] = struct{}{}
		spaceIDs[message.SpaceID] = struct{}{}
		nodeIDs[message.Sender] = struct{}{}
		for _, nodeID := range message.Refs.Nodes {
			nodeIDs[nodeID] = struct{}{}
		}
	}

	for _, space := range spaces {
		includeSpace := hasID(spaceIDs, space.ID)
		for _, member := range space.Nodes {
			if hasID(nodeIDs, member.ID) {
				includeSpace = true
				break
			}
		}
		if !includeSpace {
			continue
		}
		spaceIDs[space.ID] = struct{}{}
		for _, member := range space.Nodes {
			nodeIDs[member.ID] = struct{}{}
		}
	}

	viewSpaces := make([]ioa.SpaceInfo, 0, len(spaces))
	for _, space := range spaces {
		if hasID(spaceIDs, space.ID) {
			viewSpaces = append(viewSpaces, space)
		}
	}
	viewNodes := make([]ioa.Node, 0, len(nodes))
	for _, node := range nodes {
		if hasID(nodeIDs, node.ID) {
			viewNodes = append(viewNodes, node)
		}
	}
	sort.Slice(viewSpaces, func(i, j int) bool {
		if viewSpaces[i].Name == viewSpaces[j].Name {
			return viewSpaces[i].ID < viewSpaces[j].ID
		}
		return viewSpaces[i].Name < viewSpaces[j].Name
	})
	sort.Slice(viewNodes, func(i, j int) bool {
		if viewNodes[i].Name == viewNodes[j].Name {
			return viewNodes[i].ID < viewNodes[j].ID
		}
		return viewNodes[i].Name < viewNodes[j].Name
	})

	edges := make([]ioa.GraphEdge, 0)
	edgeKeys := make(map[string]struct{})
	addEdge := func(source, target, kind string) {
		key := source + "\x00" + target + "\x00" + kind
		if _, ok := edgeKeys[key]; ok {
			return
		}
		edgeKeys[key] = struct{}{}
		edges = append(edges, ioa.GraphEdge{Source: source, Target: target, Kind: kind})
	}
	for _, space := range viewSpaces {
		for _, member := range space.Nodes {
			if hasID(nodeIDs, member.ID) {
				addEdge("space:"+space.ID, "node:"+member.ID, "member")
			}
		}
	}
	for _, message := range messages {
		if !hasID(messageIDs, message.ID) {
			continue
		}
		if hasID(nodeIDs, message.Sender) {
			addEdge("node:"+message.Sender, "message:"+message.ID, "sender")
		}
		for _, ref := range message.Refs.Messages {
			if hasID(messageIDs, ref) {
				addEdge("message:"+message.ID, "message:"+ref, "refs.messages")
			}
		}
		for _, ref := range message.Refs.Nodes {
			if hasID(nodeIDs, ref) {
				addEdge("message:"+message.ID, "node:"+ref, "refs.nodes")
			}
		}
	}

	view := ioa.GraphView{
		Spaces:   viewSpaces,
		Nodes:    viewNodes,
		Messages: messages,
		Edges:    edges,
		Stats: ioa.GraphStats{
			SpaceCount:   len(viewSpaces),
			NodeCount:    len(viewNodes),
			MessageCount: len(messages),
			EdgeCount:    len(edges),
		},
	}
	if !include["spaces"] {
		view.Spaces = []ioa.SpaceInfo{}
	}
	if !include["nodes"] {
		view.Nodes = []ioa.Node{}
	}
	if !include["messages"] {
		view.Messages = []ioa.MessageRecord{}
	}
	if !include["edges"] {
		view.Edges = []ioa.GraphEdge{}
	}
	return view, nil
}

func (s *Service) validateMessageFilter(filter ioa.MessageFilter) error {
	if filter.Limit < 0 {
		return ioa.ProtocolError(http.StatusUnprocessableEntity, "limit must be greater than 0")
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
			return ioa.ProtocolError(http.StatusNotFound, "%s: node '%s' not found", field, nodeID)
		}
	}
	if filter.MessageID != "" {
		if _, ok, err := s.findMessage(filter.SpaceID, filter.MessageID); err != nil {
			return err
		} else if !ok {
			return ioa.ProtocolError(http.StatusNotFound, "Message '%s' not found", filter.MessageID)
		}
	}
	if filter.RefMessage != "" {
		if _, ok, err := s.findMessage(filter.SpaceID, filter.RefMessage); err != nil {
			return err
		} else if !ok {
			return ioa.ProtocolError(http.StatusUnprocessableEntity, "ref_message: '%s' not found", filter.RefMessage)
		}
	}
	if filter.After != "" {
		if _, ok, err := s.findMessage(filter.SpaceID, filter.After); err != nil {
			return err
		} else if !ok {
			return ioa.ProtocolError(http.StatusUnprocessableEntity, "after: '%s' not found", filter.After)
		}
	}
	return nil
}

func (s *Service) findMessage(spaceID, messageID string) (ioa.MessageRecord, bool, error) {
	if spaceID != "" {
		return s.store.GetMessage(spaceID, messageID)
	}
	records, err := s.store.ListMessages(ioa.MessageFilter{MessageID: messageID})
	if err != nil {
		return ioa.MessageRecord{}, false, err
	}
	if len(records) == 0 {
		return ioa.MessageRecord{}, false, nil
	}
	return records[0], true, nil
}

func graphIncludeSet(includes []string) (map[string]bool, error) {
	result := map[string]bool{
		"spaces":   true,
		"nodes":    true,
		"messages": true,
		"edges":    true,
	}
	if len(includes) == 0 {
		return result, nil
	}
	for key := range result {
		result[key] = false
	}
	for _, include := range includes {
		include = strings.TrimSpace(include)
		if include == "" {
			continue
		}
		if _, ok := result[include]; !ok {
			return nil, ioa.ProtocolError(http.StatusUnprocessableEntity, "include: unsupported value '%s'", include)
		}
		result[include] = true
	}
	return result, nil
}

func graphHasNoStructuralFilter(filter ioa.MessageFilter) bool {
	return filter.SpaceID == "" &&
		filter.MessageID == "" &&
		filter.NodeID == "" &&
		filter.Sender == "" &&
		filter.RefMessage == "" &&
		filter.RefNode == ""
}

func hasID(ids map[string]struct{}, id string) bool {
	_, ok := ids[id]
	return ok
}
