package models

import (
	"database/sql"
	"time"
)

type NotificationType string

const (
	NotificationMessage       NotificationType = "message"
	NotificationFriendRequest NotificationType = "friend_request"
	NotificationSystem        NotificationType = "system"
)

type Notification struct {
	ID        int              `json:"id"`
	UserID    int              `json:"user_id"`
	Type      NotificationType `json:"type"`
	Context   sql.NullString   `json:"context"`
	Read      bool             `json:"read"`
	CreatedAt time.Time        `json:"created_at"`
}
