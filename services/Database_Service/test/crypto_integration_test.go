package test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"io"
	"net/http"
	"project/services/Database_Service/auth"
	"strings"
	"sync"
	"testing"

	"project/services/Database_Service/e2ee"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
	"project/services/Database_Service/services"
)

func TestCryptoKeyAccess(t *testing.T) {
	db := testDB(t)
	newCryptoFixture(t, db)
	access, server := authServer(t, db)
	hash, err := bcrypt.GenerateFromPassword([]byte("crypto-test-password-123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE users SET password_hash=$1`, string(hash)); err != nil {
		t.Fatal(err)
	}
	alice, err := access.Login(context.Background(), auth.Credentials{Email: "alice@example.com", Password: "crypto-test-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := access.Register(context.Background(), auth.Registration{Username: "crypto_outsider", Email: "outsider@example.com", Password: "crypto-test-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token string, body []byte, want int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Messenger-Request", "1")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, response.StatusCode, want, data)
		}
		return data
	}
	request("GET", "/crypto/identity", "", nil, 401)
	data := request("GET", "/crypto/identity?user_id=2", alice.Token, nil, 200)
	var identity e2ee.Identity
	if err = json.Unmarshal(data, &identity); err != nil || identity.UserID != 1 {
		t.Fatalf("another user's vault exposed: %s %v", data, err)
	}
	public := request("GET", "/crypto/keys", alice.Token, nil, 200)
	if bytes.Contains(public, []byte("encrypted_private_keys")) || bytes.Contains(public, []byte(identity.EncryptedPrivateKeys)) {
		t.Fatal("public directory exposed a protected vault")
	}
	request("POST", "/crypto/identity", alice.Token, data, 409)
	request("GET", "/crypto/identity", outsider.Token, nil, 404)
	request("GET", "/crypto/chat/1", outsider.Token, nil, 403)
	request("GET", "/crypto/chat/1", alice.Token, nil, 200)
	identity.Nonce = "invalid"
	invalid, _ := json.Marshal(identity)
	request("POST", "/crypto/identity", outsider.Token, invalid, 400)
}

func TestEncryptedStorageAndLegacyMigration(t *testing.T) {
	db := testDB(t)
	fixture := newCryptoFixture(t, db)
	svc, err := services.NewMessageService(db)
	if err != nil {
		t.Fatal(err)
	}
	const text = "Секретный текст 👋"
	encrypted := fixture.seal(t, text, 1, 1, 1, 0)
	message := models.Message{ChatID: 1, SenderID: 1, Content: encrypted, EncryptionAlgorithm: models.EncryptionAlgorithmKuznechik}
	if err = svc.CreateMessage(context.Background(), &message); err != nil {
		t.Fatal(err)
	}
	var stored models.Message
	if err = db.Get(&stored, `SELECT id,content,crypto_version,encryption_algorithm FROM messages WHERE id=$1`, message.ID); err != nil {
		t.Fatal(err)
	}
	if stored.Content != encrypted || strings.Contains(stored.Content, text) || stored.CryptoVersion != 1 || stored.EncryptionAlgorithm != models.EncryptionAlgorithmAES {
		t.Fatalf("unencrypted or misleading stored message: %+v", stored)
	}
	if err = svc.CreateMessage(context.Background(), &message); err == nil {
		t.Fatal("replayed client message id accepted")
	}
	if err = svc.CreateMessage(context.Background(), &models.Message{ChatID: 1, SenderID: 1, Content: "plaintext"}); err == nil {
		t.Fatal("plaintext accepted")
	}
	for _, mutate := range []func(*e2ee.Envelope){
		func(e *e2ee.Envelope) { e.ChatID = 2 },
		func(e *e2ee.Envelope) { e.SenderID = 2 },
		func(e *e2ee.Envelope) { e.Algorithm = "Kuznechik" },
		func(e *e2ee.Envelope) { e.Recipients = e.Recipients[:1] },
		func(e *e2ee.Envelope) { e.Recipients[0].KeyID = e.Recipients[1].KeyID },
		func(e *e2ee.Envelope) { e.Signature = b64Random(t, 64) },
		func(e *e2ee.Envelope) { e.Ciphertext = b64Random(t, 40) },
	} {
		var envelope e2ee.Envelope
		if err = json.Unmarshal([]byte(strings.TrimPrefix(encrypted, e2ee.Prefix)), &envelope); err != nil {
			t.Fatal(err)
		}
		mutate(&envelope)
		raw, _ := json.Marshal(envelope)
		if err = svc.CreateMessage(context.Background(), &models.Message{ChatID: 1, SenderID: 1, Content: e2ee.Prefix + string(raw)}); err == nil {
			t.Fatal("tampered encrypted envelope accepted")
		}
	}
	repoReaders := []func() ([]models.Message, error){
		func() ([]models.Message, error) { return svc.GetMessagesByChatID(context.Background(), 1) },
		func() ([]models.Message, error) { return svc.GetMessagesAfter(context.Background(), 1, 0) },
	}
	for _, read := range repoReaders {
		messages, err := read()
		if err != nil || len(messages) != 1 || messages[0].Content != encrypted || messages[0].CryptoVersion != 1 {
			t.Fatalf("encrypted read: %v %v", messages, err)
		}
	}
	var legacyID int
	if err = db.QueryRow(`INSERT INTO messages(chat_id,sender_id,content) VALUES(1,1,$1) RETURNING id`, text).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	legacyCipher := fixture.seal(t, text, 1, 1, 2, legacyID)
	digest := sha256.Sum256([]byte(text))
	previousHash := base64.StdEncoding.EncodeToString(digest[:])
	if _, err = svc.EncryptLegacyMessage(context.Background(), legacyID, 2, legacyCipher, "wrong"); err == nil {
		t.Fatal("legacy compare-and-swap bypassed")
	}
	// Concurrent migrations preserve ID, sender, timestamp and one ciphertext.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.EncryptLegacyMessage(context.Background(), legacyID, 2, legacyCipher, previousHash)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	migrated, err := svc.GetMessageByID(context.Background(), legacyID)
	if err != nil || migrated.SenderID != 1 || migrated.Content != legacyCipher || migrated.CryptoVersion != 1 || migrated.SentAt.IsZero() {
		t.Fatalf("migration lost data: %+v %v", migrated, err)
	}
	var plaintextRows int
	if err = db.Get(&plaintextRows, `SELECT COUNT(*) FROM messages WHERE content=$1`, text); err != nil || plaintextRows != 0 {
		t.Fatalf("plaintext left after migration: %d %v", plaintextRows, err)
	}
	staleRecipients := fixture.seal(t, "stale recipient", 1, 1, 1, 0)
	participants := repository.ChatParticipantRepository{}
	if err = participants.DeleteParticipant(context.Background(), db, models.ChatParticipant{ChatID: 1, UserID: 2}); err != nil {
		t.Fatal(err)
	}
	if err = svc.CreateMessage(context.Background(), &models.Message{ChatID: 1, SenderID: 1, Content: staleRecipients}); err == nil {
		t.Fatal("message for removed participant accepted")
	}
	fixture.identities = fixture.identities[:1]
	freshRecipients := fixture.seal(t, "current recipients", 1, 1, 1, 0)
	if err = svc.CreateMessage(context.Background(), &models.Message{ChatID: 1, SenderID: 1, Content: freshRecipients}); err != nil {
		t.Fatal(err)
	}
}
