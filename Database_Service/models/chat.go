package models

import "time"

type Chat struct {
	ID        uint         `gorm:"primaryKey" json:"id"`
	Name      string       `gorm:"size:100" json:"name,omitempty"`
	CreatedBy uint         `gorm:"not null" json:"created_by"`
	CreatedAt time.Time    `gorm:"autoCreateTime" json:"created_at"`
	Members   []ChatMember `gorm:"foreignKey:ChatID" json:"members,omitempty"`
}

type ChatMember struct {
	// ...
	Chat Chat `gorm:"foreignKey:ChatID" json:"chat,omitempty"`
}
