package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	grpcapi "project/services/Database_Service/api/grpc"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	userpb "project/services/Database_Service/api/grpc/pb/user"
	"project/services/Database_Service/api/rest"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/migrations"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
)

func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_messenger_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(4)
	for i := 0; i < 2; i++ {
		if err := migrations.Apply(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		if err := migrations.SeedDemo(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM chat_participants"); err != nil || count != 2 {
		t.Fatalf("idempotent seeding: participants=%d, err=%v", count, err)
	}
	return db
}

func TestMessengerIntegration(t *testing.T) {
	db := testDB(t)
	users, err := services.NewUserService(db)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := services.NewMessageService(db)
	if err != nil {
		t.Fatal(err)
	}
	chats, err := services.NewChatService(db)
	if err != nil {
		t.Fatal(err)
	}
	participants, err := services.NewChatParticipantService(db)
	if err != nil {
		t.Fatal(err)
	}
	server := rest.NewHTTPServer(controllers.NewUserController(users), controllers.NewMessageController(messages), controllers.NewChatController(chats), controllers.NewChatParticipantController(participants), db)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	client := &http.Client{Timeout: 5 * time.Second}
	request := func(method, path, body string, want int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, resp.StatusCode, want, data)
		}
		return data
	}
	request("GET", "/healthz", "", 200)
	data := request("GET", "/users/all", "", 200)
	if bytes.Contains(data, []byte("password_hash")) {
		t.Fatal("password hash leaked")
	}
	for _, path := range []string{"/users?id=bad", "/users?id=-1", "/messages/longpoll/0", "/messages/longpoll/1?last_id=-1"} {
		request("GET", path, "", 400)
	}
	request("GET", "/users?id=999999", "", 404)
	data = request("POST", "/users", `{"username":"carol","email":"carol@example.com","password_hash":"!test","is_active":true}`, 201)
	var createdUser struct {
		User models.User `json:"user"`
	}
	if err := json.Unmarshal(data, &createdUser); err != nil {
		t.Fatal(err)
	}
	if createdUser.User.ID <= 0 || createdUser.User.CreatedAt.IsZero() || createdUser.User.PasswordHash != "" {
		t.Fatalf("invalid created user: %s", data)
	}
	request("POST", "/users", `{"username":"carol","email":"carol@example.com","password_hash":"!test"}`, 409)
	request("POST", "/chat", `{"name":"invalid","type":"invalid","created_by":1}`, 400)
	data = request("POST", "/chat", `{"name":"Integration","type":"group","created_by":1,"participant_ids":[2]}`, 201)
	var createdChat struct {
		Chat models.Chat `json:"chat"`
	}
	if err := json.Unmarshal(data, &createdChat); err != nil {
		t.Fatal(err)
	}
	chat := createdChat.Chat
	if chat.ID <= 0 || chat.CreatedAt.IsZero() {
		t.Fatalf("invalid chat: %s", data)
	}
	for _, id := range []int{1, 2} {
		data = request("GET", fmt.Sprintf("/chat_participant/chats/%d", id), "", 200)
		var list []models.Chat
		if err := json.Unmarshal(data, &list); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range list {
			if c.ID == chat.ID && c.CreatedBy == 1 && !c.CreatedAt.IsZero() {
				found = true
			}
		}
		if !found {
			t.Fatalf("chat missing or incorrectly scanned: %s", data)
		}
	}
	data = request("GET", fmt.Sprintf("/chat_participant/%d", chat.ID), "", 200)
	if !bytes.Contains(data, []byte(`"chat_id":`)) {
		t.Fatalf("invalid participants: %s", data)
	}
	request("PUT", fmt.Sprintf("/chat/%d", chat.ID), `{"name":"Renamed"}`, 200)
	request("POST", "/messages", fmt.Sprintf(`{"chat_id":%d,"sender_id":%d,"content":"outsider"}`, chat.ID, createdUser.User.ID), 400)
	text := "Привет 👋 100% + <script> & перенос\nстроки"
	body, _ := json.Marshal(map[string]interface{}{"chat_id": chat.ID, "sender_id": 1, "content": text})
	data = request("POST", "/messages", string(body), 201)
	var createdMessage struct {
		Message models.Message `json:"message"`
	}
	if err := json.Unmarshal(data, &createdMessage); err != nil {
		t.Fatal(err)
	}
	message := createdMessage.Message
	if message.ID <= 0 || message.SentAt.IsZero() || message.Content != text {
		t.Fatalf("invalid message: %s", data)
	}
	data = request("GET", fmt.Sprintf("/messages/longpoll/%d?last_id=0", chat.ID), "", 200)
	var batch []models.Message
	if err := json.Unmarshal(data, &batch); err != nil || len(batch) != 1 || batch[0].Content != text {
		t.Fatalf("long poll: %s %v", data, err)
	}
	request("PUT", fmt.Sprintf("/messages/%d", message.ID), `{"content":"edited"}`, 200)
	// Cancellation must release a waiting request without waiting for its timeout.
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/messages/longpoll/%d?last_id=%d", ts.URL, chat.ID, message.ID), nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, _ := client.Do(req)
		if response != nil {
			response.Body.Close()
		}
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("long poll ignored cancellation")
	}
	// Exercise the same data through the actual gRPC transport.
	gs := grpcapi.NewGRPCServer()
	gs.RegisterServices(users, messages, chats, participants)
	listener := bufconn.Listen(1 << 20)
	go gs.Serve(listener)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		gs.Shutdown(ctx)
		listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rpcCancel()
	got, err := messagepb.NewMessageServiceClient(conn).GetMessage(rpcCtx, &messagepb.GetMessageRequest{Id: int64(message.ID)})
	if err != nil || got.GetMessage().GetContext() != "edited" {
		t.Fatalf("gRPC GetMessage: %v %v", got, err)
	}
	_, err = messagepb.NewMessageServiceClient(conn).GetMessage(rpcCtx, &messagepb.GetMessageRequest{Id: 999999})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("gRPC missing message: %v", err)
	}
	_, err = userpb.NewUserServiceClient(conn).GetUser(rpcCtx, &userpb.GetUserRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("gRPC invalid request: %v", err)
	}
	gotUser, err := userpb.NewUserServiceClient(conn).GetUser(rpcCtx, &userpb.GetUserRequest{Identifier: &userpb.GetUserRequest_Id{Id: 1}})
	if err != nil || gotUser.GetUser().GetPasswordHash() != "" {
		t.Fatalf("gRPC user hash leak: %v %v", gotUser, err)
	}
	// Exercise participant removal, bulk removal and cascading chat deletion.
	request("DELETE", fmt.Sprintf("/chat_participant/%d/2", chat.ID), "", 200)
	if err := participants.DeleteAllParticipantsByChatID(chat.ID); err != nil {
		t.Fatal(err)
	}
	request("DELETE", fmt.Sprintf("/chat/%d", chat.ID), "", 200)
	request("DELETE", fmt.Sprintf("/users/%d", createdUser.User.ID), "", 200)
}
