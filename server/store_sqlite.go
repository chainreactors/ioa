package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chainreactors/ioa/api"
	"github.com/chainreactors/ioa/protocols"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewMemoryStore() *SQLiteStore {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		panic("ioa: memory store: " + err.Error())
	}
	return s
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			meta_json TEXT NOT NULL DEFAULT '{}'
		);
		CREATE TABLE IF NOT EXISTS spaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			tags_json TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS space_nodes (
			space_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (space_id, node_id)
		);
		CREATE TABLE IF NOT EXISTS messages (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			space_id TEXT NOT NULL,
			sender TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT '',
			content_type TEXT NOT NULL DEFAULT '',
			content_json TEXT NOT NULL,
			refs_json TEXT NOT NULL,
			meta_json TEXT,
			content_schema_json TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_messages_space_seq ON messages(space_id, seq);
		CREATE TABLE IF NOT EXISTS tokens (
			hash TEXT PRIMARY KEY,
			node_id TEXT NOT NULL
		);`)
	return err
}

// --- Nodes ---

func (s *SQLiteStore) PutNode(node protocols.Node) error {
	metaJSON, err := toJSON(defaultMeta(node.Meta))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO nodes (id,name,description,meta_json) VALUES (?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,meta_json=excluded.meta_json`,
		node.ID, node.Name, node.Description, metaJSON)
	return err
}

func (s *SQLiteStore) GetNode(nodeID string) (protocols.Node, bool, error) {
	return sqliteScanNode(s.db.QueryRow(
		`SELECT id,name,COALESCE(description,''),meta_json FROM nodes WHERE id=?`, nodeID))
}

func (s *SQLiteStore) GetNodeByName(name string) (protocols.Node, bool, error) {
	return sqliteScanNode(s.db.QueryRow(
		`SELECT id,name,COALESCE(description,''),meta_json FROM nodes WHERE name=?`, name))
}

func (s *SQLiteStore) ListNodes() ([]protocols.Node, error) {
	rows, err := s.db.Query(
		`SELECT id,name,COALESCE(description,''),meta_json FROM nodes ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []protocols.Node
	for rows.Next() {
		var id, name, desc, metaJSON string
		if err := rows.Scan(&id, &name, &desc, &metaJSON); err != nil {
			return nil, err
		}
		meta := map[string]interface{}{}
		sqliteFromJSON(metaJSON, &meta)
		nodes = append(nodes, protocols.Node{ID: id, Name: name, Description: desc, Meta: meta})
	}
	return nodes, rows.Err()
}

// --- Spaces ---

func (s *SQLiteStore) PutSpaceIfAbsent(space protocols.Space) (protocols.Space, error) {
	tagsJSON, err := sqliteTagsToJSON(protocols.NormalizeTags(space.Tags))
	if err != nil {
		return protocols.Space{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return protocols.Space{}, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO spaces (id,name,tags_json) VALUES (?,?,?) ON CONFLICT(name) DO NOTHING`,
		space.ID, space.Name, tagsJSON); err != nil {
		return protocols.Space{}, err
	}

	var id, name, existingTagsJSON string
	if err := tx.QueryRow(`SELECT id,name,COALESCE(tags_json,'') FROM spaces WHERE name=?`, space.Name).
		Scan(&id, &name, &existingTagsJSON); err != nil {
		return protocols.Space{}, err
	}

	existingTags, _ := sqliteTagsFromJSON(existingTagsJSON)
	merged := protocols.MergeTags(existingTags, space.Tags)
	if !slices.Equal(existingTags, merged) {
		newJSON, err := sqliteTagsToJSON(merged)
		if err != nil {
			return protocols.Space{}, err
		}
		if _, err := tx.Exec(`UPDATE spaces SET tags_json=? WHERE id=?`, newJSON, id); err != nil {
			return protocols.Space{}, err
		}
		existingTags = merged
	}

	if err := tx.Commit(); err != nil {
		return protocols.Space{}, err
	}
	return protocols.Space{ID: id, Name: name, Tags: existingTags}, nil
}

