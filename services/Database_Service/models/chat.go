package models

import "time"

type ChatType string

const (
	ChatTypeGroup   ChatType = "group"
	ChatTypePrivate ChatType = "private"
)

type Chat struct {
	ParticipantIDs []int     `json:"participant_ids,omitempty" db:"-"`
	ID             int       `json:"id" db:"id"`
	Name           string    `json:"name" db:"name"`
	Type           ChatType  `json:"type" db:"type"`
	CreatedBy      int       `json:"created_by" db:"created_by"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}
