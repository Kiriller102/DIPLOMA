package services

import (
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"project/models"
	"project/repository"
	"time"
)

type ChatService struct {
	chatRepo repository.ChatRepository
	db       *sqlx.DB
}

func NewChatService(db *sqlx.DB) (*ChatService, error) {
	if db == nil {
		log.Printf("database is nil")
		return nil, errors.New("db is nil")
	}

	if db.Ping() != nil {
		log.Printf("error connection to database")
		return nil, errors.New("error connection to database")
	}

	return &ChatService{
		db: db,
	}, nil
}

func (s *ChatService) CreateChat(chat models.Chat) error {
	if chat.Name == "" {
		log.Printf("error: chat.Name is empty (chat.ID: %d", chat.ID)
		return errors.New("chat.Name is empty")
	}

	if chat.CreatedBy == 0 {
		log.Printf("error: chat.CreatedBy is empty (chat.ID: %d)", chat.CreatedBy)
		return errors.New("chat.CreatedBy is empty")
	}

	chat.CreatedAt = time.Now()

	err := s.chatRepo.CreateChat(s.db, chat)
	if err != nil {
		log.Printf("error: chatRepo.CreateChat error: %v", err)
		return err
	}

	return nil
}

func (s *ChatService) GetChatById(id int) (models.Chat, error) {
	chat, err := s.chatRepo.GetChatByID(s.db, id)
	if err != nil {
		log.Printf("error: chatRepo.GetChatByID error: %v", err)
		return models.Chat{}, err
	}

	return chat, nil
}

func (s *ChatService) UpdateChat(chat models.Chat) error {
	if chat.Name == "" {
		log.Printf("error: chat.Name is empty (chat.ID: %d)", chat.ID)
		return errors.New("chat.Name is empty")
	}

	ch, err := s.chatRepo.GetChatByID(s.db, chat.ID)
	if err != nil {
		log.Printf("error: chatRepo.GetChatByID error: %v", err)
		return err
	}

	if ch == (models.Chat{}) {
		log.Printf("error: no chat found with id: %d", chat.ID)
		return errors.New("no chat found with id")
	}

	err = s.chatRepo.UpdateChat(s.db, chat)
	if err != nil {
		log.Printf("error: chatRepo.UpdateChat error: %v", err)
		return err
	}

	return nil
}
