package models

import "time"

type EncryptionAlgorithmType string

const (
	EncryptionAES       EncryptionAlgorithmType = "AES"
	EncryptionKuznechik EncryptionAlgorithmType = "Kuznechik"
)

type Message struct {
	ID                  int                     `json:"id"`
	ChatID              int                     `json:"chat_id"`
	SenderID            int                     `json:"sender_id"`
	Content             string                  `json:"content"`
	EncryptionAlgorithm EncryptionAlgorithmType `json:"encryption_algorithm"`
	SentAt              time.Time               `json:"sent_at"`
}
