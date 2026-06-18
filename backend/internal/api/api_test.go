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
