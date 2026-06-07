package wiki

import "time"

const (
	PageTypeDocument        = "document"
	PageTypeMachine         = "machine"
	PageTypeLink            = "link"
	PageTypeRunbook         = "runbook"
	PageTypeTroubleshooting = "troubleshooting"

	ResourceTypeMachine    = "machine"
	ResourceTypeLink       = "link"
	ResourceTypeCredential = "credential"
	ResourceTypeNote       = "note"
)

type Page struct {
	ID        string     `json:"id"`
	ParentID  string     `json:"parent_id"`
	Title     string     `json:"title"`
	PageType  string     `json:"page_type"`
	Category  string     `json:"category"`
	Summary   string     `json:"summary"`
	ContentMD string     `json:"content_md"`
	Tags      string     `json:"tags"`
	Resources []Resource `json:"resources"`
	CreatedBy string     `json:"created_by"`
	UpdatedBy string     `json:"updated_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type Resource struct {
	ID           string    `json:"id"`
	PageID       string    `json:"page_id"`
	ResourceType string    `json:"resource_type"`
	Title        string    `json:"title"`
	Host         string    `json:"host"`
	Port         string    `json:"port"`
	URL          string    `json:"url"`
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	Note         string    `json:"note"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Revision struct {
	ID        string    `json:"id"`
	PageID    string    `json:"page_id"`
	Version   int       `json:"version"`
	Title     string    `json:"title"`
	PageType  string    `json:"page_type"`
	Category  string    `json:"category"`
	Summary   string    `json:"summary"`
	ContentMD string    `json:"content_md"`
	Tags      string    `json:"tags"`
	Resources []Resource `json:"resources"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type SavePageInput struct {
	ID        string
	ParentID  string
	Title     string
	PageType  string
	Category  string
	Summary   string
	ContentMD string
	Tags      string
	Resources []SaveResourceInput
	ActorID   string
}

type SaveResourceInput struct {
	ID           string
	ResourceType string
	Title        string
	Host         string
	Port         string
	URL          string
	Username     string
	Password     string
	Note         string
	SortOrder    int
}
