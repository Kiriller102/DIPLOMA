package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
	"project/services/Database_Service/api/rest"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/services"
)

func authServer(t *testing.T, db *sqlx.DB) (*auth.Service, *httptest.Server) {
	t.Helper()
	access, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	users, err := services.NewUserService(db)
	if err != nil {
		t.Fatal(err)
	}
	chats, err := services.NewChatService(db)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := services.NewMessageService(db)
	if err != nil {
		t.Fatal(err)
	}
	participants, err := services.NewChatParticipantService(db)
	if err != nil {
		t.Fatal(err)
	}
	server := rest.NewHTTPServer(controllers.NewUserController(users, access), controllers.NewMessageController(messages, access), controllers.NewChatController(chats, access), controllers.NewChatParticipantController(participants, access), db, access, false)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return access, ts
}

func TestLocalSessions(t *testing.T) {
	db := testDB(t)
	access, ts := authServer(t, db)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	request := func(method, path, body string, headers map[string]string, want int) ([]byte, []*http.Cookie) {
		t.Helper()
		req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Messenger-Request", "1")
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.StatusCode, want, data)
		}
		return data, response.Cookies()
	}
	request("GET", "/users/all", "", nil, 401)
	request("POST", "/auth/register", `{"username":"short","email":"short@example.com","password":"123"}`, nil, 400)
	request("POST", "/auth/register", `{"username":"bad_fields","email":"bad@example.com","password":"safe-password-123","password_hash":"forged"}`, nil, 400)
	request("POST", "/auth/register", `{"username":"csrf_user","email":"csrf@example.com","password":"safe-password-123"}`, map[string]string{"X-Messenger-Request": ""}, 403)
	request("POST", "/auth/register", `{"username":"csrf_user","email":"csrf@example.com","password":"safe-password-123"}`, map[string]string{"Origin": "https://attacker.example"}, 403)
	data, cookies := request("POST", "/auth/register", `{"username":"session_user","email":" Session@Example.com ","password":"safe-password-123","first_name":"Тест"}`, nil, 201)
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" || cookies[0].MaxAge != 86400 {
		t.Fatalf("session cookie attributes: %+v", cookies)
	}
	firstToken := cookies[0].Value
	if bytes.Contains(data, []byte("password")) || bytes.Contains(data, []byte(firstToken)) {
		t.Fatalf("session or password exposed in registration response: %s", data)
	}
	var session struct {
		User struct {
			ID    int    `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(data, &session); err != nil || session.User.ID <= 0 || session.User.Email != "session@example.com" {
		t.Fatalf("registration response: %s %v", data, err)
	}
	var storedHash string
	if err := db.Get(&storedHash, `SELECT password_hash FROM users WHERE id=$1`, session.User.ID); err != nil {
		t.Fatal(err)
	}
	if storedHash == "safe-password-123" || bcrypt.CompareHashAndPassword([]byte(storedHash), []byte("safe-password-123")) != nil {
		t.Fatal("password was not hashed correctly")
	}
	var storedToken []byte
	if err := db.Get(&storedToken, `SELECT token_hash FROM user_sessions WHERE user_id=$1`, session.User.ID); err != nil || len(storedToken) != 32 || string(storedToken) == firstToken {
		t.Fatalf("stored session hash: %v", err)
	}
	request("GET", "/auth/me", "", nil, 200)
	request("GET", "/auth/me", "", map[string]string{"Authorization": "Bearer forged"}, 401)
	directory, _ := request("GET", "/users/all", "", nil, 200)
	if bytes.Contains(directory, []byte("session@example.com")) || bytes.Contains(directory, []byte(storedHash)) {
		t.Fatal("directory exposed private credentials")
	}
	request("POST", "/auth/register", `{"username":"another_user","email":"SESSION@example.com","password":"safe-password-123"}`, nil, 409)
	wrong, _ := request("POST", "/auth/login", `{"email":"session@example.com","password":"wrong-password"}`, nil, 401)
	missing, _ := request("POST", "/auth/login", `{"email":"missing@example.com","password":"wrong-password"}`, nil, 401)
	if !bytes.Equal(wrong, missing) {
		t.Fatal("login revealed whether account exists")
	}
	// Legacy demo hashes must not authenticate against the dummy timing hash.
	request("POST", "/auth/login", `{"email":"alice@example.com","password":"dummy-password-never-used"}`, nil, 401)
	_, cookies = request("POST", "/auth/login", `{"email":" SESSION@EXAMPLE.COM ","password":"safe-password-123"}`, nil, 200)
	if len(cookies) != 1 || cookies[0].Value == firstToken {
		t.Fatal("login did not rotate the browser session")
	}
	request("GET", "/auth/me", "", map[string]string{"Authorization": "Bearer " + firstToken}, 401)
	secondToken := cookies[0].Value
	_, cookies = request("POST", "/auth/logout", "", nil, 204)
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatal("logout did not clear cookie")
	}
	request("GET", "/auth/me", "", nil, 401)
	request("GET", "/auth/me", "", map[string]string{"Authorization": "Bearer " + secondToken}, 401)
	request("POST", "/auth/logout", "", nil, 204)
	data, _ = request("POST", "/auth/token", `{"email":"session@example.com","password":"safe-password-123"}`, nil, 200)
	var bearer struct {
		Token string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &bearer); err != nil || bearer.Token == "" {
		t.Fatalf("bearer response: %v", err)
	}
	request("GET", "/auth/me", "", map[string]string{"Authorization": "Bearer " + bearer.Token}, 200)
	if _, err := db.Exec(`UPDATE user_sessions SET expires_at=NOW()-INTERVAL '1 second' WHERE user_id=$1`, session.User.ID); err != nil {
		t.Fatal(err)
	}
	request("GET", "/auth/me", "", map[string]string{"Authorization": "Bearer " + bearer.Token}, 401)
	active, err := access.Login(context.Background(), auth.Credentials{Email: "session@example.com", Password: "safe-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET is_active=false WHERE id=$1`, session.User.ID); err != nil {
		t.Fatal(err)
	}
	request("GET", "/auth/me", "", map[string]string{"Authorization": "Bearer " + active.Token}, 403)
	request("POST", "/auth/login", `{"email":"session@example.com","password":"safe-password-123"}`, nil, 401)
	for i := 0; i < 10; i++ {
		request("POST", "/auth/register", `{"username":"rate_limit","email":"limit@example.com","password":"short"}`, nil, 400)
	}
	request("POST", "/auth/register", `{"username":"rate_limit","email":"limit@example.com","password":"short"}`, nil, 429)
}
