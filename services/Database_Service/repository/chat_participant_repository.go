package repository

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
)

type ChatParticipantRepositoryInterface interface {
	CreateParticipant(db *sqlx.DB, participant models.ChatParticipant) error
	GetParticipant(db *sqlx.DB, participant models.ChatParticipant) (models.ChatParticipant, error)
	GetAllChatParticipants(db *sqlx.DB, chatID int) ([]models.ChatParticipant, error)
	DeleteAllParticipantsByChatID(db *sqlx.DB, chatID int) error
	DeleteParticipant(db *sqlx.DB, participant models.ChatParticipant) error
}

type ChatParticipantRepository struct{}

func (r *ChatParticipantRepository) CreateParticipant(db *sqlx.DB, participant models.ChatParticipant) error {
	query := `
		INSERT INTO chat_participants (chat_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := db.Exec(query, participant.ChatID, participant.UserID, participant.UserRole, participant.JoinedAt)
	if err != nil {
		log.Printf("error inserting participant: %v", err.Error())
		return err
	}
	return nil
}

func (r *ChatParticipantRepository) GetParticipant(db *sqlx.DB, participant models.ChatParticipant) (models.ChatParticipant, error) {
	query := `
		SELECT * FROM chat_participants WHERE chat_id = $1 AND user_id = $2
	`

	var chPart models.ChatParticipant
	err := db.Get(&chPart, query, participant.ChatID, participant.UserID)
	if err != nil {
		log.Printf("error getting participant: %v", err.Error())
		return models.ChatParticipant{}, err
	}

	return chPart, nil
}

func (r *ChatParticipantRepository) GetAllChatParticipants(db *sqlx.DB, chatID int) ([]models.ChatParticipant, error) {
	participants := make([]models.ChatParticipant, 0)
	err := db.Select(&participants, `SELECT chat_id, user_id, role, joined_at FROM chat_participants WHERE chat_id = $1 ORDER BY user_id`, chatID)
	return participants, err
}

func (r *ChatParticipantRepository) GetAllChatsByUserID(db *sqlx.DB, userID int) ([]models.Chat, error) {
	chats := make([]models.Chat, 0)
	err := db.Select(&chats, `SELECT c.id, c.name, c.type, c.created_by, c.created_at FROM chats c
        JOIN chat_participants p ON p.chat_id = c.id WHERE p.user_id = $1 ORDER BY c.id`, userID)
	return chats, err
}

func (r *ChatParticipantRepository) DeleteAllParticipantsByChatID(db *sqlx.DB, chatID int) error {
	query := `
		DELETE FROM chat_participants 
		WHERE chat_id = $1
	`

	result, err := db.Exec(query, chatID)
	if err != nil {
		log.Printf("error deleting participants: %v", err.Error())
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("error deleting participants: %v", err.Error())
		return err
	}

	if rowsAffected == 0 {
		log.Printf("no participants found for chatID: %d", chatID)
		return fmt.Errorf("no participants found for chatID: %d", chatID)
	}

	return nil
}

func (r *ChatParticipantRepository) DeleteParticipant(db *sqlx.DB, participant models.ChatParticipant) error {
	query := `
		DELETE FROM chat_participants
		WHERE user_id = $1 AND chat_id = $2
	`
	result, err := db.Exec(query, participant.UserID, participant.ChatID)
	if err != nil {
		log.Printf("error deleting participants: %v", err.Error())
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("failed to check rows affcted for participant deletion: %v", err.Error())
		return err
	}

	if rowsAffected == 0 {
		log.Printf("no participants found for chatID: %d", participant.ChatID)
		return fmt.Errorf("no participants found for chatID: %d", participant.ChatID)
	}
	if rowsAffected > 1 {
		log.Printf("participants found for chatID: %d", participant.ChatID)
		return fmt.Errorf("participants found for chatID: %d (user_id: %d", participant.ChatID, participant.UserID)
	}

	return nil
}
