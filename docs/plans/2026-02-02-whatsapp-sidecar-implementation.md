# WhatsApp Sidecar Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a local sidecar app that mirrors WhatsApp conversations in a web UI with AI-generated summaries.

**Architecture:** Go backend connects to WhatsApp via `whatsmeow`, stores messages in SQLite, runs periodic summarization against Ollama (GLM-4), and serves a REST API. Next.js frontend consumes the API.

**Tech Stack:** Go 1.23, `go.mau.fi/whatsmeow`, `modernc.org/sqlite`, Next.js 15 (App Router), Tailwind CSS, mise

---

## Task 1: Project Scaffolding and Mise Config

**Files:**
- Create: `mise.toml`
- Create: `backend/go.mod`
- Create: `backend/cmd/server/main.go`
- Create: `.gitignore`

**Step 1: Initialize git repo**

Run: `git init`

**Step 2: Create `.gitignore`**

```gitignore
# Go
backend/bin/
backend/vendor/

# Node
ui/node_modules/
ui/.next/
ui/out/

# SQLite
*.db
*.db-wal
*.db-shm

# WhatsApp session
backend/whatsapp-session.db

# OS
.DS_Store

# Env
.env
.env.local
```

**Step 3: Create `mise.toml`**

```toml
[tools]
go = "1.23"
node = "22"

[tasks.backend]
description = "Run the Go backend"
run = "cd backend && go run ./cmd/server"

[tasks.ui]
description = "Run the Next.js frontend"
run = "cd ui && npm run dev"

[tasks.dev]
description = "Run both backend and UI"
depends = ["backend", "ui"]

[tasks.setup]
description = "Install all dependencies"
run = """
cd backend && go mod download
cd ../ui && npm install
"""

[tasks.build]
description = "Build both services"
run = """
cd backend && go build -o bin/server ./cmd/server
cd ../ui && npm run build
"""

[tasks."test:backend"]
description = "Run backend tests"
run = "cd backend && go test ./..."

[tasks."test:ui"]
description = "Run frontend tests"
run = "cd ui && npm test"
```

**Step 4: Initialize Go module**

Run: `cd backend && go mod init github.com/devanmcgeer/whatsapp-sidecar/backend`

**Step 5: Create minimal `backend/cmd/server/main.go`**

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("whatsapp-sidecar server starting...")
	os.Exit(0)
}
```

**Step 6: Verify it builds**

Run: `cd backend && go build ./cmd/server`
Expected: No errors

**Step 7: Commit**

```bash
git add .gitignore mise.toml backend/go.mod backend/go.sum backend/cmd/server/main.go
git commit -m "Scaffold project with mise and Go module"
```

---

## Task 2: SQLite Store — Schema and Connection

**Files:**
- Create: `backend/internal/store/store.go`
- Create: `backend/internal/store/store_test.go`

**Step 1: Write the failing test**

```go
// backend/internal/store/store_test.go
package store

import (
	"context"
	"testing"
)

