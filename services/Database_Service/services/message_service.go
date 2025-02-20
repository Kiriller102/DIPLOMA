package services

import (
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"project/models"
	"project/repository"
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

func (s *MessageService) CreateMessage(message models.Message) error {
	if message.ChatID == 0 {
		log.Printf("error: message.ChatID is nil (message.id: %d)", message.ID)
		return errors.New("chat id is required")
	}
	if message.SenderID == 0 {
		log.Printf("error: message.SenderID is nil (message.id: %d)", message.ID)
		return errors.New("sender id is required")
	}

	if message.Content == "" {
		log.Printf("error: message.Content is nil (message.id: %d)", message.ID)
		return errors.New("message content is required")
	}

	message.SentAt = time.Now()

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
	if message.ID == 0 {
		log.Printf("error: message.ID is nil (message.id: %d)", message.ID)
		return errors.New("message id is required")
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

	if message.Content == "" {
		log.Printf("error: message.Content is nil (message.id: %d)", message.ID)
		return errors.New("message content is required")
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
