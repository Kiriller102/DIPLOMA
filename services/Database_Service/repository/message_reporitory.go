package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/internal/logging"
	"project/services/Database_Service/models"
)

type MessageRepositoryInterface interface {
	CreateMessage(ctx context.Context, db sqlx.ExtContext, message *models.Message) error
	GetMessageByID(ctx context.Context, db sqlx.QueryerContext, id int) (models.Message, error)
	GetAllMessages(db *sqlx.DB) ([]models.Message, error)
	GetAllMessagesBySenderID(db *sqlx.DB, senderId string) ([]models.Message, error)
	GetMessagesByChatID(ctx context.Context, db *sqlx.DB, chatId int) ([]models.Message, error)
	GetMessagesBefore(ctx context.Context, db *sqlx.DB, chatID int, beforeID *int, limit int) ([]models.Message, error)
	UpdateMessage(ctx context.Context, db sqlx.ExtContext, message models.Message) error
	DeleteMessageByID(db *sqlx.DB, id int) error
}

type MessageRepository struct{}

func (r *MessageRepository) CreateMessage(ctx context.Context, db sqlx.ExtContext, message *models.Message) error {
	query := `
		INSERT INTO messages (chat_id, sender_id, content, encryption_algorithm, sent_at, crypto_version, client_id) VALUES ($1, $2, $3, $4::encryption_algorithm_type, $5, $6, $7) RETURNING id
	`

	err := db.QueryRowxContext(ctx,
		query,
		message.ChatID,
		message.SenderID,
		message.Content,
		message.EncryptionAlgorithm,
		message.SentAt,
		message.CryptoVersion,
		message.ClientID,
	).Scan(&message.ID)

	if err != nil {
		log.Printf("Error while inserting message into database: %v", err)
		return err
	}

	return nil
}

func (r *MessageRepository) GetMessageByID(ctx context.Context, db sqlx.QueryerContext, id int) (models.Message, error) {
	var message models.Message

	if id == 0 {
		return models.Message{}, errors.New("message ID is required")
	}

	query := `
		SELECT id, chat_id, sender_id, content, COALESCE(encryption_algorithm::text, '') AS encryption_algorithm, sent_at, crypto_version, COALESCE(client_id,'') AS client_id FROM messages WHERE id = $1
	`

	err := sqlx.GetContext(ctx, db, &message, query, id)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("No message found with ID: %v", err)
			return models.Message{}, err
		}
		log.Printf("Error while getting message by ID: %v", err)
		return models.Message{}, err
	}

	return message, nil
}