func TestNewStore(t *testing.T) {
	ctx := context.Background()
	s, err := New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer s.Close()

	tables := []string{"conversations", "messages", "summaries"}
	for _, table := range tables {
		_, err := s.db.ExecContext(ctx, "SELECT 1 FROM "+table+" LIMIT 1")
		if err != nil {
			t.Errorf("table %q does not exist: %v", table, err)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/store/ -v`
Expected: FAIL — package does not exist

**Step 3: Install dependency**

Run: `cd backend && go get modernc.org/sqlite`

**Step 4: Write minimal implementation**

```go
// backend/internal/store/store.go
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
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := migrate(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(ctx context.Context, db *sql.DB) error {
	schema := `
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

	CREATE INDEX IF NOT EXISTS idx_messages_conversation
		ON messages(conversation_id, timestamp);

	CREATE INDEX IF NOT EXISTS idx_summaries_conversation
		ON summaries(conversation_id, created_at);
	`
	_, err := db.ExecContext(ctx, schema)
	return err
}
```

**Step 5: Run test to verify it passes**

Run: `cd backend && go test ./internal/store/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add backend/internal/store/ backend/go.mod backend/go.sum
git commit -m "Add SQLite store with schema migration"
```

---

## Task 3: Store — CRUD Operations

**Files:**
- Modify: `backend/internal/store/store.go`
- Create: `backend/internal/store/models.go`
- Modify: `backend/internal/store/store_test.go`

**Step 1: Create models**

```go
// backend/internal/store/models.go
package store

import "time"

type Conversation struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IsGroup   bool      `json:"is_group"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Sender         string    `json:"sender"`
	Content        string    `json:"content"`
	IsFromMe       bool      `json:"is_from_me"`
	Timestamp      time.Time `json:"timestamp"`
}

type Summary struct {
	ID             int       `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}
```

**Step 2: Write failing tests for CRUD**

```go
// append to backend/internal/store/store_test.go

func TestUpsertAndListConversations(t *testing.T) {
	ctx := context.Background()
	s, err := New(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	err = s.UpsertConversation(ctx, Conversation{
		ID:   "123@s.whatsapp.net",
		Name: "Alice",
	})
	if err != nil {
		t.Fatalf("UpsertConversation: %v", err)
	}

	convos, err := s.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convos) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(convos))
	}
	if convos[0].Name != "Alice" {
		t.Errorf("expected name Alice, got %s", convos[0].Name)
	}
}

func TestInsertAndGetMessages(t *testing.T) {
	ctx := context.Background()
	s, err := New(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.UpsertConversation(ctx, Conversation{ID: "c1", Name: "Bob"})

	msg := Message{
		ID:             "msg1",
		ConversationID: "c1",
		Sender:         "Bob",
		Content:        "Hello there",
		Timestamp:      time.Now(),
	}
	err = s.InsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	msgs, err := s.GetMessages(ctx, "c1", 50)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Hello there" {
		t.Errorf("expected content 'Hello there', got %s", msgs[0].Content)
	}
}

func TestInsertAndGetSummary(t *testing.T) {
	ctx := context.Background()
	s, err := New(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.UpsertConversation(ctx, Conversation{ID: "c1", Name: "Bob"})

	_, err = s.InsertSummary(ctx, "c1", "Bob mentioned a meeting on Friday")
	if err != nil {
		t.Fatalf("InsertSummary: %v", err)
	}

	summary, err := s.GetLatestSummary(ctx, "c1")
	if err != nil {
		t.Fatalf("GetLatestSummary: %v", err)
	}
	if summary.Content != "Bob mentioned a meeting on Friday" {
		t.Errorf("unexpected summary: %s", summary.Content)
	}
}

func TestGetConversationsWithNewMessages(t *testing.T) {
	ctx := context.Background()
	s, err := New(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.UpsertConversation(ctx, Conversation{ID: "c1", Name: "Alice"})
	_ = s.UpsertConversation(ctx, Conversation{ID: "c2", Name: "Bob"})

	_ = s.InsertMessage(ctx, Message{
		ID: "m1", ConversationID: "c1", Sender: "Alice",
		Content: "Hey", Timestamp: time.Now(),
	})

	ids, err := s.GetConversationsWithNewMessages(ctx)
	if err != nil {
		t.Fatalf("GetConversationsWithNewMessages: %v", err)
	}
	if len(ids) != 1 || ids[0] != "c1" {
		t.Errorf("expected [c1], got %v", ids)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `cd backend && go test ./internal/store/ -v`
Expected: FAIL — methods do not exist

**Step 4: Implement CRUD methods**

Add to `backend/internal/store/store.go`:

```go
func (s *Store) UpsertConversation(ctx context.Context, c Conversation) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO conversations (id, name, is_group, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, updated_at=CURRENT_TIMESTAMP`,
		c.ID, c.Name, c.IsGroup,
	)
	return err
}

func (s *Store) ListConversations(ctx context.Context) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, is_group, updated_at FROM conversations ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convos []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Name, &c.IsGroup, &c.UpdatedAt); err != nil {
			return nil, err
		}
		convos = append(convos, c)
	}
	return convos, rows.Err()
}

