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
