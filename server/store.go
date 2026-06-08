package server

import "github.com/chainreactors/ioa"

type Store interface {
	PutNode(node ioa.Node) error
	GetNode(nodeID string) (ioa.Node, bool, error)
	GetNodeByName(name string) (ioa.Node, bool, error)
	ListNodes() ([]ioa.Node, error)

	PutSpaceIfAbsent(space ioa.Space) (ioa.Space, error)
	GetSpace(spaceID string) (ioa.Space, bool, error)
	ListSpaces() ([]ioa.Space, error)
	JoinSpace(spaceID, nodeID, description string) error
	GetSpaceNodes(spaceID string) ([]ioa.SpaceNodeRecord, error)

	SetContentSchema(spaceID, rootMessageID string, schema map[string]interface{}) error
	GetContentSchema(spaceID, rootMessageID string) (map[string]interface{}, error)

	AppendMessage(message ioa.MessageRecord) error
	GetMessage(spaceID, messageID string) (ioa.MessageRecord, bool, error)
	GetMessagesForNode(spaceID, nodeID, after string, limit int) ([]ioa.MessageRecord, error)
	GetMessageCount(spaceID string) (int, error)
	GetMessages(spaceID, after string, limit int) ([]ioa.MessageRecord, error)
	GetStartMessages(spaceID, after string, limit int) ([]ioa.MessageRecord, error)
	GetRelatedMessages(spaceID, messageID, after string, limit int) ([]ioa.MessageRecord, error)
	GetInboxMessages(nodeID, after string, limit int) ([]ioa.MessageRecord, error)
	ListMessages(filter ioa.MessageFilter) ([]ioa.MessageRecord, error)

	PutToken(tokenHash string, nodeID string) error
	GetNodeByTokenHash(tokenHash string) (ioa.Node, bool, error)
}
