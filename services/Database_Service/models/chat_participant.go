package models

import "time"

type UserRoleType string

const (
	UserRoleMember UserRoleType = "member"
	UserRoleAdmin  UserRoleType = "admin"
)

type ChatParticipant struct {
	ChatID   int          `json:"chat_id" db:"chat_id"`
	UserID   int          `json:"users_id" db:"users_id"`
	UserRole UserRoleType `json:"role_id" db:"role_id"`
	JoinedAt time.Time    `json:"joined_at" db:"joined_at"`
}