func (s *Store) InsertMessage(ctx context.Context, m Message) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO messages (id, conversation_id, sender, content, is_from_me, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.ConversationID, m.Sender, m.Content, m.IsFromMe, m.Timestamp,
	)
	return err
}

func (s *Store) GetMessages(ctx context.Context, conversationID string, limit int) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, sender, content, is_from_me, timestamp
		 FROM messages WHERE conversation_id = ?
		 ORDER BY timestamp DESC LIMIT ?`,
		conversationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Sender, &m.Content, &m.IsFromMe, &m.Timestamp); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *Store) InsertSummary(ctx context.Context, conversationID, content string) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO summaries (conversation_id, content) VALUES (?, ?)`,
		conversationID, content,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (s *Store) GetLatestSummary(ctx context.Context, conversationID string) (*Summary, error) {
	var sum Summary
	err := s.db.QueryRowContext(ctx,
		`SELECT id, conversation_id, content, created_at FROM summaries
		 WHERE conversation_id = ? ORDER BY created_at DESC LIMIT 1`,
		conversationID,
	).Scan(&sum.ID, &sum.ConversationID, &sum.Content, &sum.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &sum, nil
}

func (s *Store) GetConversationsWithNewMessages(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT m.conversation_id FROM messages m
		 LEFT JOIN summaries s ON m.conversation_id = s.conversation_id
		 WHERE s.id IS NULL
		    OR m.timestamp > (
		        SELECT MAX(created_at) FROM summaries
		        WHERE conversation_id = m.conversation_id
		    )`)
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
```

**Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/store/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add backend/internal/store/
git commit -m "Add store CRUD operations for all tables"
```

---

## Task 4: WhatsApp Client Wrapper

**Files:**
- Create: `backend/internal/whatsapp/client.go`

Note: This package wraps `whatsmeow` which requires a real WhatsApp
connection. No unit tests — the event handler logic is thin glue code.

**Step 1: Install dependency**

Run: `cd backend && go get go.mau.fi/whatsmeow`

**Step 2: Create the WhatsApp client wrapper**

```go
// backend/internal/whatsapp/client.go
package whatsapp

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/store"

	_ "modernc.org/sqlite"
)

type Client struct {
	wac   *whatsmeow.Client
	store *store.Store
}

func New(ctx context.Context, sessionDBPath string, appStore *store.Store) (*Client, error) {
	dbLog := waLog.Stdout("WhatsApp-DB", "WARN", true)
	container, err := sqlstore.New(ctx, "sqlite", fmt.Sprintf("file:%s?_foreign_keys=on", sessionDBPath), dbLog)
	if err != nil {
		return nil, fmt.Errorf("create session store: %w", err)
	}

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	clientLog := waLog.Stdout("WhatsApp", "WARN", true)
	wac := whatsmeow.NewClient(device, clientLog)

	c := &Client{wac: wac, store: appStore}
	wac.AddEventHandler(c.handleEvent)

	return c, nil
}

func (c *Client) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.wac.GetQRChannel(ctx)
}

func (c *Client) IsLoggedIn() bool {
	return c.wac.Store.ID != nil
}

func (c *Client) Connect() error {
	return c.wac.Connect()
}

func (c *Client) Disconnect() {
	c.wac.Disconnect()
}

func (c *Client) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		c.handleMessage(v)
	}
}

func (c *Client) handleMessage(msg *events.Message) {
	ctx := context.Background()

	conversationJID := msg.Info.Chat.String()
	senderJID := msg.Info.Sender.String()

	name := conversationJID
	if msg.Info.IsGroup {
		groupInfo, err := c.wac.GetGroupInfo(msg.Info.Chat)
		if err == nil {
			name = groupInfo.Name
		}
	} else {
		name = c.contactName(msg.Info.Chat)
	}

	_ = c.store.UpsertConversation(ctx, store.Conversation{
		ID:      conversationJID,
		Name:    name,
		IsGroup: msg.Info.IsGroup,
	})

	content := extractText(msg)
	if content == "" {
		return
	}

	_ = c.store.InsertMessage(ctx, store.Message{
		ID:             msg.Info.ID,
		ConversationID: conversationJID,
		Sender:         senderJID,
		Content:        content,
		IsFromMe:       msg.Info.IsFromMe,
		Timestamp:      msg.Info.Timestamp,
	})
}

