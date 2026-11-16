package services

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"project/services/Database_Service/e2ee"
	"project/services/Database_Service/models"
)

type CryptoService struct{ db *sqlx.DB }

func NewCryptoService(db *sqlx.DB) *CryptoService { return &CryptoService{db: db} }

func (s *CryptoService) Identity(ctx context.Context, userID int) (e2ee.Identity, error) {
	var identity e2ee.Identity
	err := s.db.GetContext(ctx, &identity, `SELECT user_id,key_id,exchange_key,signing_key,salt,nonce,encrypted_private_keys,iterations FROM crypto_identities WHERE user_id=$1`, userID)
	return identity, err
}

func (s *CryptoService) CreateIdentity(ctx context.Context, identity e2ee.Identity) error {
	if err := identity.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	// No replacement endpoint: replacing a public key would silently strand history.
	_, err := s.db.ExecContext(ctx, `INSERT INTO crypto_identities(user_id,key_id,exchange_key,signing_key,salt,nonce,encrypted_private_keys,iterations) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		identity.UserID, identity.KeyID, identity.ExchangeKey, identity.SigningKey, identity.Salt, identity.Nonce, identity.EncryptedPrivateKeys, identity.Iterations)
	return err
}

func (s *CryptoService) PublicKeys(ctx context.Context) ([]e2ee.PublicIdentity, error) {
	keys := make([]e2ee.PublicIdentity, 0)
	err := s.db.SelectContext(ctx, &keys, `SELECT user_id,key_id,exchange_key,signing_key FROM crypto_identities ORDER BY user_id`)
	return keys, err
}

func (s *CryptoService) ChatKeys(ctx context.Context, chatID int) ([]e2ee.PublicIdentity, error) {
	keys := make([]e2ee.PublicIdentity, 0)
	err := s.db.SelectContext(ctx, &keys, `SELECT p.user_id,COALESCE(i.key_id,'') AS key_id,COALESCE(i.exchange_key,'') AS exchange_key,COALESCE(i.signing_key,'') AS signing_key
        FROM chat_participants p LEFT JOIN crypto_identities i ON i.user_id=p.user_id
        WHERE p.chat_id=$1 ORDER BY p.user_id`, chatID)
	return keys, err
}

func (s *MessageService) validateEncryption(ctx context.Context, tx *sqlx.Tx, message *models.Message, signerID, legacyID int) error {
	keys := make([]e2ee.PublicIdentity, 0)
	// Hold participant rows until commit, including against cascading account deletion.
	err := tx.SelectContext(ctx, &keys, `SELECT p.user_id,COALESCE(i.key_id,'') AS key_id,COALESCE(i.exchange_key,'') AS exchange_key,COALESCE(i.signing_key,'') AS signing_key
        FROM chat_participants p LEFT JOIN crypto_identities i ON i.user_id=p.user_id
        WHERE p.chat_id=$1 ORDER BY p.user_id FOR SHARE OF p`, message.ChatID)
	if err != nil {
		return err
	}
	envelope, err := e2ee.Validate(message.Content, message.ChatID, message.SenderID, signerID, legacyID, keys)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	message.EncryptionAlgorithm = models.EncryptionAlgorithmType(envelope.Algorithm)
	message.CryptoVersion = 1
	message.ClientID = envelope.MessageID
	return nil
}
