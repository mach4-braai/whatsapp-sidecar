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
	b.WriteString("Summarize this WhatsApp conversation in 1-3 short sentences. Be factual and neutral. Do not interpret tone or emotions beyond what is explicitly stated.\n\nMessages:\n")
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
