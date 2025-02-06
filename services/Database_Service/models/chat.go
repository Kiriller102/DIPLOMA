package models

import "time"

type ChatType string

const (
	ChatTypeGroup   ChatType = "Group"
	ChatTypePrivate ChatType = "Private"
)

type Chat struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Type      ChatType  `json:"type"`
	CreatedBy int       `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}
