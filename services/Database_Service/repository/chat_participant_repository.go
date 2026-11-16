package repository

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
)

type ChatParticipantRepositoryInterface interface {
	CreateParticipant(ctx context.Context, db *sqlx.DB, participant models.ChatParticipant) error
	GetParticipant(ctx context.Context, db *sqlx.DB, participant models.ChatParticipant) (models.ChatParticipant, error)
	GetAllChatParticipants(ctx context.Context, db *sqlx.DB, chatID int) ([]models.ChatParticipant, error)
	DeleteAllParticipantsByChatID(db *sqlx.DB, chatID int) error
	DeleteParticipant(ctx context.Context, db *sqlx.DB, participant models.ChatParticipant) error
}

type ChatParticipantRepository struct{}

func (r *ChatParticipantRepository) CreateParticipant(ctx context.Context, db *sqlx.DB, participant models.ChatParticipant) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = LockChat(ctx, tx, participant.ChatID); err != nil {
		return err
	}
	query := `
		INSERT INTO chat_participants (chat_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err = tx.ExecContext(ctx, query, participant.ChatID, participant.UserID, participant.UserRole, participant.JoinedAt)
	if err != nil {
		log.Printf("error inserting participant: %v", err.Error())
		return err
	}
	return tx.Commit()
}

func (r *ChatParticipantRepository) GetParticipant(ctx context.Context, db *sqlx.DB, participant models.ChatParticipant) (models.ChatParticipant, error) {
	query := `
		SELECT * FROM chat_participants WHERE chat_id = $1 AND user_id = $2
	`

	var chPart models.ChatParticipant
	err := db.GetContext(ctx, &chPart, query, participant.ChatID, participant.UserID)
	if err != nil {
		log.Printf("error getting participant: %v", err.Error())
		return models.ChatParticipant{}, err
	}

	return chPart, nil
}

func (r *ChatParticipantRepository) GetAllChatParticipants(ctx context.Context, db *sqlx.DB, chatID int) ([]models.ChatParticipant, error) {
	participants := make([]models.ChatParticipant, 0)
	err := db.SelectContext(ctx, &participants, `SELECT chat_id, user_id, role, joined_at FROM chat_participants WHERE chat_id = $1 ORDER BY user_id`, chatID)
	return participants, err
}

func (r *ChatParticipantRepository) GetAllChatsByUserID(ctx context.Context, db *sqlx.DB, userID int) ([]models.Chat, error) {
	chats := make([]models.Chat, 0)
	err := db.SelectContext(ctx, &chats, `SELECT c.id, c.name, c.type, c.created_by, c.created_at FROM chats c
        JOIN chat_participants p ON p.chat_id = c.id WHERE p.user_id = $1 ORDER BY c.id`, userID)
	return chats, err
}

func (r *ChatParticipantRepository) DeleteAllParticipantsByChatID(db *sqlx.DB, chatID int) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = LockChat(context.Background(), tx, chatID); err != nil {
		return err
	}
	query := `
		DELETE FROM chat_participants 
		WHERE chat_id = $1
	`

	result, err := tx.Exec(query, chatID)
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

	return tx.Commit()
}

func (r *ChatParticipantRepository) DeleteParticipant(ctx context.Context, db *sqlx.DB, participant models.ChatParticipant) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = LockChat(ctx, tx, participant.ChatID); err != nil {
		return err
	}
	query := `
		DELETE FROM chat_participants
		WHERE user_id = $1 AND chat_id = $2
	`
	result, err := tx.ExecContext(ctx, query, participant.UserID, participant.ChatID)
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

	return tx.Commit()
}
