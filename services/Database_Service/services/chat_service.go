package services

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
	"strings"
	"time"
	"unicode/utf8"
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

	if err := db.Ping(); err != nil {
		log.Printf("error connection to database")
		return nil, errors.New("error connection to database")
	}

	return &ChatService{
		db: db,
	}, nil
}

func (s *ChatService) CreateChat(chat *models.Chat) error {
	if chat != nil {
		chat.Name = strings.TrimSpace(chat.Name)
	}
	if chat == nil || chat.Name == "" || utf8.RuneCountInString(chat.Name) > 100 || chat.CreatedBy <= 0 || (chat.Type != models.ChatTypeGroup && chat.Type != models.ChatTypePrivate) {
		return fmt.Errorf("%w: name, positive created_by and type group/private are required", ErrInvalidInput)
	}
	for _, id := range chat.ParticipantIDs {
		if id <= 0 {
			return fmt.Errorf("%w: participant IDs must be positive", ErrInvalidInput)
		}
	}
	chat.CreatedAt = time.Now().UTC()
	return s.chatRepo.CreateChat(s.db, chat)
}

func (s *ChatService) GetChatByID(id int) (models.Chat, error) {
	chat, err := s.chatRepo.GetChatByID(s.db, id)
	if err != nil {
		log.Printf("error: chatRepo.GetChatByID error: %v", err)
		return models.Chat{}, err
	}

	if chat.ID == 0 {
		return models.Chat{}, sql.ErrNoRows
	}

	return chat, nil
}

func (s *ChatService) UpdateChat(chat models.Chat) error {
	if strings.TrimSpace(chat.Name) == "" || utf8.RuneCountInString(chat.Name) > 100 {
		log.Printf("error: chat.Name is empty (chat.ID: %d)", chat.ID)
		return fmt.Errorf("%w: chat.Name is empty", ErrInvalidInput)
	}

	ch, err := s.chatRepo.GetChatByID(s.db, chat.ID)
	if err != nil {
		log.Printf("error: chatRepo.GetChatByID error: %v", err)
		return err
	}

	if ch.ID == 0 {
		log.Printf("error: no chat found with id: %d", chat.ID)
		return sql.ErrNoRows
	}
	ch.Name = chat.Name

	err = s.chatRepo.UpdateChat(s.db, ch)
	if err != nil {
		log.Printf("error: chatRepo.UpdateChat error: %v", err)
		return err
	}

	return nil
}

func (s *ChatService) DeleteChatByID(id int) error {
	if id <= 0 {
		log.Printf("error: DeleteChatByID id is zero")
		return fmt.Errorf("%w: chat id is required", ErrInvalidInput)
	}

	if c, err := s.chatRepo.GetChatByID(s.db, id); err != nil {
		log.Printf("error: chatRepo.GetChatByID error: %v", err)
		return err
	} else if c.ID == 0 {
		log.Printf("error: no chat found with id: %d", id)
		return sql.ErrNoRows
	}

	err := s.chatRepo.DeleteChatByID(s.db, id)
	if err != nil {
		log.Printf("error: chatRepo.DeleteChat error: %v", err)
		return err
	}

	return nil
}
