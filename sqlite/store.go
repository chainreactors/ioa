//go:build sqlite

package sqlite

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/server"
	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SQLiteStore struct {
	db *gorm.DB
}

type nodeModel struct {
	ID          string `gorm:"column:id;primaryKey"`
	Name        string `gorm:"column:name;not null"`
	Description string `gorm:"column:description"`
	MetaJSON    string `gorm:"column:meta_json;not null"`
}

func (nodeModel) TableName() string { return "nodes" }

type spaceModel struct {
	ID       string `gorm:"column:id;primaryKey"`
	Name     string `gorm:"column:name;uniqueIndex;not null"`
	TagsJSON string `gorm:"column:tags_json"`
}

func (spaceModel) TableName() string { return "spaces" }

type spaceNodeModel struct {
	SpaceID     string `gorm:"column:space_id;primaryKey"`
	NodeID      string `gorm:"column:node_id;primaryKey"`
	Description string `gorm:"column:description;not null"`
}

func (spaceNodeModel) TableName() string { return "space_nodes" }

type messageModel struct {
	Seq               uint   `gorm:"column:seq;primaryKey;autoIncrement;index:idx_messages_space_seq,priority:2"`
	ID                string `gorm:"column:id;uniqueIndex;not null"`
	SpaceID           string `gorm:"column:space_id;not null;index:idx_messages_space_seq,priority:1"`
	Sender            string `gorm:"column:sender;not null"`
	CreatedAt         string `gorm:"column:created_at"`
	ContentType       string `gorm:"column:content_type"`
	ContentJSON       string `gorm:"column:content_json;not null"`
	RefsJSON          string `gorm:"column:refs_json;not null"`
	MetaJSON          string `gorm:"column:meta_json"`
	ContentSchemaJSON string `gorm:"column:content_schema_json"`
}

func (messageModel) TableName() string { return "messages" }

type tokenModel struct {
	Hash   string `gorm:"column:hash;primaryKey"`
	NodeID string `gorm:"column:node_id;not null"`
}

func (tokenModel) TableName() string { return "tokens" }

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}
	db, err := gorm.Open(gormsqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *SQLiteStore) initSchema() error {
	return s.db.AutoMigrate(
		&nodeModel{},
		&spaceModel{},
		&spaceNodeModel{},
		&messageModel{},
		&tokenModel{},
	)
}

func (s *SQLiteStore) PutNode(node ioa.Node) error {
	model, err := makeNodeModel(node)
	if err != nil {
		return err
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "description", "meta_json"}),
	}).Create(&model).Error
}

func (s *SQLiteStore) GetNode(nodeID string) (ioa.Node, bool, error) {
	var model nodeModel
	if err := s.db.Where(&nodeModel{ID: nodeID}).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ioa.Node{}, false, nil
		}
		return ioa.Node{}, false, err
	}
	node, err := model.toNode()
	return node, err == nil, err
}

func (s *SQLiteStore) GetNodeByName(name string) (ioa.Node, bool, error) {
	var model nodeModel
	if err := s.db.Where("name = ?", name).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ioa.Node{}, false, nil
		}
		return ioa.Node{}, false, err
	}
	node, err := model.toNode()
	return node, err == nil, err
}

func (s *SQLiteStore) ListNodes() ([]ioa.Node, error) {
	var models []nodeModel
	if err := s.db.
		Order(clause.OrderByColumn{Column: clause.Column{Name: "name"}}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]ioa.Node, 0, len(models))
	for _, model := range models {
		node, err := model.toNode()
		if err != nil {
			return nil, err
		}
		result = append(result, node)
	}
	return result, nil
}

func (s *SQLiteStore) ListSpaces() ([]ioa.Space, error) {
	var models []spaceModel
	if err := s.db.
		Order(clause.OrderByColumn{Column: clause.Column{Name: "name"}}).
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]ioa.Space, 0, len(models))
	for _, model := range models {
		space, err := model.toSpace()
		if err != nil {
			return nil, err
		}
		result = append(result, space)
	}
	return result, nil
}

