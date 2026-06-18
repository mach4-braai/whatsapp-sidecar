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
	origin := r.Header.Get("Origin")
	allowedOrigins := map[string]bool{
		"http://localhost:3000":   true,
		"http://10.0.0.147:3000": true,
	}
	if allowedOrigins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
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
