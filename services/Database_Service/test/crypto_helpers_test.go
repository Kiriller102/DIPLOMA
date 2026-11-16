package test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"testing"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/hkdf"
	"project/services/Database_Service/e2ee"
)

type cryptoFixture struct {
	identities []e2ee.PublicIdentity
	signing    map[int]*ecdsa.PrivateKey
}

func newCryptoFixture(t *testing.T, db *sqlx.DB) *cryptoFixture {
	t.Helper()
	f := &cryptoFixture{signing: make(map[int]*ecdsa.PrivateKey)}
	for _, id := range []int{1, 2} {
		exchange, err := ecdh.P256().GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signing, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pub := elliptic.Marshal(elliptic.P256(), signing.X, signing.Y)
		digest := sha256.Sum256(append(exchange.PublicKey().Bytes(), pub...))
		identity := e2ee.PublicIdentity{UserID: id, KeyID: hex.EncodeToString(digest[:]), ExchangeKey: base64.StdEncoding.EncodeToString(exchange.PublicKey().Bytes()), SigningKey: base64.StdEncoding.EncodeToString(pub)}
		f.identities = append(f.identities, identity)
		f.signing[id] = signing
		// Test-only opaque vault. Browser E2E exercises real protected PKCS8 keys.
		if _, err = db.Exec(`INSERT INTO crypto_identities(user_id,key_id,exchange_key,signing_key,salt,nonce,encrypted_private_keys,iterations) VALUES($1,$2,$3,$4,$5,$6,$7,600000)`, id, identity.KeyID, identity.ExchangeKey, identity.SigningKey, b64Random(t, 16), b64Random(t, 12), b64Random(t, 32)); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func b64Random(t *testing.T, size int) string {
	t.Helper()
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func (f *cryptoFixture) seal(t *testing.T, text string, chatID, senderID, signerID, legacyID int) string {
	t.Helper()
	e := e2ee.Envelope{Version: 1, Algorithm: "AES", ChatID: chatID, SenderID: senderID, SignerID: signerID, MessageID: b64Random(t, 16), LegacyID: legacyID, Nonce: b64Random(t, 12), Recipients: make([]e2ee.Recipient, 0)}
	header := []any{1, "AES", chatID, senderID, signerID, e.MessageID, legacyID}
	aad, _ := json.Marshal(header)
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(seed)
	aead, _ := cipher.NewGCM(block)
	nonce, _ := base64.StdEncoding.DecodeString(e.Nonce)
	e.Ciphertext = base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, []byte(text), aad))
	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	e.EphemeralKey = base64.StdEncoding.EncodeToString(ephemeral.PublicKey().Bytes())
	salt, _ := base64.StdEncoding.DecodeString(e.MessageID)
	for _, identity := range f.identities {
		r := e2ee.Recipient{UserID: identity.UserID, KeyID: identity.KeyID, Nonce: b64Random(t, 12)}
		pub, _ := base64.StdEncoding.DecodeString(identity.ExchangeKey)
		key, _ := ecdh.P256().NewPublicKey(pub)
		shared, _ := ephemeral.ECDH(key)
		derived := make([]byte, 32)
		if _, err := io.ReadFull(hkdf.New(sha256.New, shared, salt, []byte("DIPLOMA:message-key-wrap:v1")), derived); err != nil {
			t.Fatal(err)
		}
		wrappingBlock, _ := aes.NewCipher(derived)
		wrapping, _ := cipher.NewGCM(wrappingBlock)
		wrapNonce, _ := base64.StdEncoding.DecodeString(r.Nonce)
		wrapHeader := append(append([]any{}, header...), r.UserID, r.KeyID)
		wrapAAD, _ := json.Marshal(wrapHeader)
		r.Key = base64.StdEncoding.EncodeToString(wrapping.Seal(nil, wrapNonce, seed, wrapAAD))
		e.Recipients = append(e.Recipients, r)
	}
	digest := sha256.Sum256(e.SigningBytes())
	r, s, err := ecdsa.Sign(rand.Reader, f.signing[signerID], digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	e.Signature = base64.StdEncoding.EncodeToString(sig)
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return e2ee.Prefix + string(raw)
}