func (s *SQLiteStore) PutSpaceIfAbsent(space ioa.Space) (ioa.Space, error) {
	var result ioa.Space
	err := s.db.Transaction(func(tx *gorm.DB) error {
		model, err := makeSpaceModel(space)
		if err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoNothing: true,
		}).Create(&model).Error; err != nil {
			return err
		}

		var existing spaceModel
		if err := tx.Where(&spaceModel{Name: space.Name}).First(&existing).Error; err != nil {
			return err
		}
		existingTags, err := decodeTagsJSON(existing.TagsJSON)
		if err != nil {
			return err
		}
		mergedTags := ioa.MergeTags(existingTags, space.Tags)
		if !slices.Equal(existingTags, mergedTags) {
			encoded, err := encodeTagsJSON(mergedTags)
			if err != nil {
				return err
			}
			if err := tx.Model(&spaceModel{}).
				Where(&spaceModel{ID: existing.ID}).
				Update("tags_json", encoded).Error; err != nil {
				return err
			}
			existing.TagsJSON = encoded
		}
		result, err = existing.toSpace()
		return err
	})
	return result, err
}

func (s *SQLiteStore) GetSpace(spaceID string) (ioa.Space, bool, error) {
	var model spaceModel
	if err := s.db.Where(&spaceModel{ID: spaceID}).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ioa.Space{}, false, nil
		}
		return ioa.Space{}, false, err
	}
	space, err := model.toSpace()
	return space, err == nil, err
}

func (s *SQLiteStore) JoinSpace(spaceID, nodeID, description string) error {
	model := spaceNodeModel{SpaceID: spaceID, NodeID: nodeID, Description: description}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "space_id"}, {Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"description"}),
	}).Create(&model).Error
}

func (s *SQLiteStore) GetSpaceNodes(spaceID string) ([]ioa.SpaceNodeRecord, error) {
	var memberships []spaceNodeModel
	if err := s.db.Where(&spaceNodeModel{SpaceID: spaceID}).Find(&memberships).Error; err != nil {
		return nil, err
	}
	result := make([]ioa.SpaceNodeRecord, 0, len(memberships))
	for _, membership := range memberships {
		node, ok, err := s.GetNode(membership.NodeID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		result = append(result, ioa.SpaceNodeRecord{Node: node, Description: membership.Description})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Node.Name == result[j].Node.Name {
			return result[i].Node.ID < result[j].Node.ID
		}
		return result[i].Node.Name < result[j].Node.Name
	})
	return result, nil
}

func (s *SQLiteStore) AppendMessage(message ioa.MessageRecord) error {
	model, err := makeMessageModel(message)
	if err != nil {
		return err
	}
	return s.db.Create(&model).Error
}

func (s *SQLiteStore) GetMessage(spaceID, messageID string) (ioa.MessageRecord, bool, error) {
	var model messageModel
	if err := s.db.Where(&messageModel{SpaceID: spaceID, ID: messageID}).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ioa.MessageRecord{}, false, nil
		}
		return ioa.MessageRecord{}, false, err
	}
	message, err := model.toMessage()
	return message, err == nil, err
}

func (s *SQLiteStore) GetMessagesForNode(spaceID, nodeID, after string, limit int) ([]ioa.MessageRecord, error) {
	all, err := s.allMessages(spaceID)
	if err != nil {
		return nil, err
	}
	messages := make([]ioa.MessageRecord, 0, len(all))
	for _, message := range all {
		if server.ContainsString(message.Refs.Nodes, nodeID) {
			messages = append(messages, message)
		}
	}
	return server.WindowMessages(messages, all, after, limit), nil
}

func (s *SQLiteStore) GetMessageCount(spaceID string) (int, error) {
	var count int64
	if err := s.db.Model(&messageModel{}).Where(&messageModel{SpaceID: spaceID}).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *SQLiteStore) GetMessages(spaceID, after string, limit int) ([]ioa.MessageRecord, error) {
	all, err := s.allMessages(spaceID)
	if err != nil {
		return nil, err
	}
	return server.WindowMessages(all, all, after, limit), nil
}

