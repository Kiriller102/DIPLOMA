package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
)

type paginationHTTPFixture struct {
	url      string
	client   *http.Client
	crypto   *cryptoFixture
	alice    *http.Cookie
	bob      *http.Cookie
	outsider *http.Cookie
}

func newPaginationHTTPFixture(t *testing.T) *paginationHTTPFixture {
	t.Helper()
	db := testDB(t) // Separate temporary schema; no writes to the running demo.
	f := &paginationHTTPFixture{crypto: newCryptoFixture(t, db), client: &http.Client{Timeout: 30 * time.Second}}
	_, server := authServer(t, db)
	f.url = server.URL
	hash, err := bcrypt.GenerateFromPassword([]byte("pagination-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET password_hash=$1 WHERE id IN (1,2)`, string(hash)); err != nil {
		t.Fatal(err)
	}
	f.alice = f.login(t, "alice@example.com")
	f.bob = f.login(t, "bob@example.com")
	_, f.outsider = f.request(t, http.MethodPost, "/auth/register", map[string]string{
		"username": "pagination_outsider", "email": "pagination-outsider@example.com", "password": "pagination-test-password",
	}, nil, http.StatusCreated)
	if f.outsider == nil {
		t.Fatal("registration did not issue session cookie")
	}
	return f
}

func (f *paginationHTTPFixture) request(t *testing.T, method, path string, body any, cookie *http.Cookie, wantStatus int) ([]byte, *http.Cookie) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, f.url+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Messenger-Request", "1")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	response, err := f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, response.StatusCode, wantStatus, data)
	}
	var session *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "messenger_session" {
			session = cookie
		}
	}
	return data, session
}

func (f *paginationHTTPFixture) login(t *testing.T, email string) *http.Cookie {
	t.Helper()
	_, cookie := f.request(t, http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": "pagination-test-password",
	}, nil, http.StatusOK)
	if cookie == nil {
		t.Fatal("login did not issue session cookie")
	}
	return cookie
}

func (f *paginationHTTPFixture) send(t *testing.T, chatID, number int) models.Message {
	t.Helper()
	senderID, cookie := 1, f.alice
	if number%2 == 0 {
		senderID, cookie = 2, f.bob
	}
	content := f.crypto.seal(t, fmt.Sprintf("%d. Pagination integration fixture", number), chatID, senderID, senderID, 0)
	data, _ := f.request(t, http.MethodPost, "/messages", map[string]any{"chat_id": chatID, "content": content}, cookie, http.StatusCreated)
	var result struct {
		Message models.Message `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Message.ID <= 0 || result.Message.ChatID != chatID || result.Message.SenderID != senderID || result.Message.Content != content || result.Message.CryptoVersion != 1 {
		t.Fatalf("invalid created message: %+v", result.Message)
	}
	return result.Message
}

func (f *paginationHTTPFixture) chat(t *testing.T, count int) (int, []models.Message) {
	t.Helper()
	data, _ := f.request(t, http.MethodPost, "/chat", map[string]any{
		"name": fmt.Sprintf("Pagination %d", count), "type": "group", "participant_ids": []int{2},
	}, f.alice, http.StatusCreated)
	var result struct {
		Chat models.Chat `json:"chat"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.Chat.ID <= 0 {
		t.Fatalf("invalid chat: %s, err=%v", data, err)
	}
	messages := make([]models.Message, 0, count)
	for i := 1; i <= count; i++ {
		messages = append(messages, f.send(t, result.Chat.ID, i))
	}
	return result.Chat.ID, messages
}

func (f *paginationHTTPFixture) page(t *testing.T, path string, cookie *http.Cookie) services.MessageHistoryPage {
	t.Helper()
	data, _ := f.request(t, http.MethodGet, path, nil, cookie, http.StatusOK)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 2 || fields["messages"] == nil || fields["next_cursor"] == nil {
		t.Fatalf("invalid page contract: %s, err=%v", data, err)
	}
	var page services.MessageHistoryPage
	if err := json.Unmarshal(data, &page); err != nil || page.Messages == nil {
		t.Fatalf("invalid page: %s, err=%v", data, err)
	}
	return page
}

func assertPaginationMessages(t *testing.T, got, want []models.Message) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("message count=%d, want=%d", len(got), len(want))
	}
	for i, message := range got {
		expected := want[i]
		if message.ID != expected.ID || message.ChatID != expected.ChatID || message.SenderID != expected.SenderID || message.Content != expected.Content || message.CryptoVersion != expected.CryptoVersion || message.EncryptionAlgorithm != expected.EncryptionAlgorithm || message.SentAt.IsZero() {
			t.Fatalf("message[%d] identity, order or stored content changed: id=%d, want=%d", i, message.ID, expected.ID)
		}
	}
}

