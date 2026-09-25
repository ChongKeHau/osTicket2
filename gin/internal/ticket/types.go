// Package ticket implements tickets, threads, and the actions agents take on them.
package ticket

import (
	"encoding/json"
	"time"

	"github.com/grandpine/ticket-api/internal/httpx"
)

type Ref struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Ticket struct {
	ID             int64           `json:"id"`
	Number         string          `json:"number"`
	Subject        string          `json:"subject"`
	Status         Ref             `json:"status"`
	State          string          `json:"state"`
	Department     Ref             `json:"department"`
	Topic          *Ref            `json:"topic"`
	Priority       Ref             `json:"priority"`
	Assignee       *Ref            `json:"assignee"`
	RequesterName  string          `json:"requester_name"`
	RequesterEmail string          `json:"requester_email"`
	Source         string          `json:"source"`
	IsAnswered     bool            `json:"is_answered"`
	DueAt          *time.Time      `json:"due_at"`
	ClosedAt       *time.Time      `json:"closed_at"`
	LastMessageAt  time.Time       `json:"last_message_at"`
	LastResponseAt *time.Time      `json:"last_response_at"`
	Extra          json.RawMessage `json:"extra"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type Priority struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Urgency int32  `json:"urgency"`
	Color   string `json:"color"`
}

type Status struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	SortOrder int32  `json:"sort_order"`
}

type CreateInput struct {
	Subject        string          `json:"subject" binding:"required,max=255"`
	Message        string          `json:"message" binding:"required"`
	MessageFormat  string          `json:"message_format" binding:"omitempty,oneof=html text"`
	RequesterName  string          `json:"requester_name" binding:"max=128"`
	RequesterEmail string          `json:"requester_email" binding:"required,email,max=255"`
	DeptID         *int64          `json:"dept_id"`
	TopicID        *int64          `json:"topic_id"`
	PriorityID     *int64          `json:"priority_id"`
	Source         string          `json:"source" binding:"omitempty,oneof=web api phone other"`
	DueAt          *time.Time      `json:"due_at"`
	Extra          json.RawMessage `json:"extra"`
	FileIDs        []int64         `json:"file_ids"`
	// AutoSubmitted is set only by inbound mail processing (CreateExternal);
	// it suppresses the autoresponder for tickets opened by inbound mail.
	AutoSubmitted bool `json:"-"`
}

type UpdateInput struct {
	Subject        *string         `json:"subject" binding:"omitempty,min=1,max=255"`
	PriorityID     *int64          `json:"priority_id"`
	TopicID        *int64          `json:"topic_id"`
	DueAt          *time.Time      `json:"due_at"`
	Extra          json.RawMessage `json:"extra"`
	RequesterName  *string         `json:"requester_name" binding:"omitempty,max=128"`
	RequesterEmail *string         `json:"requester_email" binding:"omitempty,email,max=255"`
	// ClearTopic and ClearDueAt are set by the handler when the request body
	// contains an explicit `"topic_id": null` / `"due_at": null`, as opposed
	// to the key being absent (leave unchanged). json:"-" because these are
	// derived from raw body inspection, never bound directly.
	ClearTopic bool `json:"-"`
	ClearDueAt bool `json:"-"`
}

type ListFilter struct {
	StatusID   *int64
	State      string
	DeptID     *int64
	AssignedTo string
	Q          string
	Sort       string
	Page       httpx.Page
}
