package store

import (
	"context"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	s, err := New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewStore(t *testing.T) {
	s := newTestStore(t)

	// Verify all three tables exist.
	for _, table := range []string{"conversations", "messages", "summaries"} {
		var name string
		err := s.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %s not found: %v", table, err)
		}
	}
}

func TestUpsertAndListConversations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	c1 := Conversation{ID: "c1", Name: "Alice", IsGroup: false, UpdatedAt: now}
	c2 := Conversation{ID: "c2", Name: "Group Chat", IsGroup: true, UpdatedAt: now.Add(time.Second)}

	if err := s.UpsertConversation(ctx, c1); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConversation(ctx, c2); err != nil {
		t.Fatal(err)
	}

	convos, err := s.ListConversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(convos) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(convos))
	}
	// Should be ordered by updated_at DESC, so c2 first.
	if convos[0].ID != "c2" {
		t.Errorf("expected first conversation c2, got %s", convos[0].ID)
	}
	if !convos[0].IsGroup {
		t.Error("expected c2 to be a group")
	}

	// Upsert c1 with new name.
	c1.Name = "Alice Updated"
	c1.UpdatedAt = now.Add(2 * time.Second)
	if err := s.UpsertConversation(ctx, c1); err != nil {
		t.Fatal(err)
	}
	convos, _ = s.ListConversations(ctx)
	if convos[0].ID != "c1" || convos[0].Name != "Alice Updated" {
		t.Errorf("upsert did not update: got %+v", convos[0])
	}
}

func TestInsertAndGetMessages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	if err := s.UpsertConversation(ctx, Conversation{ID: "c1", Name: "Test", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	m1 := Message{ID: "m1", ConversationID: "c1", Sender: "alice", Content: "hello", Timestamp: now}
	m2 := Message{ID: "m2", ConversationID: "c1", Sender: "me", Content: "hi", IsFromMe: true, Timestamp: now.Add(time.Second)}

	if err := s.InsertMessage(ctx, m1); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertMessage(ctx, m2); err != nil {
		t.Fatal(err)
	}

	// Duplicate insert should be ignored.
	if err := s.InsertMessage(ctx, m1); err != nil {
		t.Fatal(err)
	}

	msgs, err := s.GetMessages(ctx, "c1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	// Ordered by timestamp DESC.
	if msgs[0].ID != "m2" {
		t.Errorf("expected m2 first, got %s", msgs[0].ID)
	}
	if !msgs[0].IsFromMe {
		t.Error("expected m2 IsFromMe=true")
	}
}

func TestInsertAndGetSummary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	if err := s.UpsertConversation(ctx, Conversation{ID: "c1", Name: "Test", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	// No summary yet.
	sum, err := s.GetLatestSummary(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if sum != nil {
		t.Fatal("expected nil summary")
	}

	id, err := s.InsertSummary(ctx, "c1", "summary content")
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Error("expected non-zero summary ID")
	}

	sum, err = s.GetLatestSummary(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if sum == nil {
		t.Fatal("expected summary, got nil")
	}
	if sum.Content != "summary content" {
		t.Errorf("unexpected content: %s", sum.Content)
	}
}

func TestGetConversationsWithNewMessages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	if err := s.UpsertConversation(ctx, Conversation{ID: "c1", Name: "A", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertConversation(ctx, Conversation{ID: "c2", Name: "B", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	// Messages for both.
	s.InsertMessage(ctx, Message{ID: "m1", ConversationID: "c1", Sender: "x", Content: "hi", Timestamp: now})
	s.InsertMessage(ctx, Message{ID: "m2", ConversationID: "c2", Sender: "y", Content: "yo", Timestamp: now})

	// No summaries yet — both should appear.
	ids, err := s.GetConversationsWithNewMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2, got %d", len(ids))
	}

	// Summarize c1.
	s.InsertSummary(ctx, "c1", "done")

	// Add a new message to c1 after the summary.
	time.Sleep(time.Second) // ensure timestamp > summary created_at
	s.InsertMessage(ctx, Message{ID: "m3", ConversationID: "c1", Sender: "x", Content: "new", Timestamp: time.Now()})

	ids, err = s.GetConversationsWithNewMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 (both have new msgs), got %d: %v", len(ids), ids)
	}
}