func (c *Client) contactName(jid types.JID) string {
	contact, err := c.wac.Store.Contacts.GetContact(jid)
	if err == nil && contact.FullName != "" {
		return contact.FullName
	}
	if err == nil && contact.PushName != "" {
		return contact.PushName
	}
	return jid.User
}

func extractText(msg *events.Message) string {
	if msg.Message == nil {
		return ""
	}
	if conv := msg.Message.GetConversation(); conv != "" {
		return conv
	}
	if ext := msg.Message.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	return ""
}
```

**Step 3: Verify it compiles**

Run: `cd backend && go build ./internal/whatsapp/`
Expected: No errors

**Step 4: Commit**

```bash
git add backend/internal/whatsapp/ backend/go.mod backend/go.sum
git commit -m "Add whatsmeow client wrapper with message handling"
```

---

## Task 5: Ollama Summarizer

**Files:**
- Create: `backend/internal/summarizer/summarizer.go`
- Create: `backend/internal/summarizer/summarizer_test.go`

**Step 1: Write failing test**

```go
// backend/internal/summarizer/summarizer_test.go
package summarizer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/store"
)

func TestSummarize(t *testing.T) {
	ctx := context.Background()
	s, err := store.New(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_ = s.UpsertConversation(ctx, store.Conversation{ID: "c1", Name: "Alice"})
	_ = s.InsertMessage(ctx, store.Message{
		ID: "m1", ConversationID: "c1", Sender: "Alice",
		Content: "Meeting moved to Friday at 3pm", Timestamp: time.Now(),
	})
	_ = s.InsertMessage(ctx, store.Message{
		ID: "m2", ConversationID: "c1", Sender: "me",
		Content: "Got it, thanks", IsFromMe: true, Timestamp: time.Now(),
	})

	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"response": "Alice moved the meeting to Friday 3pm. You acknowledged.",
			"done":     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockOllama.Close()

	sum := New(s, mockOllama.URL, "glm4")

	err = sum.SummarizeConversation(ctx, "c1")
	if err != nil {
		t.Fatalf("SummarizeConversation: %v", err)
	}

	summary, err := s.GetLatestSummary(ctx, "c1")
	if err != nil {
		t.Fatalf("GetLatestSummary: %v", err)
	}
	if summary.Content == "" {
		t.Error("expected non-empty summary")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/summarizer/ -v`
Expected: FAIL — package does not exist

**Step 3: Implement summarizer**

```go
// backend/internal/summarizer/summarizer.go
package summarizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/store"
)

type Summarizer struct {
	store     *store.Store
	ollamaURL string
	model     string
}

func New(s *store.Store, ollamaURL, model string) *Summarizer {
	return &Summarizer{store: s, ollamaURL: ollamaURL, model: model}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (s *Summarizer) SummarizeConversation(ctx context.Context, conversationID string) error {
	msgs, err := s.store.GetMessages(ctx, conversationID, 100)
	if err != nil {
		return fmt.Errorf("get messages: %w", err)
	}
	if len(msgs) == 0 {
		return nil
	}

	prompt := buildPrompt(msgs)

	summary, err := s.generate(ctx, prompt)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	_, err = s.store.InsertSummary(ctx, conversationID, summary)
	if err != nil {
		return fmt.Errorf("insert summary: %w", err)
	}

	return nil
}

func (s *Summarizer) RunPeriodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runAll(ctx)
		}
	}
}