func (s *SQLiteStore) GetSpace(spaceID string) (protocols.Space, bool, error) {
	var id, name, tagsJSON string
	err := s.db.QueryRow(`SELECT id,name,COALESCE(tags_json,'') FROM spaces WHERE id=?`, spaceID).
		Scan(&id, &name, &tagsJSON)
	if err == sql.ErrNoRows {
		return protocols.Space{}, false, nil
	}
	if err != nil {
		return protocols.Space{}, false, err
	}
	tags, _ := sqliteTagsFromJSON(tagsJSON)
	return protocols.Space{ID: id, Name: name, Tags: tags}, true, nil
}

func (s *SQLiteStore) ListSpaces() ([]protocols.Space, error) {
	rows, err := s.db.Query(`SELECT id,name,COALESCE(tags_json,'') FROM spaces ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var spaces []protocols.Space
	for rows.Next() {
		var id, name, tagsJSON string
		if err := rows.Scan(&id, &name, &tagsJSON); err != nil {
			return nil, err
		}
		tags, _ := sqliteTagsFromJSON(tagsJSON)
		spaces = append(spaces, protocols.Space{ID: id, Name: name, Tags: tags})
	}
	return spaces, rows.Err()
}

func (s *SQLiteStore) JoinSpace(spaceID, nodeID, description string) error {
	_, err := s.db.Exec(
		`INSERT INTO space_nodes (space_id,node_id,description) VALUES (?,?,?)
		 ON CONFLICT(space_id,node_id) DO UPDATE SET description=excluded.description`,
		spaceID, nodeID, description)
	return err
}

func (s *SQLiteStore) GetSpaceNodes(spaceID string) ([]protocols.Node, error) {
	rows, err := s.db.Query(
		`SELECT n.id,n.name,COALESCE(n.description,''),n.meta_json,sn.description
		 FROM space_nodes sn JOIN nodes n ON sn.node_id=n.id
		 WHERE sn.space_id=? ORDER BY n.name,n.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []protocols.Node
	for rows.Next() {
		var id, name, desc, metaJSON, memberDesc string
		if err := rows.Scan(&id, &name, &desc, &metaJSON, &memberDesc); err != nil {
			return nil, err
		}
		meta := map[string]interface{}{}
		sqliteFromJSON(metaJSON, &meta)
		if memberDesc != "" {
			desc = memberDesc
		}
		nodes = append(nodes, protocols.Node{ID: id, Name: name, Description: desc, Meta: meta})
	}
	return nodes, rows.Err()
}

// --- Messages ---

func (s *SQLiteStore) AppendMessage(msg protocols.Message) error {
	contentJSON, err := toJSON(msg.Content)
	if err != nil {
		return err
	}
	refsJSON, err := toJSON(msg.Refs)
	if err != nil {
		return err
	}
	var metaJSON, schemaJSON string
	if msg.Meta != nil {
		if metaJSON, err = toJSON(msg.Meta); err != nil {
			return err
		}
	}
	if msg.ContentSchema != nil {
		if schemaJSON, err = toJSON(msg.ContentSchema); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(
		`INSERT INTO messages (id,space_id,sender,created_at,content_type,content_json,refs_json,meta_json,content_schema_json)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		msg.ID, msg.SpaceID, msg.Sender, msg.CreatedAt, msg.ContentType,
		contentJSON, refsJSON, nullIfEmpty(metaJSON), nullIfEmpty(schemaJSON))
	return err
}

func (s *SQLiteStore) GetMessage(spaceID, messageID string) (protocols.Message, bool, error) {
	return sqliteScanMessage(s.db.QueryRow(
		`SELECT id,space_id,sender,COALESCE(created_at,''),COALESCE(content_type,''),
		        content_json,refs_json,COALESCE(meta_json,''),COALESCE(content_schema_json,'')
		 FROM messages WHERE space_id=? AND id=?`, spaceID, messageID))
}

func (s *SQLiteStore) GetMessageCount(spaceID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE space_id=?`, spaceID).Scan(&count)
	return count, err
}

func (s *SQLiteStore) GetMessages(spaceID, after string, limit int) ([]protocols.Message, error) {
	all, err := s.spaceMessages(spaceID)
	if err != nil {
		return nil, err
	}
	return WindowMessages(all, all, after, limit), nil
}

func (s *SQLiteStore) GetMessagesForNode(spaceID, nodeID, after string, limit int) ([]protocols.Message, error) {
	all, err := s.spaceMessages(spaceID)
	if err != nil {
		return nil, err
	}
	var filtered []protocols.Message
	for _, m := range all {
		if ContainsString(m.Refs.Nodes, nodeID) {
			filtered = append(filtered, m)
		}
	}
	return WindowMessages(filtered, all, after, limit), nil
}

func (s *SQLiteStore) GetStartMessages(spaceID, after string, limit int) ([]protocols.Message, error) {
	all, err := s.spaceMessages(spaceID)
	if err != nil {
		return nil, err
	}
	var filtered []protocols.Message
	for _, m := range all {
		if len(m.Refs.Messages) == 0 && len(m.Refs.Nodes) == 0 {
			filtered = append(filtered, m)
		}
	}
	return WindowMessages(filtered, all, after, limit), nil
}

func (s *SQLiteStore) GetRelatedMessages(spaceID, messageID, direction, after string, limit int) ([]protocols.Message, error) {
	all, err := s.spaceMessages(spaceID)
	if err != nil {
		return nil, err
	}
	return RelatedMessages(all, messageID, direction, after, limit), nil
}

func (s *SQLiteStore) GetInboxMessages(nodeID, after string, limit int) ([]protocols.Message, error) {
	rows, err := s.db.Query(`SELECT space_id FROM space_nodes WHERE node_id=?`, nodeID)
	if err != nil {
		return nil, err
	}
	var spaceIDs []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			rows.Close()
			return nil, err
		}
		spaceIDs = append(spaceIDs, sid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(spaceIDs)

	var allMessages []protocols.Message
	for _, sid := range spaceIDs {
		msgs, err := s.spaceMessages(sid)
		if err != nil {
			return nil, err
		}
		allMessages = append(allMessages, msgs...)
	}
	var filtered []protocols.Message
	for _, m := range allMessages {
		if ContainsString(m.Refs.Nodes, nodeID) {
			filtered = append(filtered, m)
		}
	}
	return WindowMessages(filtered, allMessages, after, limit), nil
}

func (s *SQLiteStore) ListMessages(filter api.MessageFilter) ([]protocols.Message, error) {
	query := `SELECT id,space_id,sender,COALESCE(created_at,''),COALESCE(content_type,''),
	                 content_json,refs_json,COALESCE(meta_json,''),COALESCE(content_schema_json,'')
	          FROM messages`
	var conds []string
	var args []any
	if filter.SpaceID != "" {
		conds = append(conds, "space_id=?")
		args = append(args, filter.SpaceID)
	}
	if filter.MessageID != "" {
		conds = append(conds, "id=?")
		args = append(args, filter.MessageID)
	}
	if filter.Sender != "" {
		conds = append(conds, "sender=?")
		args = append(args, filter.Sender)
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY seq"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := sqliteScanMessages(rows)
	if err != nil {
		return nil, err
	}
	filtered := FilterMessages(all, filter)
	return WindowMessages(filtered, all, filter.After, filter.Limit), nil
}

// --- Tokens ---

func (s *SQLiteStore) PutToken(tokenHash string, nodeID string) error {
	_, err := s.db.Exec(
		`INSERT INTO tokens (hash,node_id) VALUES (?,?)
		 ON CONFLICT(hash) DO UPDATE SET node_id=excluded.node_id`,
		tokenHash, nodeID)
	return err
}

func (s *SQLiteStore) GetNodeByTokenHash(tokenHash string) (protocols.Node, bool, error) {
	var nodeID string
	err := s.db.QueryRow(`SELECT node_id FROM tokens WHERE hash=?`, tokenHash).Scan(&nodeID)
	if err == sql.ErrNoRows {
		return protocols.Node{}, false, nil
	}
	if err != nil {
		return protocols.Node{}, false, err
	}
	return s.GetNode(nodeID)
}

// --- internal helpers ---

func (s *SQLiteStore) spaceMessages(spaceID string) ([]protocols.Message, error) {
	rows, err := s.db.Query(
		`SELECT id,space_id,sender,COALESCE(created_at,''),COALESCE(content_type,''),
		        content_json,refs_json,COALESCE(meta_json,''),COALESCE(content_schema_json,'')
		 FROM messages WHERE space_id=? ORDER BY seq`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sqliteScanMessages(rows)
}

func sqliteScanNode(row *sql.Row) (protocols.Node, bool, error) {
	var id, name, desc, metaJSON string
	if err := row.Scan(&id, &name, &desc, &metaJSON); err != nil {
		if err == sql.ErrNoRows {
			return protocols.Node{}, false, nil
		}
		return protocols.Node{}, false, err
	}
	meta := map[string]interface{}{}
	sqliteFromJSON(metaJSON, &meta)
	return protocols.Node{ID: id, Name: name, Description: desc, Meta: meta}, true, nil
}

func sqliteScanMessage(row *sql.Row) (protocols.Message, bool, error) {
	var id, spaceID, sender, createdAt, contentType, contentJSON, refsJSON, metaJSON, schemaJSON string
	if err := row.Scan(&id, &spaceID, &sender, &createdAt, &contentType, &contentJSON, &refsJSON, &metaJSON, &schemaJSON); err != nil {
		if err == sql.ErrNoRows {
			return protocols.Message{}, false, nil
		}
		return protocols.Message{}, false, err
	}
	return sqliteBuildMessage(id, spaceID, sender, createdAt, contentType, contentJSON, refsJSON, metaJSON, schemaJSON)
}

func sqliteScanMessages(rows *sql.Rows) ([]protocols.Message, error) {
	var messages []protocols.Message
	for rows.Next() {
		var id, spaceID, sender, createdAt, contentType, contentJSON, refsJSON, metaJSON, schemaJSON string
		if err := rows.Scan(&id, &spaceID, &sender, &createdAt, &contentType, &contentJSON, &refsJSON, &metaJSON, &schemaJSON); err != nil {
			return nil, err
		}
		msg, _, err := sqliteBuildMessage(id, spaceID, sender, createdAt, contentType, contentJSON, refsJSON, metaJSON, schemaJSON)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func sqliteBuildMessage(id, spaceID, sender, createdAt, contentType, contentJSON, refsJSON, metaJSON, schemaJSON string) (protocols.Message, bool, error) {
	msg := protocols.Message{
		ID: id, SpaceID: spaceID, Sender: sender,
		CreatedAt: createdAt, ContentType: contentType,
		Content: map[string]interface{}{},
	}
	if err := sqliteFromJSON(contentJSON, &msg.Content); err != nil {
		return protocols.Message{}, false, err
	}
	if err := sqliteFromJSON(refsJSON, &msg.Refs); err != nil {
		return protocols.Message{}, false, err
	}
	if msg.Refs.Messages == nil {
		msg.Refs.Messages = []string{}
	}
	if msg.Refs.Nodes == nil {
		msg.Refs.Nodes = []string{}
	}
	if strings.TrimSpace(metaJSON) != "" {
		msg.Meta = map[string]interface{}{}
		sqliteFromJSON(metaJSON, &msg.Meta)
	}
	if strings.TrimSpace(schemaJSON) != "" {
		msg.ContentSchema = map[string]interface{}{}
		sqliteFromJSON(schemaJSON, &msg.ContentSchema)
	}
	return msg, true, nil
}

func toJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	return string(data), err
}

func sqliteFromJSON(raw string, v any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), v)
}

func sqliteTagsToJSON(tags []string) (string, error) {
	tags = protocols.NormalizeTags(tags)
	if len(tags) == 0 {
		return "", nil
	}
	return toJSON(tags)
}

func sqliteTagsFromJSON(raw string) ([]string, error) {
	var tags []string
	if err := sqliteFromJSON(raw, &tags); err != nil {
		return nil, err
	}
	return protocols.NormalizeTags(tags), nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
