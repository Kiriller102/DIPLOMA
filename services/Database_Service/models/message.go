package models

import "time"

type EncryptionAlgorithmType string

const (
	EncryptionAlgorithmAES       EncryptionAlgorithmType = "AES"
	EncryptionAlgorithmKuznechik EncryptionAlgorithmType = "Kuznechik"
)

type Message struct {
	ID                  int                     `json:"id" db:"id"`
	ChatID              int                     `json:"chat_id" db:"chat_id"`
	SenderID            int                     `json:"sender_id" db:"sender_id"`
	Content             string                  `json:"content" db:"content"`
	EncryptionAlgorithm EncryptionAlgorithmType `json:"encryption_algorithm" db:"encryption_algorithm"`
	SentAt              time.Time               `json:"sent_at" db:"sent_at"`
}
