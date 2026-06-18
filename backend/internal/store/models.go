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