func (s *SQLiteStore) GetStartMessages(spaceID, after string, limit int) ([]ioa.MessageRecord, error) {
	all, err := s.allMessages(spaceID)
	if err != nil {
		return nil, err
	}
	messages := make([]ioa.MessageRecord, 0, len(all))
	for _, message := range all {
		if len(message.Refs.Messages) == 0 && len(message.Refs.Nodes) == 0 {
			messages = append(messages, message)
		}
	}
	return server.WindowMessages(messages, all, after, limit), nil
}

func (s *SQLiteStore) GetRelatedMessages(spaceID, messageID, after string, limit int) ([]ioa.MessageRecord, error) {
	all, err := s.allMessages(spaceID)
	if err != nil {
		return nil, err
	}
	return server.RelatedMessages(all, messageID, after, limit), nil
}

func (s *SQLiteStore) GetInboxMessages(nodeID, after string, limit int) ([]ioa.MessageRecord, error) {
	var memberships []spaceNodeModel
	if err := s.db.Where(&spaceNodeModel{NodeID: nodeID}).Find(&memberships).Error; err != nil {
		return nil, err
	}
	spaceIDs := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		spaceIDs = append(spaceIDs, membership.SpaceID)
	}
	sort.Strings(spaceIDs)

	var allMessages []ioa.MessageRecord
	for _, spaceID := range spaceIDs {
		messages, err := s.allMessages(spaceID)
		if err != nil {
			return nil, err
		}
		allMessages = append(allMessages, messages...)
	}
	filtered := make([]ioa.MessageRecord, 0, len(allMessages))
	for _, message := range allMessages {
		if server.ContainsString(message.Refs.Nodes, nodeID) {
			filtered = append(filtered, message)
		}
	}
	return server.WindowMessages(filtered, allMessages, after, limit), nil
}

func (s *SQLiteStore) ListMessages(filter ioa.MessageFilter) ([]ioa.MessageRecord, error) {
	query := s.db.Model(&messageModel{})
	if filter.SpaceID != "" {
		query = query.Where(&messageModel{SpaceID: filter.SpaceID})
	}
	if filter.MessageID != "" {
		query = query.Where(&messageModel{ID: filter.MessageID})
	}
	if filter.Sender != "" {
		query = query.Where(&messageModel{Sender: filter.Sender})
	}

	var models []messageModel
	if err := query.
		Order(clause.OrderByColumn{Column: clause.Column{Name: "seq"}}).
		Find(&models).Error; err != nil {
		return nil, err
	}
	all, err := decodeMessageModels(models)
	if err != nil {
		return nil, err
	}
	filtered := server.FilterMessages(all, filter)
	return server.WindowMessages(filtered, all, filter.After, filter.Limit), nil
}

func (s *SQLiteStore) PutToken(tokenHash string, nodeID string) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"node_id"}),
	}).Create(&tokenModel{Hash: tokenHash, NodeID: nodeID}).Error
}

func (s *SQLiteStore) GetNodeByTokenHash(tokenHash string) (ioa.Node, bool, error) {
	var model tokenModel
	if err := s.db.Where(&tokenModel{Hash: tokenHash}).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ioa.Node{}, false, nil
		}
		return ioa.Node{}, false, err
	}
	return s.GetNode(model.NodeID)
}

func (s *SQLiteStore) allMessages(spaceID string) ([]ioa.MessageRecord, error) {
	var models []messageModel
	if err := s.db.Where(&messageModel{SpaceID: spaceID}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "seq"}}).
		Find(&models).Error; err != nil {
		return nil, err
	}
	return decodeMessageModels(models)
}

func makeNodeModel(node ioa.Node) (nodeModel, error) {
	meta, err := encodeJSON(defaultMap(node.Meta))
	if err != nil {
		return nodeModel{}, err
	}
	return nodeModel{ID: node.ID, Name: node.Name, Description: node.Description, MetaJSON: meta}, nil
}

func (m nodeModel) toNode() (ioa.Node, error) {
	node := ioa.Node{ID: m.ID, Name: m.Name, Description: m.Description, Meta: map[string]interface{}{}}
	if err := decodeJSON(m.MetaJSON, &node.Meta); err != nil {
		return ioa.Node{}, err
	}
	if node.Meta == nil {
		node.Meta = map[string]interface{}{}
	}
	return node, nil
}

