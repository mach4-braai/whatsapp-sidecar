package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    is_group INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    sender TEXT NOT NULL,
    content TEXT NOT NULL,
    is_from_me INTEGER NOT NULL DEFAULT 0,
    timestamp DATETIME NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id)
);

CREATE TABLE IF NOT EXISTS summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id)
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_summaries_conversation ON summaries(conversation_id, created_at);
`
	_, err := s.db.ExecContext(ctx, ddl)
	return err
}

func (s *Store) UpsertConversation(ctx context.Context, c Conversation) error {
	const q = `
INSERT INTO conversations (id, name, is_group, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
    is_group = excluded.is_group,
    updated_at = excluded.updated_at
`
	_, err := s.db.ExecContext(ctx, q, c.ID, c.Name, c.IsGroup, c.UpdatedAt)
	return err
}

func (s *Store) ListConversations(ctx context.Context) ([]Conversation, error) {
	const q = `SELECT id, name, is_group, updated_at FROM conversations ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convos []Conversation
	for rows.Next() {
		var c Conversation
		var isGroup int
		if err := rows.Scan(&c.ID, &c.Name, &isGroup, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.IsGroup = isGroup != 0
		convos = append(convos, c)
	}
	return convos, rows.Err()
}

func (s *Store) InsertMessage(ctx context.Context, m Message) error {
	const q = `
INSERT OR IGNORE INTO messages (id, conversation_id, sender, content, is_from_me, timestamp)
VALUES (?, ?, ?, ?, ?, ?)
`
	_, err := s.db.ExecContext(ctx, q, m.ID, m.ConversationID, m.Sender, m.Content, m.IsFromMe, m.Timestamp)
	return err
}

func (s *Store) GetMessages(ctx context.Context, conversationID string, limit int) ([]Message, error) {
	const q = `
SELECT id, conversation_id, sender, content, is_from_me, timestamp
FROM messages
WHERE conversation_id = ?
ORDER BY timestamp DESC
LIMIT ?
`
	rows, err := s.db.QueryContext(ctx, q, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		var isFromMe int
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Sender, &m.Content, &isFromMe, &m.Timestamp); err != nil {
			return nil, err
		}
		m.IsFromMe = isFromMe != 0
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *Store) InsertSummary(ctx context.Context, conversationID, content string) (int, error) {
	const q = `INSERT INTO summaries (conversation_id, content) VALUES (?, ?)`
	res, err := s.db.ExecContext(ctx, q, conversationID, content)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (s *Store) GetLatestSummary(ctx context.Context, conversationID string) (*Summary, error) {
	const q = `
SELECT id, conversation_id, content, created_at
FROM summaries
WHERE conversation_id = ?
ORDER BY created_at DESC
LIMIT 1
`
	var sum Summary
	err := s.db.QueryRowContext(ctx, q, conversationID).Scan(&sum.ID, &sum.ConversationID, &sum.Content, &sum.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sum, nil
}

func (s *Store) GetConversationsWithNewMessages(ctx context.Context) ([]string, error) {
	const q = `
SELECT DISTINCT m.conversation_id
FROM messages m
LEFT JOIN (
    SELECT conversation_id, MAX(created_at) AS latest
    FROM summaries
    GROUP BY conversation_id
) s ON m.conversation_id = s.conversation_id
WHERE s.latest IS NULL OR m.timestamp > s.latest
`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

