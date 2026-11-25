package services

import (
	"context"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
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

func (s *MessageService) CreateMessage(ctx context.Context, message *models.Message) error {
	if message == nil || message.ChatID <= 0 {
		log.Printf("error: message or chat ID is missing")
		return fmt.Errorf("%w: chat id is required", ErrInvalidInput)
	}
	if message.SenderID <= 0 {
		log.Printf("error: message.SenderID is nil (message.id: %d)", message.ID)
		return fmt.Errorf("%w: sender id is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = repository.LockChat(ctx, tx, message.ChatID); err != nil {
		return err
	}
	if err := s.validateEncryption(ctx, tx, message, message.SenderID, 0); err != nil {
		return err
	}
	message.SentAt = time.Now().UTC()

	err = s.messageRepo.CreateMessage(ctx, tx, message)
	if err != nil {
		log.Printf("error: messageRepo.CreateMessage error: %v", err)
		return err
	}

	return tx.Commit()
}

func (s *MessageService) GetMessagesByChatID(ctx context.Context, ChatID int) ([]models.Message, error) {
	messages, err := s.messageRepo.GetMessagesByChatID(ctx, s.db, ChatID)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

func (s *MessageService) GetMessageByID(ctx context.Context, messageID int) (models.Message, error) {
	message, err := s.messageRepo.GetMessageByID(ctx, s.db, messageID)
	if err != nil {
		log.Printf("error: messageRepo.GetMessageByID error: %v", err)
		return models.Message{}, err
	}

	return message, nil
}

func (s *MessageService) UpdateMessage(ctx context.Context, message models.Message) error {
	if message.ID <= 0 {
		log.Printf("error: message.ID is nil (message.id: %d)", message.ID)
		return fmt.Errorf("%w: message id is required", ErrInvalidInput)
	}

	msg, err := s.messageRepo.GetMessageByID(ctx, s.db, message.ID)
	if err != nil {
		log.Printf("error: messageRepo.GetMessageByID error: %v", err)
		return err
	}

	if msg == (models.Message{}) {
		log.Printf("error: no message found with id: %d", message.ID)
		return errors.New("no message found with id")
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = repository.LockChat(ctx, tx, msg.ChatID); err != nil {
		return err
	}
	msg, err = s.messageRepo.GetMessageByID(ctx, tx, message.ID)
	if err != nil {
		return err
	}

	msg.Content = message.Content
	if err := s.validateEncryption(ctx, tx, &msg, msg.SenderID, 0); err != nil {
		return err
	}

	err = s.messageRepo.UpdateMessage(ctx, tx, msg)
	if err != nil {
		log.Printf("error: messageRepo.UpdateMessage error: %v", err)
		return err
	}

	return tx.Commit()
}

func (s *MessageService) GetMessagesAfter(ctx context.Context, chatID, lastID int) ([]models.Message, error) {
	return s.messageRepo.GetMessagesAfter(ctx, s.db, chatID, lastID)
}
