package services

import (
	"context"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
	"strings"
	"time"
)

type MessageService struct {
	messageRepo repository.MessageRepository
	db          *sqlx.DB
}

func NewMessageService(db *sqlx.DB) (*MessageService, error) {
	if db == nil {
		log.Printf("database is nil")
		return nil, errors.New("database is nil")
	}

	if err := db.Ping(); err != nil {
		log.Printf("error connection to database: %v", err)
		return nil, err
	}

	return &MessageService{
		db: db,
	}, nil
}

func (s *MessageService) CreateMessage(message *models.Message) error {
	if message == nil || message.ChatID <= 0 {
		log.Printf("error: message or chat ID is missing")
		return fmt.Errorf("%w: chat id is required", ErrInvalidInput)
	}
	if message.SenderID <= 0 {
		log.Printf("error: message.SenderID is nil (message.id: %d)", message.ID)
		return fmt.Errorf("%w: sender id is required", ErrInvalidInput)
	}

	if strings.TrimSpace(message.Content) == "" || len(message.Content) > 65536 {
		log.Printf("error: message.Content is nil (message.id: %d)", message.ID)
		return fmt.Errorf("%w: message content is required", ErrInvalidInput)
	}

	if len(message.Content) > 65536 {
		return fmt.Errorf("%w: message exceeds 65536 bytes", ErrInvalidInput)
	}
	if message.EncryptionAlgorithm != "" && message.EncryptionAlgorithm != models.EncryptionAlgorithmAES && message.EncryptionAlgorithm != models.EncryptionAlgorithmKuznechik {
		return fmt.Errorf("%w: unknown encryption_algorithm", ErrInvalidInput)
	}
	var member bool
	if err := s.db.Get(&member, `SELECT EXISTS (SELECT 1 FROM chat_participants WHERE chat_id = $1 AND user_id = $2)`, message.ChatID, message.SenderID); err != nil {
		return err
	}
	if !member {
		return fmt.Errorf("%w: sender must be a chat participant", ErrInvalidInput)
	}
	message.SentAt = time.Now().UTC()

	err := s.messageRepo.CreateMessage(s.db, message)
	if err != nil {
		log.Printf("error: messageRepo.CreateMessage error: %v", err)
		return err
	}

	return nil
}

func (s *MessageService) GetMessagesByChatID(ChatID int) ([]models.Message, error) {
	messages, err := s.messageRepo.GetMessagesByChatID(s.db, ChatID)
	if err != nil {
		log.Printf("error: messageRepo.GetMessagesByChatID error: %v", err)
		return nil, err
	}

	return messages, nil
}

func (s *MessageService) GetMessageByID(messageID int) (models.Message, error) {
	message, err := s.messageRepo.GetMessageByID(s.db, messageID)
	if err != nil {
		log.Printf("error: messageRepo.GetMessageByID error: %v", err)
		return models.Message{}, err
	}

	return message, nil
}

func (s *MessageService) UpdateMessage(message models.Message) error {
	if message.ID <= 0 {
		log.Printf("error: message.ID is nil (message.id: %d)", message.ID)
		return fmt.Errorf("%w: message id is required", ErrInvalidInput)
	}

	msg, err := s.messageRepo.GetMessageByID(s.db, message.ID)
	if err != nil {
		log.Printf("error: messageRepo.GetMessageByID error: %v", err)
		return err
	}

	if msg == (models.Message{}) {
		log.Printf("error: no message found with id: %d", message.ID)
		return errors.New("no message found with id")
	}

	if strings.TrimSpace(message.Content) == "" || len(message.Content) > 65536 {
		log.Printf("error: message.Content is nil (message.id: %d)", message.ID)
		return fmt.Errorf("%w: message content is required", ErrInvalidInput)
	}

	msg.Content = message.Content
	msg.EncryptionAlgorithm = message.EncryptionAlgorithm

	err = s.messageRepo.UpdateMessage(s.db, msg)
	if err != nil {
		log.Printf("error: messageRepo.UpdateMessage error: %v", err)
		return err
	}

	return nil
}

func (s *MessageService) GetMessagesAfter(ctx context.Context, chatID, lastID int) ([]models.Message, error) {
	return s.messageRepo.GetMessagesAfter(ctx, s.db, chatID, lastID)
}
