package models

import "time"

type Message struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	SenderID            uint      `gorm:"not null" json:"sender_id"`
	ReceiverID          uint      `gorm:"not null" json:"receiver_id"`
	Content             string    `gorm:"type:text;not null" json:"content"`
	CreatedAt           time.Time `gorm:"autoCreateTime" json:"created_at"`
	IsEncrypted         bool      `gorm:"default:false" json:"is_encrypted"`
	EncryptionAlgorithm string    `gorm:"size:50" json:"encryption_algorithm,omitempty"`
}
