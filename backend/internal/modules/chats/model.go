package chats

import "time"

type Message struct {
	ID        string    `json:"id"`
	Sender    string    `json:"sender"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type Conversation struct {
	ID        string    `json:"id"`
	FullName  string    `json:"fullName"`
	Username  string    `json:"username"`
	Profile   string    `json:"profile"`
	Title     string    `json:"title"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
