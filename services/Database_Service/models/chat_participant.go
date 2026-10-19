package models

import "time"

type UserRoleType string

const (
	UserRoleMember UserRoleType = "member"
	UserRoleAdmin  UserRoleType = "admin"
)

type ChatParticipant struct {
	ChatID   int          `json:"chat_id" db:"chat_id"`
	UserID   int          `json:"user_id" db:"user_id"`
	UserRole UserRoleType `json:"role" db:"role"`
	JoinedAt time.Time    `json:"joined_at" db:"joined_at"`
}