func (s *Summarizer) runAll(ctx context.Context) {
	ids, err := s.store.GetConversationsWithNewMessages(ctx)
	if err != nil {
		return
	}
	for _, id := range ids {
		_ = s.SummarizeConversation(ctx, id)
	}
}

func (s *Summarizer) generate(ctx context.Context, prompt string) (string, error) {
	reqBody, _ := json.Marshal(ollamaRequest{
		Model:  s.model,
		Prompt: prompt,
		Stream: false,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ollamaURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}

	return ollamaResp.Response, nil
}

func buildPrompt(msgs []store.Message) string {
	var b strings.Builder
	b.WriteString(`You are a conversation summarizer. Given the following WhatsApp messages, write a concise summary of notable events, decisions, and action items. Focus on what matters — skip small talk.

Messages:
`)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		sender := m.Sender
		if m.IsFromMe {
			sender = "me"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", m.Timestamp.Format("15:04"), sender, m.Content)
	}
	return b.String()
}
```

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/summarizer/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/summarizer/ backend/go.mod backend/go.sum
git commit -m "Add Ollama summarizer with periodic scheduling"
```

---

## Task 6: REST API

**Files:**
- Create: `backend/internal/api/api.go`
- Create: `backend/internal/api/api_test.go`

**Step 1: Write failing tests**

```go
// backend/internal/api/api_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/store"
)

func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.New(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	_ = s.UpsertConversation(ctx, store.Conversation{ID: "c1", Name: "Alice"})
	_ = s.InsertMessage(ctx, store.Message{
		ID: "m1", ConversationID: "c1", Sender: "Alice",
		Content: "Hello", Timestamp: time.Now(),
	})
	_, _ = s.InsertSummary(ctx, "c1", "Alice said hello")

	return s
}

func TestListConversations(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	handler := New(s, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var convos []store.Conversation
	if err := json.NewDecoder(w.Body).Decode(&convos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(convos) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(convos))
	}
}

func TestGetConversation(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	handler := New(s, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/conversations/c1", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result ConversationDetail
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Summary == nil {
		t.Error("expected summary, got nil")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/api/ -v`
Expected: FAIL — package does not exist

**Step 3: Implement API handler**

```go
// backend/internal/api/api.go
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/store"
	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/summarizer"
)

type ConversationDetail struct {
	Conversation store.Conversation `json:"conversation"`
	Messages     []store.Message    `json:"messages"`
	Summary      *store.Summary     `json:"summary"`
}

type Handler struct {
	mux        *http.ServeMux
	store      *store.Store
	summarizer *summarizer.Summarizer
	qrCode     string
	qrMu       sync.Mutex
}

func New(s *store.Store, sum *summarizer.Summarizer) *Handler {
	h := &Handler{
		mux:        http.NewServeMux(),
		store:      s,
		summarizer: sum,
	}

	h.mux.HandleFunc("GET /api/conversations", h.listConversations)
	h.mux.HandleFunc("GET /api/conversations/", h.getConversation)
	h.mux.HandleFunc("GET /api/pairing", h.getPairing)
	h.mux.HandleFunc("POST /api/summarize/", h.triggerSummarize)

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	h.mux.ServeHTTP(w, r)
}

func (h *Handler) SetQRCode(code string) {
	h.qrMu.Lock()
	defer h.qrMu.Unlock()
	h.qrCode = code
}

func (h *Handler) getPairing(w http.ResponseWriter, r *http.Request) {
	h.qrMu.Lock()
	code := h.qrCode
	h.qrMu.Unlock()
	json.NewEncoder(w).Encode(map[string]string{"qr_code": code})
}

func (h *Handler) listConversations(w http.ResponseWriter, r *http.Request) {
	convos, err := h.store.ListConversations(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if convos == nil {
		convos = []store.Conversation{}
	}
	json.NewEncoder(w).Encode(convos)
}

func (h *Handler) getConversation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/conversations/")
	if id == "" {
		http.Error(w, "missing conversation id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	msgs, err := h.store.GetMessages(ctx, id, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []store.Message{}
	}

	summary, _ := h.store.GetLatestSummary(ctx, id)

	convos, _ := h.store.ListConversations(ctx)
	var convo store.Conversation
	for _, c := range convos {
		if c.ID == id {
			convo = c
			break
		}
	}

	json.NewEncoder(w).Encode(ConversationDetail{
		Conversation: convo,
		Messages:     msgs,
		Summary:      summary,
	})
}

func (h *Handler) triggerSummarize(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/summarize/")
	if id == "" {
		http.Error(w, "missing conversation id", http.StatusBadRequest)
		return
	}

	if h.summarizer == nil {
		http.Error(w, "summarizer not configured", http.StatusServiceUnavailable)
		return
	}

	if err := h.summarizer.SummarizeConversation(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

**Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/api/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/api/
git commit -m "Add REST API with conversation and summary endpoints"
```

---

## Task 7: Wire Up `main.go`

**Files:**
- Modify: `backend/cmd/server/main.go`

**Step 1: Update `main.go` to wire all components**

```go
// backend/cmd/server/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/api"
	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/store"
	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/summarizer"
	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/whatsapp"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	ollamaURL := envOr("OLLAMA_URL", "http://localhost:11434")
	ollamaModel := envOr("OLLAMA_MODEL", "glm4")
	listenAddr := envOr("LISTEN_ADDR", "localhost:8080")
	dbPath := envOr("DB_PATH", "whatsapp-sidecar.db")
	sessionDBPath := envOr("SESSION_DB_PATH", "whatsapp-session.db")
	summaryInterval := envOr("SUMMARY_INTERVAL", "6h")

	interval, err := time.ParseDuration(summaryInterval)
	if err != nil {
		log.Fatalf("invalid SUMMARY_INTERVAL: %v", err)
	}

	s, err := store.New(ctx, dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	wac, err := whatsapp.New(ctx, sessionDBPath, s)
	if err != nil {
		log.Fatalf("whatsapp: %v", err)
	}

	if !wac.IsLoggedIn() {
		qrChan, err := wac.GetQRChannel(ctx)
		if err != nil {
			log.Fatalf("qr channel: %v", err)
		}
		if err := wac.Connect(); err != nil {
			log.Fatalf("connect: %v", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Printf("Scan this QR code in WhatsApp:\n%s\n", evt.Code)
			} else {
				fmt.Printf("QR event: %s\n", evt.Event)
				break
			}
		}
	} else {
		if err := wac.Connect(); err != nil {
			log.Fatalf("connect: %v", err)
		}
	}
	defer wac.Disconnect()

	sum := summarizer.New(s, ollamaURL, ollamaModel)
	go sum.RunPeriodic(ctx, interval)

	handler := api.New(s, sum)

	srv := &http.Server{Addr: listenAddr, Handler: handler}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s", listenAddr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

**Step 2: Verify it compiles**

Run: `cd backend && go build ./cmd/server/`
Expected: No errors

**Step 3: Commit**

```bash
git add backend/cmd/server/main.go
git commit -m "Wire up main.go with all backend components"
```

---

## Task 8: Next.js Frontend Scaffold

**Files:**
- Create: `ui/` (via `create-next-app`)

**Step 1: Scaffold Next.js project**

Run: `cd /path/to/whatsapp-sidecar && npx create-next-app@latest ui --typescript --tailwind --eslint --app --no-src-dir --import-alias "@/*" --use-npm`

**Step 2: Verify it runs**

Run: `cd ui && npm run dev`
Expected: Dev server starts on port 3000

**Step 3: Clean up default content**

Replace `ui/app/page.tsx` with:

```tsx
export default function Home() {
  return (
    <main className="min-h-screen p-8">
      <h1 className="text-2xl font-bold">WhatsApp Sidecar</h1>
      <p className="text-gray-500 mt-2">Conversations will appear here.</p>
    </main>
  );
}
```

Replace `ui/app/layout.tsx` with:

```tsx
import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "WhatsApp Sidecar",
  description: "AI-powered WhatsApp conversation summaries",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="bg-gray-50 text-gray-900">{children}</body>
    </html>
  );
}
```

**Step 4: Commit**

```bash
git add ui/
git commit -m "Scaffold Next.js frontend with Tailwind"
```

---

## Task 9: Frontend — API Client and Conversation List

**Files:**
- Create: `ui/app/api.ts`
- Modify: `ui/app/page.tsx`

**Step 1: Create API client**

```ts
// ui/app/api.ts
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export interface Conversation {
  id: string;
  name: string;
  is_group: boolean;
  updated_at: string;
}

