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
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	grpcapi "project/services/Database_Service/api/grpc"
	chatpb "project/services/Database_Service/api/grpc/pb/chat"
	chatpartpb "project/services/Database_Service/api/grpc/pb/chat_participant"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	userpb "project/services/Database_Service/api/grpc/pb/user"
	"project/services/Database_Service/api/rest"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
)

func TestMessengerIntegration(t *testing.T) {
	db := testDB(t)
	crypto := newCryptoFixture(t, db)
	access, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
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
	server := rest.NewHTTPServer(controllers.NewUserController(users, access), controllers.NewMessageController(messages, access), controllers.NewChatController(chats, access), controllers.NewChatParticipantController(participants, access), db, access, false)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	hash, err := bcrypt.GenerateFromPassword([]byte("integration-password-123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET password_hash=$1 WHERE username IN ('alice','bob')`, string(hash)); err != nil {
		t.Fatal(err)
	}
	alice, err := access.Login(context.Background(), auth.Credentials{Email: "alice@example.com", Password: "integration-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := access.Login(context.Background(), auth.Credentials{Email: "bob@example.com", Password: "integration-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	activeToken := alice.Token
	client := &http.Client{Timeout: 30 * time.Second}
	request := func(method, path, body string, want int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Messenger-Request", "1")
		if path != "/auth/register" && path != "/auth/login" {
			req.Header.Set("Authorization", "Bearer "+activeToken)
		}
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
	activeToken = "invalid"
	request("GET", "/users/all", "", 401)
	activeToken = alice.Token
	request("POST", "/users", `{"username":"legacy","email":"legacy@example.com","password_hash":"!test"}`, 403)
	data := request("GET", "/users/all", "", 200)
	if bytes.Contains(data, []byte("password_hash")) {
		t.Fatal("password hash leaked")
	}
	for _, path := range []string{"/users?id=bad", "/users?id=-1", "/messages/longpoll/0", "/messages/longpoll/1?last_id=-1"} {
		request("GET", path, "", 400)
	}
	request("GET", "/users?id=999999", "", 404)
	data = request("POST", "/auth/register", `{"username":"carol","email":"carol@example.com","password":"integration-password-123"}`, 201)
	var createdUser struct {
		User models.User `json:"user"`
	}
	if err := json.Unmarshal(data, &createdUser); err != nil {
		t.Fatal(err)
	}
	if createdUser.User.ID <= 0 || createdUser.User.CreatedAt.IsZero() || createdUser.User.PasswordHash != "" {
		t.Fatalf("invalid created user: %s", data)
	}
	request("POST", "/auth/register", `{"username":"carol","email":"carol@example.com","password":"integration-password-123"}`, 409)
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
		if id == 2 {
			activeToken = bob.Token
		} else {
			activeToken = alice.Token
		}
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
	activeToken = alice.Token
	data = request("GET", fmt.Sprintf("/chat_participant/%d", chat.ID), "", 200)
	if !bytes.Contains(data, []byte(`"chat_id":`)) {
		t.Fatalf("invalid participants: %s", data)
	}
	activeToken = bob.Token
	request("PUT", fmt.Sprintf("/chat/%d", chat.ID), `{"name":"Hacked"}`, 403)
	request("DELETE", fmt.Sprintf("/chat/%d", chat.ID), "", 403)
	request("POST", "/chat_participant", fmt.Sprintf(`{"chat_id":%d,"user_id":2,"role":"admin"}`, chat.ID), 403)
	request("GET", "/chat_participant/chats/1", "", 403)
	request("DELETE", "/users/1", "", 403)
	activeToken = alice.Token
	request("PUT", fmt.Sprintf("/chat/%d", chat.ID), `{"name":"Renamed"}`, 200)
	carol, err := access.Login(context.Background(), auth.Credentials{Email: "carol@example.com", Password: "integration-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	activeToken = carol.Token
	request("POST", "/messages", fmt.Sprintf(`{"chat_id":%d,"sender_id":1,"content":"outsider"}`, chat.ID), 403)
	request("GET", fmt.Sprintf("/messages/%d", chat.ID), "", 403)
	activeToken = alice.Token
	text := "Привет 👋 100% + <script> & перенос\nстроки"
	ciphertext := crypto.seal(t, text, chat.ID, 1, 1, 0)
	request("POST", "/messages", fmt.Sprintf(`{"chat_id":%d,"content":"plaintext forbidden"}`, chat.ID), 400)
	body, _ := json.Marshal(map[string]interface{}{"chat_id": chat.ID, "sender_id": createdUser.User.ID, "content": ciphertext})
	data = request("POST", "/messages", string(body), 201)
	var createdMessage struct {
		Message models.Message `json:"message"`
	}
	if err := json.Unmarshal(data, &createdMessage); err != nil {
		t.Fatal(err)
	}
	message := createdMessage.Message
	if message.ID <= 0 || message.SenderID != 1 || message.SentAt.IsZero() || message.Content != ciphertext || message.CryptoVersion != 1 {
		t.Fatalf("invalid message: %s", data)
	}
	data = request("GET", fmt.Sprintf("/messages/longpoll/%d?last_id=0", chat.ID), "", 200)
	var batch []models.Message
	if err := json.Unmarshal(data, &batch); err != nil || len(batch) != 1 || batch[0].Content != ciphertext {
		t.Fatalf("long poll: %s %v", data, err)
	}
	activeToken = bob.Token
	request("PUT", fmt.Sprintf("/messages/%d", message.ID), `{"content":"hacked"}`, 403)
	activeToken = alice.Token
	request("PUT", fmt.Sprintf("/messages/%d", message.ID), `{"content":"plaintext edit forbidden"}`, 400)
	edited := crypto.seal(t, "edited", chat.ID, 1, 1, 0)
	editBody, _ := json.Marshal(map[string]string{"content": edited})
	request("PUT", fmt.Sprintf("/messages/%d", message.ID), string(editBody), 200)
	// Cancellation must release a waiting request without waiting for its timeout.
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/messages/longpoll/%d?last_id=%d", ts.URL, chat.ID, message.ID), nil)
	req.Header.Set("Authorization", "Bearer "+alice.Token)
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
	gs := grpcapi.NewGRPCServer(access)
	gs.RegisterServices(users, messages, chats, participants, access)
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
	rpcCtx = metadata.AppendToOutgoingContext(rpcCtx, "authorization", "Bearer "+alice.Token)
	got, err := messagepb.NewMessageServiceClient(conn).GetMessage(rpcCtx, &messagepb.GetMessageRequest{Id: int64(message.ID)})
	if err != nil || got.GetMessage().GetContext() != edited {
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
	// Both transports must enforce the same identity and permission checks.
	_, err = messagepb.NewMessageServiceClient(conn).GetMessage(context.Background(), &messagepb.GetMessageRequest{Id: int64(message.ID)})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("gRPC anonymous access: %v", err)
	}
	bobCtx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+bob.Token)
	_, err = messagepb.NewMessageServiceClient(conn).UpdateMessage(bobCtx, &messagepb.UpdateMessageRequest{Message: &messagepb.Message{Id: int64(message.ID), Context: "hacked"}})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("gRPC author access: %v", err)
	}
	carolCtx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+carol.Token)
	_, err = messagepb.NewMessageServiceClient(conn).GetMessage(carolCtx, &messagepb.GetMessageRequest{Id: int64(message.ID)})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("gRPC outsider access: %v", err)
	}
	_, err = userpb.NewUserServiceClient(conn).GetUser(bobCtx, &userpb.GetUserRequest{Identifier: &userpb.GetUserRequest_Id{Id: 1}})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("gRPC private profile access: %v", err)
	}
	_, err = userpb.NewUserServiceClient(conn).CreateUser(rpcCtx, &userpb.CreateUserRequest{User: &userpb.User{Username: "raw_hash", PasswordHash: "forged"}})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("gRPC raw hash registration: %v", err)
	}
	for _, supplied := range []int64{0, int64(createdUser.User.ID)} {
		name := fmt.Sprintf("RPC creator %d", supplied)
		_, err = chatpb.NewChatServiceClient(conn).CreateChat(rpcCtx, &chatpb.CreateChatRequest{Chat: &chatpb.Chat{Name: name, Type: chatpb.ChatType_CHAT_TYPE_GROUP, CreatedBy: supplied}})
		if err != nil {
			t.Fatalf("gRPC create without trusted created_by: %v", err)
		}
		var persisted models.Chat
		if err := db.Get(&persisted, `SELECT id,created_by FROM chats WHERE name=$1`, name); err != nil || persisted.CreatedBy != 1 {
			t.Fatalf("gRPC creator spoofing: %+v %v", persisted, err)
		}
		request("DELETE", fmt.Sprintf("/chat/%d", persisted.ID), "", 200)
	}
	_, err = messagepb.NewMessageServiceClient(conn).CreateMessage(rpcCtx, &messagepb.CreateMessageRequest{Message: &messagepb.Message{ChatId: int64(chat.ID), Context: "plaintext forbidden"}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("gRPC accepted plaintext: %v", err)
	}
	rpcCiphertext := crypto.seal(t, "RPC sender", chat.ID, 1, 1, 0)
	_, err = messagepb.NewMessageServiceClient(conn).CreateMessage(rpcCtx, &messagepb.CreateMessageRequest{Message: &messagepb.Message{ChatId: int64(chat.ID), SenderId: int64(createdUser.User.ID), Context: rpcCiphertext}})
	if err != nil {
		t.Fatal(err)
	}
	var rpcMessage models.Message
	if err := db.Get(&rpcMessage, `SELECT id,sender_id FROM messages WHERE chat_id=$1 AND content=$2`, chat.ID, rpcCiphertext); err != nil || rpcMessage.SenderID != 1 {
		t.Fatalf("gRPC sender spoofing: %+v %v", rpcMessage, err)
	}
	// A waiting request must stop when the participant is removed.
	pollCtx, pollCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pollCancel()
	pollReq, _ := http.NewRequestWithContext(pollCtx, "GET", fmt.Sprintf("%s/messages/longpoll/%d?last_id=%d", ts.URL, chat.ID, rpcMessage.ID), nil)
	pollReq.Header.Set("Authorization", "Bearer "+bob.Token)
	pollResult := make(chan int, 1)
	go func() {
		response, err := client.Do(pollReq)
		if err != nil {
			pollResult <- 0
			return
		}
		defer response.Body.Close()
		pollResult <- response.StatusCode
	}()
	time.Sleep(100 * time.Millisecond)
	request("DELETE", fmt.Sprintf("/chat_participant/%d/1", chat.ID), "", 403)
	// Exercise participant removal, protected bulk removal and cascading chat deletion.
	request("DELETE", fmt.Sprintf("/chat_participant/%d/2", chat.ID), "", 200)
	select {
	case code := <-pollResult:
		if code != 403 {
			t.Fatalf("revoked long poll status: %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("long poll kept access after participant removal")
	}
	_, err = chatpartpb.NewChatParticipantServiceClient(conn).DeleteAllChatParticipants(rpcCtx, &chatpartpb.DeleteAllChatParticipantsRequest{ChatId: int64(chat.ID)})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bulk removal: %v", err)
	}
	request("DELETE", fmt.Sprintf("/chat/%d", chat.ID), "", 200)
	activeToken = carol.Token
	request("DELETE", fmt.Sprintf("/users/%d", createdUser.User.ID), "", 200)
}