func TestMessagePaginationRESTPostgres(t *testing.T) {
	f := newPaginationHTTPFixture(t)
	// Keep previous chats populated to detect missing chat_id filtering, including
	// the empty-chat case. Both participants use real HTTP login session cookies.
	for _, count := range []int{49, 50, 51, 125, 0} {
		t.Run(fmt.Sprintf("messages=%d", count), func(t *testing.T) {
			chatID, expected := f.chat(t, count)
			for _, user := range []struct {
				name   string
				cookie *http.Cookie
			}{{"alice", f.alice}, {"bob", f.bob}} {
				t.Run(user.name, func(t *testing.T) {
					path := fmt.Sprintf("/messages/%d", chatID)
					end, requests := count, 0
					seen := make(map[int]bool)
					for {
						page := f.page(t, path, user.cookie)
						requests++
						start := max(0, end-50)
						assertPaginationMessages(t, page.Messages, expected[start:end])
						for _, message := range page.Messages {
							if seen[message.ID] {
								t.Fatalf("duplicate ID %d", message.ID)
							}
							seen[message.ID] = true
						}
						if start == 0 {
							if page.NextCursor != nil {
								t.Fatalf("unexpected cursor at history end: %d", *page.NextCursor)
							}
							break
						}
						if page.NextCursor == nil || *page.NextCursor != expected[start].ID {
							t.Fatalf("wrong page boundary: %+v", page.NextCursor)
						}
						path = fmt.Sprintf("/messages/%d?before_id=%d", chatID, *page.NextCursor)
						end = start
						if requests >= max(1, (count+49)/50) {
							t.Fatal("pagination did not terminate")
						}
					}
					if len(seen) != count || requests != max(1, (count+49)/50) {
						t.Fatalf("read %d messages in %d requests", len(seen), requests)
					}
				})
			}
		})
	}

	t.Run("unauthorized access", func(t *testing.T) {
		chatID, expected := f.chat(t, 51)
		first := fmt.Sprintf("/messages/%d", chatID)
		page := f.page(t, first, f.alice)
		if page.NextCursor == nil {
			t.Fatal("expected next page")
		}
		next := first + "?before_id=" + strconv.Itoa(*page.NextCursor)
		for _, tc := range []struct {
			name   string
			cookie *http.Cookie
			status int
		}{
			{"no session", nil, http.StatusUnauthorized},
			{"malformed session", &http.Cookie{Name: "messenger_session", Value: "invalid"}, http.StatusUnauthorized},
			{"unknown session", &http.Cookie{Name: "messenger_session", Value: strings.Repeat("A", 43)}, http.StatusUnauthorized},
			{"nonparticipant", f.outsider, http.StatusForbidden},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for _, path := range []string{first, next, next + "&user_id=1&sender_id=1"} {
					data, _ := f.request(t, http.MethodGet, path, nil, tc.cookie, tc.status)
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 1 || fields["error"] == nil || bytes.Contains(data, []byte(expected[0].Content)) {
						t.Fatalf("history exposed in rejected response: %s, err=%v", data, err)
					}
				}
			})
		}
	})

	t.Run("removed participant cannot read next page", func(t *testing.T) {
		chatID, _ := f.chat(t, 51)
		path := fmt.Sprintf("/messages/%d", chatID)
		page := f.page(t, path, f.bob)
		if page.NextCursor == nil {
			t.Fatal("expected next page")
		}
		f.request(t, http.MethodDelete, fmt.Sprintf("/chat_participant/%d/2", chatID), nil, f.alice, http.StatusOK)
		f.request(t, http.MethodGet, fmt.Sprintf("%s?before_id=%d", path, *page.NextCursor), nil, f.bob, http.StatusForbidden)
		f.request(t, http.MethodGet, path, nil, f.bob, http.StatusForbidden)
		f.page(t, path, f.alice)
	})

	t.Run("revoked session cannot read next page", func(t *testing.T) {
		chatID, _ := f.chat(t, 51)
		cookie := f.login(t, "bob@example.com")
		path := fmt.Sprintf("/messages/%d", chatID)
		page := f.page(t, path, cookie)
		if page.NextCursor == nil {
			t.Fatal("expected next page")
		}
		f.request(t, http.MethodPost, "/auth/logout", nil, cookie, http.StatusNoContent)
		f.request(t, http.MethodGet, fmt.Sprintf("%s?before_id=%d", path, *page.NextCursor), nil, cookie, http.StatusUnauthorized)
	})

	t.Run("new message between pages", func(t *testing.T) {
		chatID, expected := f.chat(t, 51)
		path := fmt.Sprintf("/messages/%d", chatID)
		first := f.page(t, path, f.alice)
		if first.NextCursor == nil {
			t.Fatal("expected next page")
		}
		assertPaginationMessages(t, first.Messages, expected[1:])
		newMessage := f.send(t, chatID, 52)
		older := f.page(t, fmt.Sprintf("%s?before_id=%d", path, *first.NextCursor), f.alice)
		assertPaginationMessages(t, older.Messages, expected[:1])
		if older.NextCursor != nil {
			t.Fatal("expected history end")
		}
		// The existing polling API supplies the new message using the initial
		// page's maximum ID; history pagination must not consume or duplicate it.
		data, _ := f.request(t, http.MethodGet, fmt.Sprintf("/messages/longpoll/%d?last_id=%d", chatID, expected[50].ID), nil, f.alice, http.StatusOK)
		var polled []models.Message
		if err := json.Unmarshal(data, &polled); err != nil {
			t.Fatal(err)
		}
		assertPaginationMessages(t, polled, []models.Message{newMessage})
	})

	t.Run("invalid cursor", func(t *testing.T) {
		chatID, _ := f.chat(t, 0)
		for _, query := range []string{"before_id=", "before_id=abc", "before_id=0", "before_id=-1", "before_id=1.5", "before_id=999999999999999999999999", "before_id=1&before_id=2"} {
			f.request(t, http.MethodGet, fmt.Sprintf("/messages/%d?%s", chatID, query), nil, f.alice, http.StatusBadRequest)
		}
	})
}
