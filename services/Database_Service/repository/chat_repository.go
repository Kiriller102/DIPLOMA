package repository

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"project/models"
)

type ChatRepositoryInterface interface {
	CreateChat(db *sqlx.DB, chat models.Chat) error
	GetChatByID(db *sqlx.DB, id uint) (models.Chat, error)
	UpdateChat(db *sqlx.DB, chat models.Chat) error
	DeleteChatByID(db *sqlx.DB, chat models.Chat) error
}

type ChatRepository struct{}

func (r *ChatRepository) CreateChat(db *sqlx.DB, chat models.Chat) error {
	query := `
		INSERT INTO chats (name, type, created_by, created_at) VALUES ($1, $2, $3, $4)
	`

	_, err := db.Exec(query, chat.Name, chat.Type, chat.CreatedBy, chat.CreatedAt)
	if err != nil {
		log.Printf("error inserting chat: %v", err)
		return err
	}

	return nil
}

func (r *ChatRepository) GetChatByID(db *sqlx.DB, id int) (models.Chat, error) {
	query := `
		SELECT id, name, type, created_by, created_at FROM chats WHERE id = $1 
	`

	var chat models.Chat
	err := db.QueryRow(query, id).Scan(
		&chat.ID,
		&chat.Name,
		&chat.Type,
		&chat.CreatedBy,
		&chat.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("No chat found with ID: %d", id)
			return models.Chat{}, nil
		}
		log.Printf("error getting chat: %v", err)
		return models.Chat{}, err
	}

	return chat, nil
}

func (r *ChatRepository) UpdateChat(db *sqlx.DB, chat models.Chat) error {
	query := `
		UPDATE chats
		SET 
		    name = $1,
		    type = $2,
		    created_by = $3,
		    created_at = $4
		WHERE 
		    id = $5
	`

	_, err := db.Exec(query, chat.Name, chat.Type, chat.CreatedBy, chat.CreatedAt, chat.ID)
	if err != nil {
		log.Printf("error updating chat (id: %d): %v", chat.ID, err)
		return err
	}

	return nil
}

func (r *ChatRepository) DeleteChatByID(db *sqlx.DB, id int) error {
	query := `
		DELETE FROM chats WHERE id = $1
	`

	result, err := db.Exec(query, id)
	if err != nil {
		log.Printf("error deleting chat (id: %d): %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("failed to check rows affected for chat deletion (ID: %d): %v", id, err)
		return err
	}

	if rowsAffected == 0 {
		log.Printf("chat (ID: %d) not found", id)
		return errors.New("chat not found")
	}

	return nil
}
