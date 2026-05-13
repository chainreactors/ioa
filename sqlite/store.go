//go:build sqlite

package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chainreactors/ioa"
	"github.com/chainreactors/ioa/server"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) initSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS nodes (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	meta_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS spaces (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS space_nodes (
	space_id TEXT NOT NULL,
	node_id TEXT NOT NULL,
	description TEXT NOT NULL,
	PRIMARY KEY (space_id, node_id)
);
CREATE TABLE IF NOT EXISTS messages (
	seq INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT NOT NULL UNIQUE,
	space_id TEXT NOT NULL,
	sender TEXT NOT NULL,
	content_json TEXT NOT NULL,
	refs_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_space_seq ON messages (space_id, seq);
`)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`ALTER TABLE spaces ADD COLUMN content_schema_json TEXT`)
	return nil
}

func (s *SQLiteStore) PutNode(node ioa.Node) error {
	meta, err := json.Marshal(node.Meta)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO nodes (id, name, meta_json)
VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	meta_json = excluded.meta_json
`, node.ID, node.Name, string(meta))
	return err
}

func (s *SQLiteStore) GetNode(nodeID string) (ioa.Node, bool, error) {
	row := s.db.QueryRow(`SELECT id, name, meta_json FROM nodes WHERE id = ?`, nodeID)
	return scanNode(row)
}

func (s *SQLiteStore) ListNodes() ([]ioa.Node, error) {
	rows, err := s.db.Query(`SELECT id, name, meta_json FROM nodes ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ioa.Node
	for rows.Next() {
		var node ioa.Node
		var metaJSON string
		if err := rows.Scan(&node.ID, &node.Name, &metaJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(metaJSON), &node.Meta); err != nil {
			return nil, err
		}
		result = append(result, node)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) ListSpaces() ([]ioa.Space, error) {
	rows, err := s.db.Query(`SELECT id, name FROM spaces ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ioa.Space
	for rows.Next() {
		var space ioa.Space
		if err := rows.Scan(&space.ID, &space.Name); err != nil {
			return nil, err
		}
		result = append(result, space)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) PutSpaceIfAbsent(space ioa.Space) (ioa.Space, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return ioa.Space{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO spaces (id, name) VALUES (?, ?)`, space.ID, space.Name); err != nil {
		return ioa.Space{}, err
	}
	row := tx.QueryRow(`SELECT id, name FROM spaces WHERE name = ?`, space.Name)
	existing, ok, err := scanSpace(row)
	if err != nil {
		return ioa.Space{}, err
	}
	if !ok {
		return ioa.Space{}, fmt.Errorf("space %q was not created", space.Name)
	}
	if err := tx.Commit(); err != nil {
		return ioa.Space{}, err
	}
	return existing, nil
}

func (s *SQLiteStore) GetSpace(spaceID string) (ioa.Space, bool, error) {
	row := s.db.QueryRow(`SELECT id, name FROM spaces WHERE id = ?`, spaceID)
	return scanSpace(row)
}

func (s *SQLiteStore) SetContentSchema(spaceID string, schema map[string]interface{}) error {
	if schema == nil {
		_, err := s.db.Exec(`UPDATE spaces SET content_schema_json = NULL WHERE id = ?`, spaceID)
		return err
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE spaces SET content_schema_json = ? WHERE id = ?`, string(data), spaceID)
	return err
}

func (s *SQLiteStore) GetContentSchema(spaceID string) (map[string]interface{}, error) {
	var raw sql.NullString
	err := s.db.QueryRow(`SELECT content_schema_json FROM spaces WHERE id = ?`, spaceID).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if !raw.Valid {
		return nil, nil
	}
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(raw.String), &schema); err != nil {
		return nil, err
	}
	return schema, nil
}

func (s *SQLiteStore) JoinSpace(spaceID, nodeID, description string) error {
	_, err := s.db.Exec(`
INSERT INTO space_nodes (space_id, node_id, description)
VALUES (?, ?, ?)
ON CONFLICT(space_id, node_id) DO UPDATE SET
	description = excluded.description
`, spaceID, nodeID, description)
	return err
}

func (s *SQLiteStore) GetSpaceNodes(spaceID string) ([]ioa.SpaceNodeRecord, error) {
	rows, err := s.db.Query(`
SELECT n.id, n.name, n.meta_json, sn.description
FROM space_nodes sn
JOIN nodes n ON n.id = sn.node_id
WHERE sn.space_id = ?
ORDER BY n.name, n.id
`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ioa.SpaceNodeRecord
	for rows.Next() {
		var node ioa.Node
		var metaJSON string
		var description string
		if err := rows.Scan(&node.ID, &node.Name, &metaJSON, &description); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(metaJSON), &node.Meta); err != nil {
			return nil, err
		}
		result = append(result, ioa.SpaceNodeRecord{Node: node, Description: description})
	}
	return result, rows.Err()
}

func (s *SQLiteStore) AppendMessage(message ioa.MessageRecord) error {
	content, err := json.Marshal(message.Content)
	if err != nil {
		return err
	}
	refs, err := json.Marshal(message.Refs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO messages (id, space_id, sender, content_json, refs_json)
VALUES (?, ?, ?, ?, ?)
`, message.ID, message.SpaceID, message.Sender, string(content), string(refs))
	return err
}

func (s *SQLiteStore) GetMessage(spaceID, messageID string) (ioa.MessageRecord, bool, error) {
	row := s.db.QueryRow(`
SELECT id, space_id, sender, content_json, refs_json
FROM messages
WHERE space_id = ? AND id = ?
`, spaceID, messageID)
	return scanMessage(row)
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
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE space_id = ?`, spaceID).Scan(&count)
	return count, err
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

func (s *SQLiteStore) allMessages(spaceID string) ([]ioa.MessageRecord, error) {
	rows, err := s.db.Query(`
SELECT id, space_id, sender, content_json, refs_json
FROM messages
WHERE space_id = ?
ORDER BY seq ASC
`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ioa.MessageRecord
	for rows.Next() {
		message, err := scanMessageRow(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanNode(row rowScanner) (ioa.Node, bool, error) {
	var node ioa.Node
	var metaJSON string
	if err := row.Scan(&node.ID, &node.Name, &metaJSON); err != nil {
		if err == sql.ErrNoRows {
			return ioa.Node{}, false, nil
		}
		return ioa.Node{}, false, err
	}
	if err := json.Unmarshal([]byte(metaJSON), &node.Meta); err != nil {
		return ioa.Node{}, false, err
	}
	return node, true, nil
}

func scanSpace(row rowScanner) (ioa.Space, bool, error) {
	var space ioa.Space
	if err := row.Scan(&space.ID, &space.Name); err != nil {
		if err == sql.ErrNoRows {
			return ioa.Space{}, false, nil
		}
		return ioa.Space{}, false, err
	}
	return space, true, nil
}

func scanMessage(row rowScanner) (ioa.MessageRecord, bool, error) {
	message, err := scanMessageRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ioa.MessageRecord{}, false, nil
		}
		return ioa.MessageRecord{}, false, err
	}
	return message, true, nil
}

func scanMessageRow(row rowScanner) (ioa.MessageRecord, error) {
	var message ioa.MessageRecord
	var contentJSON string
	var refsJSON string
	if err := row.Scan(&message.ID, &message.SpaceID, &message.Sender, &contentJSON, &refsJSON); err != nil {
		return ioa.MessageRecord{}, err
	}
	if err := json.Unmarshal([]byte(contentJSON), &message.Content); err != nil {
		return ioa.MessageRecord{}, err
	}
	if err := json.Unmarshal([]byte(refsJSON), &message.Refs); err != nil {
		return ioa.MessageRecord{}, err
	}
	return message, nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}
