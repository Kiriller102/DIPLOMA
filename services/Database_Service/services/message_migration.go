package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"project/services/Database_Service/e2ee"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
)

// EncryptLegacyMessage replaces exactly the legacy text the client read. It is
// idempotent when two participants migrate the same message simultaneously.
func (s *MessageService) EncryptLegacyMessage(ctx context.Context, id, signerID int, content, previousHash string) (models.Message, error) {
	var chatID int
	if err := s.db.GetContext(ctx, &chatID, `SELECT chat_id FROM messages WHERE id=$1`, id); err != nil {
		return models.Message{}, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.Message{}, err
	}
	defer tx.Rollback()
	if err = repository.LockChat(tx, chatID); err != nil {
		return models.Message{}, err
	}
	var message models.Message
	err = tx.GetContext(ctx, &message, `SELECT id,chat_id,sender_id,content,COALESCE(encryption_algorithm::text,'') AS encryption_algorithm,sent_at,crypto_version,COALESCE(client_id,'') AS client_id FROM messages WHERE id=$1 FOR UPDATE`, id)
	if err != nil {
		return message, err
	}
	// Authorize inside the transaction as well as in the HTTP controller.
	var member bool
	if err = tx.GetContext(ctx, &member, `SELECT EXISTS(SELECT 1 FROM chat_participants WHERE chat_id=$1 AND user_id=$2)`, message.ChatID, signerID); err != nil {
		return message, err
	}
	if !member {
		return message, fmt.Errorf("%w: chat participant required", ErrInvalidInput)
	}
	if message.CryptoVersion == 1 {
		return message, nil
	}
	digest := sha256.Sum256([]byte(message.Content))
	if previousHash != base64.StdEncoding.EncodeToString(digest[:]) {
		return message, fmt.Errorf("%w: legacy message changed; reload history", ErrInvalidInput)
	}
	keys := make([]e2ee.PublicIdentity, 0)
	if err = tx.SelectContext(ctx, &keys, `SELECT p.user_id,COALESCE(i.key_id,'') AS key_id,COALESCE(i.exchange_key,'') AS exchange_key,COALESCE(i.signing_key,'') AS signing_key FROM chat_participants p LEFT JOIN crypto_identities i ON i.user_id=p.user_id WHERE p.chat_id=$1 ORDER BY p.user_id FOR SHARE OF p`, message.ChatID); err != nil {
		return message, err
	}
	envelope, err := e2ee.Validate(content, message.ChatID, message.SenderID, signerID, id, keys)
	if err != nil {
		return message, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	message.Content = content
	message.EncryptionAlgorithm = models.EncryptionAlgorithmType(envelope.Algorithm)
	message.CryptoVersion = 1
	message.ClientID = envelope.MessageID
	_, err = tx.ExecContext(ctx, `UPDATE messages SET content=$1,encryption_algorithm=$2::encryption_algorithm_type,crypto_version=1,client_id=$3 WHERE id=$4`, content, envelope.Algorithm, envelope.MessageID, id)
	if err != nil {
		return message, err
	}
	return message, tx.Commit()
}
