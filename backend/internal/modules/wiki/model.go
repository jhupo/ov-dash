package wiki

import "time"

const (
	PageTypeDocument        = "document"
	PageTypeMachine         = "machine"
	PageTypeLink            = "link"
	PageTypeRunbook         = "runbook"
	PageTypeTroubleshooting = "troubleshooting"
)

type Page struct {
	ID              string    `json:"id"`
	ParentID        string    `json:"parent_id"`
	Title           string    `json:"title"`
	PageType        string    `json:"page_type"`
	Category        string    `json:"category"`
	Summary         string    `json:"summary"`
	ContentMD       string    `json:"content_md"`
	LinkLabel       string    `json:"link_label"`
	LinkURL         string    `json:"link_url"`
	MachineHost     string    `json:"machine_host"`
	MachinePort     string    `json:"machine_port"`
	MachineUsername string    `json:"machine_username"`
	Tags            string    `json:"tags"`
	CreatedBy       string    `json:"created_by"`
	UpdatedBy       string    `json:"updated_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Revision struct {
	ID              string    `json:"id"`
	PageID          string    `json:"page_id"`
	Version         int       `json:"version"`
	Title           string    `json:"title"`
	PageType        string    `json:"page_type"`
	Category        string    `json:"category"`
	Summary         string    `json:"summary"`
	ContentMD       string    `json:"content_md"`
	LinkLabel       string    `json:"link_label"`
	LinkURL         string    `json:"link_url"`
	MachineHost     string    `json:"machine_host"`
	MachinePort     string    `json:"machine_port"`
	MachineUsername string    `json:"machine_username"`
	Tags            string    `json:"tags"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}

type SavePageInput struct {
	ID              string
	ParentID        string
	Title           string
	PageType        string
	Category        string
	Summary         string
	ContentMD       string
	LinkLabel       string
	LinkURL         string
	MachineHost     string
	MachinePort     string
	MachineUsername string
	Tags            string
	ActorID         string
}
