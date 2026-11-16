// Package e2ee validates signed client envelopes. It never decrypts messages.
package e2ee

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
)

const Prefix = "e2ee:v1:"
const MaxContentBytes = 256 << 10
const MaxRecipients = 100

type PublicIdentity struct {
	UserID      int    `json:"user_id" db:"user_id"`
	KeyID       string `json:"key_id" db:"key_id"`
	ExchangeKey string `json:"exchange_key" db:"exchange_key"`
	SigningKey  string `json:"signing_key" db:"signing_key"`
}

type Identity struct {
	PublicIdentity
	Salt                 string `json:"salt" db:"salt"`
	Nonce                string `json:"nonce" db:"nonce"`
	EncryptedPrivateKeys string `json:"encrypted_private_keys" db:"encrypted_private_keys"`
	Iterations           int    `json:"iterations" db:"iterations"`
}

type Recipient struct {
	UserID int    `json:"user_id"`
	KeyID  string `json:"key_id"`
	Nonce  string `json:"nonce"`
	Key    string `json:"key"`
}

type Envelope struct {
	Version      int         `json:"v"`
	Algorithm    string      `json:"algorithm"`
	ChatID       int         `json:"chat_id"`
	SenderID     int         `json:"sender_id"`
	SignerID     int         `json:"signer_id"`
	MessageID    string      `json:"message_id"`
	LegacyID     int         `json:"legacy_id"`
	EphemeralKey string      `json:"ephemeral_key"`
	Nonce        string      `json:"nonce"`
	Ciphertext   string      `json:"ciphertext"`
	Tag          string      `json:"tag"`
	Recipients   []Recipient `json:"recipients"`
	Signature    string      `json:"signature"`
}

func decode(value string, size int) ([]byte, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || (size >= 0 && len(raw) != size) || base64.StdEncoding.EncodeToString(raw) != value {
		return nil, errors.New("invalid base64 or length")
	}
	return raw, nil
}

func (p PublicIdentity) Validate() error {
	exchange, err := decode(p.ExchangeKey, 65)
	if err != nil {
		return err
	}
	if _, err = ecdh.P256().NewPublicKey(exchange); err != nil {
		return err
	}
	signing, err := decode(p.SigningKey, 65)
	if err != nil {
		return err
	}
	if x, _ := elliptic.Unmarshal(elliptic.P256(), signing); x == nil {
		return errors.New("invalid signing key")
	}
	digest := sha256.Sum256(append(exchange, signing...))
	if p.KeyID != hex.EncodeToString(digest[:]) {
		return errors.New("invalid key fingerprint")
	}
	return nil
}

func (i Identity) Validate() error {
	if err := i.PublicIdentity.Validate(); err != nil {
		return err
	}
	if _, err := decode(i.Salt, 16); err != nil {
		return err
	}
	if _, err := decode(i.Nonce, 12); err != nil {
		return err
	}
	keys, err := decode(i.EncryptedPrivateKeys, -1)
	if err != nil || len(keys) < 32 || len(keys) > 4096 || i.Iterations != 600000 {
		return errors.New("invalid encrypted private keys")
	}
	return nil
}

// SigningBytes is shared with the browser protocol. Only ASCII strings and integers
// occur here, so Go and JavaScript encode this array identically.
func (e Envelope) SigningBytes() []byte {
	recipients := make([][]any, 0, len(e.Recipients))
	for _, r := range e.Recipients {
		recipients = append(recipients, []any{r.UserID, r.KeyID, r.Nonce, r.Key})
	}
	data, _ := json.Marshal([]any{e.Version, e.Algorithm, e.ChatID, e.SenderID, e.SignerID,
		e.MessageID, e.LegacyID, e.EphemeralKey, e.Nonce, e.Ciphertext, e.Tag, recipients})
	return data
}

// Validate checks metadata, recipient coverage and the sender's signature.
// Authentication of the ciphertext is also checked by each receiving browser.
func Validate(content string, chatID, senderID, signerID, legacyID int, identities []PublicIdentity) (Envelope, error) {
	var e Envelope
	if len(content) > MaxContentBytes || !strings.HasPrefix(content, Prefix) {
		return e, errors.New("end-to-end encrypted message required")
	}
	decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(content, Prefix)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil {
		return e, errors.New("invalid encrypted envelope")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return e, errors.New("trailing envelope data")
	}
	if e.Version != 1 || e.ChatID != chatID || e.SenderID != senderID || e.SignerID != signerID || e.LegacyID != legacyID {
		return e, errors.New("encrypted message metadata mismatch")
	}
	if _, err := decode(e.MessageID, 16); err != nil {
		return e, err
	}
	epk, err := decode(e.EphemeralKey, 65)
	if err != nil {
		return e, err
	}
	if _, err = ecdh.P256().NewPublicKey(epk); err != nil {
		return e, err
	}
	ciphertext, err := decode(e.Ciphertext, -1)
	if err != nil {
		return e, err
	}
	switch e.Algorithm {
	case "AES":
		if _, err = decode(e.Nonce, 12); err != nil {
			return e, err
		}
		if len(ciphertext) < 17 || len(ciphertext) > 65536+16 || e.Tag != "" {
			return e, errors.New("invalid AES-GCM payload")
		}
	case "Kuznechik":
		nonce, nonceErr := decode(e.Nonce, 16)
		if nonceErr != nil {
			return e, nonceErr
		}
		for _, b := range nonce[8:] {
			if b != 0 {
				return e, errors.New("invalid Kuznechik counter")
			}
		}
		if _, err = decode(e.Tag, 32); err != nil {
			return e, err
		}
		if len(ciphertext) < 1 || len(ciphertext) > 65536 {
			return e, errors.New("invalid Kuznechik payload")
		}
	default:
		return e, errors.New("unsupported encryption algorithm")
	}
	if len(identities) == 0 || len(identities) > MaxRecipients || len(e.Recipients) != len(identities) {
		return e, errors.New("all participants must configure encryption before sending")
	}
	var signer PublicIdentity
	for index, identity := range identities {
		r := e.Recipients[index]
		if identity.KeyID == "" || r.UserID != identity.UserID || r.KeyID != identity.KeyID || (index > 0 && r.UserID <= e.Recipients[index-1].UserID) {
			return e, errors.New("chat participants or their keys changed; refresh and retry")
		}
		if _, err = decode(r.Nonce, 12); err != nil {
			return e, err
		}
		if _, err = decode(r.Key, 48); err != nil {
			return e, err
		}
		if identity.UserID == signerID {
			signer = identity
		}
	}
	if signer.UserID == 0 {
		return e, errors.New("signer must be a chat participant")
	}
	pub, err := decode(signer.SigningKey, 65)
	if err != nil {
		return e, err
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), pub)
	if x == nil {
		return e, errors.New("invalid signing key")
	}
	sig, err := decode(e.Signature, 64)
	if err != nil {
		return e, err
	}
	digest := sha256.Sum256(e.SigningBytes())
	if !ecdsa.Verify(&ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return e, fmt.Errorf("invalid message signature")
	}
	return e, nil
}
