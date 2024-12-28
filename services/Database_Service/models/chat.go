package models

import "time"

type ChatType string

const (
	ChatGroup   ChatType = "group"
	ChatPrivate ChatType = "private"
)

type Chat struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Type      ChatType  `json:"type"`
	CreatedBy int       `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}
