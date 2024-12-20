package models

import "time"

type User struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Username         string    `gorm:"unique;not null" json:"username"`
	Email            string    `gorm:"unique;not null" json:"email"`
	PasswordHash     string    `gorm:"not null" json:"-"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	IsActive         bool      `gorm:"default:true" json:"is_active"`
	SentMessages     []Message `gorm:"foreignKey:SenderID" json:"sent_messages,omitempty"`
	ReceivedMessages []Message `gorm:"foreignKey:ReceiverID" json:"received_messages,omitempty"`
}
