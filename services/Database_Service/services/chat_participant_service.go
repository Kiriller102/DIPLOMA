package services

import (
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"project/models"
	"project/repository"
	"time"
)

type ChatParticipantService struct {
	chatParticipantRepo repository.ChatParticipantRepository
	db                  *sqlx.DB
}

func NewChatParticipantService(db *sqlx.DB) (*ChatParticipantService, error) {
	if db == nil {
		log.Printf("database is nil")
		return nil, errors.New("database is nil")
	}
	if err := db.Ping(); err != nil {
		log.Printf("database ping error: %s", err)
		return nil, err
	}

	return &ChatParticipantService{
		db: db,
	}, nil
}

func (s *ChatParticipantService) CreateChatParticipant(chPart models.ChatParticipant) error {
	if chPart.UserID == 0 {
		log.Printf("error: ChatParticipant.UserID is zero")
		return errors.New("user id is required")
	}

	if chPart.ChatID == 0 {
		log.Printf("error: ChatParicipant.ChatID is zero")
		return errors.New("chat id is required")
	}

	chPart.JoinedAt = time.Now()

	err := s.chatParticipantRepo.CreateParticipant(s.db, chPart)
	if err != nil {
		log.Printf("error: chatParticipant.CreateParticipant error: %s", err)
		return err
	}

	return nil
}

func (s *ChatParticipantService) GetAllChatParticipants(chatID int) ([]models.ChatParticipant, error) {
	if chatID == 0 {
		log.Printf("error: ChatParticipant.ChatID is zero")
		return []models.ChatParticipant{}, errors.New("chat id is required")
	}

	chatParticipants, err := s.chatParticipantRepo.GetAllChatParticipants(s.db, chatID)
	if err != nil {
		log.Printf("error: GetAllChatParticipants error: %s", err)
		return []models.ChatParticipant{}, err
	}

	return chatParticipants, nil
}

// TODO написать GetChatsIdsByUserID для service а так же написать метод который будет возвращать массив из []chats{chat_id chat_name}

func (s *ChatParticipantService) GetChatsIdsByUserID(userID int) ([]int, error) {
	if userID == 0 {
		log.Printf("error: ChatParticipant.UserID is zero")
		return []int{}, errors.New("user id is required")
	}

	ChatsIDs, err := s.chatParticipantRepo.GetAllChatsIDsByUserID(s.db, userID)

	if err != nil {
		log.Printf("error: GetChatsIdsByUserID error: %s", err)
		return []int{}, err
	}

	return ChatsIDs, nil
}

func (s *ChatParticipantService) DeleteParticipantByUserID(participant models.ChatParticipant) error {
	if participant.UserID == 0 {
		log.Printf("error: participant.UserID is zero")
		return errors.New("user id is required")
	}

	if participant.ChatID == 0 {
		log.Printf("error: participant.ChatID is zero")
		return errors.New("chat id is required")
	}

	if cp, err := s.chatParticipantRepo.GetParticipant(s.db, participant); err != nil {
		log.Printf("error: u.GetParticipantByUserId error: %s", err)
		return err
	} else if cp == (models.ChatParticipant{}) {
		log.Printf("error: u.GetParticipantByUserId error: chat participant not found")
		return errors.New("chat participant not found")
	}

	err := s.chatParticipantRepo.DeleteParticipant(s.db, participant)
	if err != nil {
		log.Printf("error: u.GetParticipantByUserId error: %s", err)
		return err
	}

	return nil
}

func (s *ChatParticipantService) DeleteAllParticipantsByChatID(chatID int) error {
	if chatID == 0 {
		log.Printf("error: ChatParticipant.ChatID is zero")
		return errors.New("chat id is required")
	}

	err := s.chatParticipantRepo.DeleteAllParticipantsByChatID(s.db, chatID)
	if err != nil {
		log.Printf("error: u.GetParticipantByUserId error: %s", err)
		return err
	}

	return nil
}
