package tasks

import "time"

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Label       string     `json:"label"`
	Priority    string     `json:"priority"`
	Description string     `json:"description"`
	Assignee    string     `json:"assignee"`
	DueDate     *time.Time `json:"dueDate,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
