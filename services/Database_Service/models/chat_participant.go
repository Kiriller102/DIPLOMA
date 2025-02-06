package models

import "time"

type UserRoleType string

const (
	UserRoleMember UserRoleType = "Member"
	UserRoleAdmin  UserRoleType = "Admin"
)

type ChatParticipant struct {
	ChatID   int          `json:"chat_id"`
	UserID   int          `json:"usersId"`
	UserRole UserRoleType `json:"roleId"`
	JoinedAt time.Time    `json:"joinedAt"`
}
