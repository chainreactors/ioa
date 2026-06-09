package server

import (
	"github.com/chainreactors/ioa/api"
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
	GetRelatedMessages(spaceID, messageID, after string, limit int) ([]protocols.Message, error)
	GetInboxMessages(nodeID, after string, limit int) ([]protocols.Message, error)
	ListMessages(filter api.MessageFilter) ([]protocols.Message, error)

	PutToken(tokenHash string, nodeID string) error
	GetNodeByTokenHash(tokenHash string) (protocols.Node, bool, error)
}