export interface Message {
  id: string;
  conversation_id: string;
  sender: string;
  content: string;
  is_from_me: boolean;
  timestamp: string;
}

export interface Summary {
  id: number;
  conversation_id: string;
  content: string;
  created_at: string;
}

export interface ConversationDetail {
  conversation: Conversation;
  messages: Message[];
  summary: Summary | null;
}

export async function getConversations(): Promise<Conversation[]> {
  const res = await fetch(`${API_BASE}/api/conversations`, {
    cache: "no-store",
  });
  return res.json();
}

export async function getConversation(
  id: string
): Promise<ConversationDetail> {
  const res = await fetch(
    `${API_BASE}/api/conversations/${encodeURIComponent(id)}`,
    { cache: "no-store" }
  );
  return res.json();
}

export async function triggerSummarize(id: string): Promise<void> {
  await fetch(`${API_BASE}/api/summarize/${encodeURIComponent(id)}`, {
    method: "POST",
  });
}
```

**Step 2: Build conversation list page**

```tsx
// ui/app/page.tsx
"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Conversation, getConversations } from "./api";

export default function Home() {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getConversations()
      .then(setConversations)
      .catch((e) => setError(e.message));
  }, []);

  return (
    <main className="min-h-screen p-8 max-w-2xl mx-auto">
      <h1 className="text-2xl font-bold mb-6">WhatsApp Sidecar</h1>
      {error && <p className="text-red-500 mb-4">Error: {error}</p>}
      {conversations.length === 0 && !error && (
        <p className="text-gray-500">No conversations yet.</p>
      )}
      <ul className="space-y-2">
        {conversations.map((c) => (
          <li key={c.id}>
            <Link
              href={`/conversations/${encodeURIComponent(c.id)}`}
              className="block p-4 bg-white rounded-lg shadow hover:shadow-md transition-shadow"
            >
              <div className="font-medium">{c.name}</div>
              <div className="text-sm text-gray-500">
                {c.is_group ? "Group" : "Direct"} &middot;{" "}
                {new Date(c.updated_at).toLocaleString()}
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </main>
  );
}
```

**Step 3: Verify it compiles**

Run: `cd ui && npm run build`
Expected: No errors

**Step 4: Commit**

```bash
git add ui/app/api.ts ui/app/page.tsx
git commit -m "Add conversation list page with API client"
```

---

## Task 10: Frontend — Conversation Detail Page

**Files:**
- Create: `ui/app/conversations/[id]/page.tsx`

**Step 1: Create conversation detail page**

```tsx
// ui/app/conversations/[id]/page.tsx
"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ConversationDetail, getConversation, triggerSummarize } from "../../api";

export default function ConversationPage() {
  const params = useParams();
  const id = params.id as string;
  const [data, setData] = useState<ConversationDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [summarizing, setSummarizing] = useState(false);

  const load = () => {
    getConversation(decodeURIComponent(id))
      .then(setData)
      .catch((e) => setError(e.message));
  };

  useEffect(() => {
    load();
  }, [id]);

  const handleSummarize = async () => {
    setSummarizing(true);
    await triggerSummarize(decodeURIComponent(id));
    load();
    setSummarizing(false);
  };

  if (error) return <p className="p-8 text-red-500">Error: {error}</p>;
  if (!data) return <p className="p-8 text-gray-500">Loading...</p>;

  return (
    <main className="min-h-screen p-8 max-w-4xl mx-auto">
      <Link href="/" className="text-blue-600 hover:underline text-sm">
        &larr; Back
      </Link>

      <h1 className="text-2xl font-bold mt-4 mb-6">
        {data.conversation.name}
      </h1>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Messages */}
        <div className="lg:col-span-2 bg-white rounded-lg shadow p-4 max-h-[70vh] overflow-y-auto">
          <h2 className="font-semibold mb-3">Messages</h2>
          <div className="space-y-2">
            {[...data.messages].reverse().map((m) => (
              <div
                key={m.id}
                className={`p-2 rounded ${
                  m.is_from_me
                    ? "bg-blue-50 ml-8 text-right"
                    : "bg-gray-100 mr-8"
                }`}
              >
                <div className="text-xs text-gray-500">
                  {m.is_from_me ? "You" : m.sender} &middot;{" "}
                  {new Date(m.timestamp).toLocaleTimeString()}
                </div>
                <div>{m.content}</div>
              </div>
            ))}
          </div>
        </div>

        {/* Summary */}
        <div className="bg-white rounded-lg shadow p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="font-semibold">Summary</h2>
            <button
              onClick={handleSummarize}
              disabled={summarizing}
              className="text-sm px-3 py-1 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
            >
              {summarizing ? "Summarizing..." : "Refresh"}
            </button>
          </div>
          {data.summary ? (
            <p className="text-sm text-gray-700">{data.summary.content}</p>
          ) : (
            <p className="text-sm text-gray-400">No summary yet.</p>
          )}
        </div>
      </div>
    </main>
  );
}
```

**Step 2: Verify it compiles**

Run: `cd ui && npm run build`
Expected: No errors

**Step 3: Commit**

```bash
git add ui/app/conversations/
git commit -m "Add conversation detail page with summary panel"
```

---

## Task 11: Frontend — Setup/Pairing Page

**Files:**
- Create: `ui/app/setup/page.tsx`

**Step 1: Create setup page**

```tsx
// ui/app/setup/page.tsx
"use client";

import { useEffect, useState } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export default function SetupPage() {
  const [qrCode, setQrCode] = useState<string>("");

  useEffect(() => {
    const poll = setInterval(async () => {
      const res = await fetch(`${API_BASE}/api/pairing`);
      const data = await res.json();
      setQrCode(data.qr_code || "");
    }, 2000);
    return () => clearInterval(poll);
  }, []);

  return (
    <main className="min-h-screen p-8 max-w-lg mx-auto text-center">
      <h1 className="text-2xl font-bold mb-6">WhatsApp Pairing</h1>
      {qrCode ? (
        <div>
          <p className="mb-4 text-gray-600">
            Scan this code in WhatsApp &gt; Linked Devices &gt; Link a Device
          </p>
          <pre className="bg-white p-4 rounded shadow text-xs break-all">
            {qrCode}
          </pre>
        </div>
      ) : (
        <p className="text-gray-500">
          Waiting for QR code... Make sure the backend is running.
        </p>
      )}
    </main>
  );
}
```

**Step 2: Verify it compiles**

Run: `cd ui && npm run build`
Expected: No errors

**Step 3: Commit**

```bash
git add ui/app/setup/
git commit -m "Add pairing setup page"
```

---

## Task 12: End-to-End Smoke Test

**Files:** None (manual verification)

**Step 1: Ensure Ollama is running with GLM-4**

Run: `ollama pull glm4 && ollama serve`

**Step 2: Start the backend**

Run: `mise run backend`
Expected: Server starts, prints QR code if not yet paired

**Step 3: Start the frontend**

Run: `mise run ui`
Expected: Dev server starts on port 3000

**Step 4: Verify the UI loads**

Open `http://localhost:3000` — should show "No conversations yet"
or the conversation list if already paired.

**Step 5: Run all tests**

Run: `mise run test:backend`
Expected: All tests pass

**Step 6: Final commit**

```bash
git add -A
git commit -m "Complete initial WhatsApp Sidecar implementation"
```