func (r *MessageRepository) GetAllMessages(db *sqlx.DB) ([]models.Message, error) {
	messages := make([]models.Message, 0)
	query := `
		SELECT id, chat_id, sender_id, content, COALESCE(encryption_algorithm::text, '') AS encryption_algorithm, sent_at, crypto_version, COALESCE(client_id,'') AS client_id FROM messages
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Error while getting all messages: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var message models.Message
		err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.SenderID,
			&message.Content,
			&message.EncryptionAlgorithm,
			&message.SentAt, &message.CryptoVersion, &message.ClientID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return messages, rows.Err()
			}
			log.Printf("Error while getting all messages: %v", err)
			return nil, err
		}
		messages = append(messages, message)
	}

	return messages, rows.Err()
}

func (r *MessageRepository) GetAllMessagesBySenderID(db *sqlx.DB, senderId string) ([]models.Message, error) {
	messages := make([]models.Message, 0)

	query := `
		SELECT id, chat_id, sender_id, content, COALESCE(encryption_algorithm::text, '') AS encryption_algorithm, sent_at, crypto_version, COALESCE(client_id,'') AS client_id FROM messages WHERE sender_id = $1
	`
	rows, err := db.Query(query, senderId)
	if err != nil {
		log.Printf("Error while getting all messages: %v", err)
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var message models.Message
		err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.SenderID,
			&message.Content,
			&message.EncryptionAlgorithm,
			&message.SentAt, &message.CryptoVersion, &message.ClientID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return messages, rows.Err()
			}
			log.Printf("Error while getting all messages: %v", err)
			return nil, err
		}

		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r *MessageRepository) GetMessagesByChatID(ctx context.Context, db *sqlx.DB, chatId int) ([]models.Message, error) {
	messages := make([]models.Message, 0)

	query := `
		SELECT id, chat_id, sender_id, content, COALESCE(encryption_algorithm::text, '') AS encryption_algorithm, sent_at, crypto_version, COALESCE(client_id,'') AS client_id
		FROM messages
		WHERE chat_id = $1
		ORDER BY id
	`
	rows, err := db.QueryContext(ctx, query, chatId)
	if err != nil {
		return nil, logging.Annotate(err, logging.MessageHistory, "query")
	}

	defer rows.Close()

	for rows.Next() {
		var message models.Message
		err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.SenderID,
			&message.Content,
			&message.EncryptionAlgorithm,
			&message.SentAt, &message.CryptoVersion, &message.ClientID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return messages, logging.Annotate(rows.Err(), logging.MessageHistory, "iteration")
			}
			return nil, logging.Annotate(err, logging.MessageHistory, "scan")
		}

		messages = append(messages, message)
	}
	return messages, logging.Annotate(rows.Err(), logging.MessageHistory, "iteration")
}

// GetMessagesBefore returns messages in descending ID order. A nil cursor starts
// at the newest message; limit includes any lookahead requested by the service.
func (r *MessageRepository) GetMessagesBefore(ctx context.Context, db *sqlx.DB, chatID int, beforeID *int, limit int) ([]models.Message, error) {
	query := `
		SELECT id, chat_id, sender_id, content, COALESCE(encryption_algorithm::text, '') AS encryption_algorithm, sent_at, crypto_version, COALESCE(client_id,'') AS client_id
		FROM messages
		WHERE chat_id = $1
	`
	args := []any{chatID}
	if beforeID != nil {
		query += " AND id < $2"
		args = append(args, *beforeID)
	}
	query += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d", len(args)+1)
	args = append(args, limit)

	rows, err := db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, logging.Annotate(err, logging.MessageHistory, "query")
	}
	defer rows.Close()

	messages := make([]models.Message, 0)
	for rows.Next() {
		var message models.Message
		if err := rows.StructScan(&message); err != nil {
			return nil, logging.Annotate(err, logging.MessageHistory, "scan")
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, logging.Annotate(err, logging.MessageHistory, "iteration")
	}
	return messages, nil
}

func (r *MessageRepository) UpdateMessage(ctx context.Context, db sqlx.ExtContext, message models.Message) error {
	query := `
		UPDATE messages
		SET
		    content = $1, encryption_algorithm = $3::encryption_algorithm_type, crypto_version = $4, client_id = $5
		WHERE
		    id = $2
	`

	_, err := db.ExecContext(ctx, query, message.Content, message.ID, message.EncryptionAlgorithm, message.CryptoVersion, message.ClientID)

	if err != nil {
		log.Printf("rrror while updating message (ID: %d) into database: %v", message.ID, err)
		return err
	}

	return nil
}

func (r *MessageRepository) DeleteMessageByID(db *sqlx.DB, id int) error {
	query := `
		DELETE FROM messages
		WHERE id = $1
	`

	result, err := db.Exec(query, id)
	if err != nil {
		log.Printf("error deleting message (ID:%d): %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("failed to check rows affected for messaeg deletion (ID: %d): %v", id, err)
		return err
	}

	if rowsAffected == 0 {
		log.Printf("message (ID: %d) not found", id)
		return fmt.Errorf("message (ID: %d) not found", id)
	}

	return nil
}

func (r *MessageRepository) GetMessagesAfter(ctx context.Context, db *sqlx.DB, chatID, lastID int) ([]models.Message, error) {
	messages := make([]models.Message, 0)
	err := db.SelectContext(ctx, &messages, `SELECT id, chat_id, sender_id, content,
        COALESCE(encryption_algorithm::text, '') AS encryption_algorithm, sent_at, crypto_version, COALESCE(client_id,'') AS client_id
        FROM messages WHERE chat_id = $1 AND id > $2 ORDER BY id LIMIT 100`, chatID, lastID)
	return messages, err
}