func makeSpaceModel(space ioa.Space) (spaceModel, error) {
	tags, err := encodeTagsJSON(space.Tags)
	if err != nil {
		return spaceModel{}, err
	}
	return spaceModel{ID: space.ID, Name: space.Name, TagsJSON: tags}, nil
}

func (m spaceModel) toSpace() (ioa.Space, error) {
	tags, err := decodeTagsJSON(m.TagsJSON)
	if err != nil {
		return ioa.Space{}, err
	}
	return ioa.Space{ID: m.ID, Name: m.Name, Tags: tags}, nil
}

func makeMessageModel(message ioa.MessageRecord) (messageModel, error) {
	content, err := encodeJSON(message.Content)
	if err != nil {
		return messageModel{}, err
	}
	refs, err := encodeJSON(message.Refs)
	if err != nil {
		return messageModel{}, err
	}
	var metaJSON string
	if message.Meta != nil {
		metaJSON, err = encodeJSON(message.Meta)
		if err != nil {
			return messageModel{}, err
		}
	}
	var schemaJSON string
	if message.ContentSchema != nil {
		schemaJSON, err = encodeJSON(message.ContentSchema)
		if err != nil {
			return messageModel{}, err
		}
	}
	return messageModel{
		ID:                message.ID,
		SpaceID:           message.SpaceID,
		Sender:            message.Sender,
		CreatedAt:         message.CreatedAt,
		ContentType:       message.ContentType,
		ContentJSON:       content,
		RefsJSON:          refs,
		MetaJSON:          metaJSON,
		ContentSchemaJSON: schemaJSON,
	}, nil
}

func (m messageModel) toMessage() (ioa.MessageRecord, error) {
	message := ioa.MessageRecord{
		ID:          m.ID,
		SpaceID:     m.SpaceID,
		Sender:      m.Sender,
		CreatedAt:   m.CreatedAt,
		ContentType: m.ContentType,
		Content:     map[string]interface{}{},
	}
	if err := decodeJSON(m.ContentJSON, &message.Content); err != nil {
		return ioa.MessageRecord{}, err
	}
	if err := decodeJSON(m.RefsJSON, &message.Refs); err != nil {
		return ioa.MessageRecord{}, err
	}
	if message.Refs.Messages == nil {
		message.Refs.Messages = []string{}
	}
	if message.Refs.Nodes == nil {
		message.Refs.Nodes = []string{}
	}
	if strings.TrimSpace(m.MetaJSON) != "" {
		message.Meta = map[string]interface{}{}
		if err := decodeJSON(m.MetaJSON, &message.Meta); err != nil {
			return ioa.MessageRecord{}, err
		}
	}
	if strings.TrimSpace(m.ContentSchemaJSON) != "" {
		message.ContentSchema = map[string]interface{}{}
		if err := decodeJSON(m.ContentSchemaJSON, &message.ContentSchema); err != nil {
			return ioa.MessageRecord{}, err
		}
	}
	return message, nil
}

func decodeMessageModels(models []messageModel) ([]ioa.MessageRecord, error) {
	messages := make([]ioa.MessageRecord, 0, len(models))
	for _, model := range models {
		message, err := model.toMessage()
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func encodeTagsJSON(tags []string) (string, error) {
	tags = ioa.NormalizeTags(tags)
	if len(tags) == 0 {
		return "", nil
	}
	data, err := encodeJSON(tags)
	if err != nil {
		return "", err
	}
	return data, nil
}

func decodeTagsJSON(raw string) ([]string, error) {
	var tags []string
	if err := decodeJSON(raw, &tags); err != nil {
		return nil, err
	}
	return ioa.NormalizeTags(tags), nil
}

func encodeJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeJSON(raw string, value any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), value)
}

func defaultMap(value map[string]interface{}) map[string]interface{} {
	if value != nil {
		return value
	}
	return map[string]interface{}{}
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}
