package repository

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
)

type MessageRepository interface {
	CreateMessage(db *sqlx.DB, message models.Message) error
	GetAllMessages(db *sqlx.DB) ([]models.Message, error)
	GetAllMessagesBySenderID(db *sqlx.DB, senderId string) ([]models.Message, error)
	GetMessagesByChatID(db *sqlx.DB, chatId string) ([]models.Message, error)
	UpdateMessage(db *sqlx.DB, message models.Message) error
	DeleteMessage(db *sqlx.DB, message models.Message) error
}

func CreateMessage(db *sqlx.DB, message models.Message) error {
	query := `
		INSERT INTO messages (chat_id, sender_id, content, encryption_algorithm, sent_at) VALUES ($1, $2, $3, $4, $5) 
	`

	_, err := db.Exec(
		query,
		message.ChatID,
		message.SenderID,
		message.Content,
		message.EncryptionAlgorithm,
		message.SentAt,
	)

	if err != nil {
		log.Printf("Error while inserting message into database: %v", err)
		return err
	}

	return nil
}

func GetAllMessages(db *sqlx.DB) ([]models.Message, error) {
	var messages []models.Message
	query := `
		SELECT id, chat_id, sender_id, content, encryption_algorithm, sent_at FROM messages
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Error while getting all messages: %v", err)
		return nil, err
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	for rows.Next() {
		var message models.Message
		err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.SenderID,
			&message.Content,
			&message.EncryptionAlgorithm,
			&message.SentAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return messages, nil
			}
			log.Printf("Error while getting all messages: %v", err)
			return nil, err
		}
		messages = append(messages, message)
	}

	return messages, nil
}

func GetAllMessagesBySenderID(db *sqlx.DB, senderId string) ([]models.Message, error) {
	var messages []models.Message

	query := `
		SELECT id, chat_id, sender_id, content, encryption_algorithm FROM messages WHERE sender_id = $1
	`
	rows, err := db.Query(query, senderId)
	if err != nil {
		log.Printf("Error while getting all messages: %v", err)
		return nil, err
	}

	for rows.Next() {
		var message models.Message
		err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.SenderID,
			&message.Content,
			&message.EncryptionAlgorithm,
			&message.SentAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return messages, nil
			}
			log.Printf("Error while getting all messages: %v", err)
			return nil, err
		}

		messages = append(messages, message)
	}
	return messages, nil
}

func GetMessagesByChatID(db *sqlx.DB, chatId string) ([]models.Message, error) {
	var messages []models.Message

	query := `
		SELECT id, chat_id, sender_id, content, encryption_algorithm FROM messages WHERE chat_id = $1
	`
	rows, err := db.Query(query, chatId)
	if err != nil {
		log.Printf("Error while getting all messages: %v", err)
		return nil, err
	}

	for rows.Next() {
		var message models.Message
		err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.SenderID,
			&message.Content,
			&message.EncryptionAlgorithm,
			&message.SentAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return messages, nil
			}
			log.Printf("Error while getting all messages: %v", err)
			return nil, err
		}

		messages = append(messages, message)
	}
	return messages, nil
}

func UpdateMessage(db *sqlx.DB, message models.Message) error {
	query := `
		UPDATE messages
		SET
		    content = $1
		WHERE 
		    id = $2
	`

	_, err := db.Exec(query, message.Content, message.ID)

	if err != nil {
		log.Printf("rrror while updating message (ID: %d) into database: %v", message.ID, err)
		return err
	}

	return nil
}

func DeleteMessage(db *sqlx.DB, message models.Message) error {
	query := `
		DELETE FROM messages
		WHERE id = $1
	`

	result, err := db.Exec(query, message.ID)
	if err != nil {
		log.Printf("error deleting message (ID:%d): %v", message.ID, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("failed to check rows affected for messaeg deletion (ID: %d): %v", message.ID, err)
		return err
	}

	if rowsAffected == 0 {
		log.Printf("message (ID: %d) not found", message.ID)
		return errors.New("message not found")
	}

	return nil
}
