package chats

import (
	"context"

	"ov-dash/backend/internal/db"
)

type Repository struct {
	db *db.Pool
}

func NewRepository(db *db.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListConversations(ctx context.Context) ([]Conversation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, full_name, username, profile, title, created_at, updated_at
		FROM chat_conversations
		ORDER BY updated_at DESC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Conversation, 0)
	for rows.Next() {
		var item Conversation
		if err := rows.Scan(
			&item.ID,
			&item.FullName,
			&item.Username,
			&item.Profile,
			&item.Title,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Messages = []Message{}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range items {
		messages, err := r.listMessages(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].Messages = messages
	}

	return items, nil
}

func (r *Repository) listMessages(ctx context.Context, conversationID string) ([]Message, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, sender_id, content, created_at
		FROM chat_messages
		WHERE conversation_id = $1
		ORDER BY created_at DESC, id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Message, 0)
	for rows.Next() {
		var item Message
		if err := rows.Scan(
			&item.ID,
			&item.Sender,
			&item.Message,
			&item.Timestamp,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
