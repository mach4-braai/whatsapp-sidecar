package whatsapp

import (
	"context"
	"fmt"
	"time"

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
	container, err := sqlstore.New(ctx, "sqlite", fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", sessionDBPath), dbLog)
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
		groupInfo, err := c.wac.GetGroupInfo(ctx, msg.Info.Chat)
		if err == nil {
			name = groupInfo.Name
		}
	} else {
		name = c.contactName(msg.Info.Chat)
	}

	_ = c.store.UpsertConversation(ctx, store.Conversation{
		ID:        conversationJID,
		Name:      name,
		IsGroup:   msg.Info.IsGroup,
		UpdatedAt: time.Now(),
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
	contact, err := c.wac.Store.Contacts.GetContact(context.Background(), jid)
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
