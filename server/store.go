package server

import "github.com/chainreactors/ioa"

type Store interface {
	PutNode(node ioa.Node) error
	GetNode(nodeID string) (ioa.Node, bool, error)
	ListNodes() ([]ioa.Node, error)

	PutSpaceIfAbsent(space ioa.Space) (ioa.Space, error)
	GetSpace(spaceID string) (ioa.Space, bool, error)
	ListSpaces() ([]ioa.Space, error)
	JoinSpace(spaceID, nodeID, description string) error
	GetSpaceNodes(spaceID string) ([]ioa.SpaceNodeRecord, error)

	AppendMessage(message ioa.MessageRecord) error
	GetMessage(spaceID, messageID string) (ioa.MessageRecord, bool, error)
	GetMessagesForNode(spaceID, nodeID, after string, limit int) ([]ioa.MessageRecord, error)
	GetMessageCount(spaceID string) (int, error)
	GetMessages(spaceID, after string, limit int) ([]ioa.MessageRecord, error)
	GetStartMessages(spaceID, after string, limit int) ([]ioa.MessageRecord, error)
	GetRelatedMessages(spaceID, messageID, after string, limit int) ([]ioa.MessageRecord, error)
}
