package test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"errors"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	chatpb "project/services/Database_Service/api/grpc/pb/chat"
	cppb "project/services/Database_Service/api/grpc/pb/chat_participant"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	userpb "project/services/Database_Service/api/grpc/pb/user"
	gs "project/services/Database_Service/api/grpc/server"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/e2ee"
	"project/services/Database_Service/models"
)

type cancellationCase struct {
	name, method, path, route, body, stage, defect string
	http                                           gin.HandlerFunc
	grpc                                           func(context.Context) error
	barrier                                        func(*testing.T) sqlBarrier
	deadline                                       bool
}

func jsonBody(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func cancellationCases(f *cancellationFixture) []cancellationCase {
	uc := controllers.NewUserController(f.users, f.access)
	cc := controllers.NewChatController(f.chats, f.access)
	pc := controllers.NewChatParticipantController(f.participants, f.access)
	mc := controllers.NewMessageController(f.messages, f.access)
	ug := gs.NewUserGRPCsService(f.users, f.access)
	cg := gs.NewChatGRPCService(f.chats, f.access)
	pg := gs.NewChatParticipantGRPCService(f.participants, f.access)
	mg := gs.NewMessageGRPCService(f.messages, f.access)
	read := func(table, stage string) func(*testing.T) sqlBarrier {
		return func(t *testing.T) sqlBarrier { return f.readBarrier(t, table, stage) }
	}
	write := func(table, event string) func(*testing.T) sqlBarrier {
		return func(t *testing.T) sqlBarrier { return f.advisoryBarrier(t, table, event, "true") }
	}
	row := func(table, where string) func(*testing.T) sqlBarrier {
		return func(t *testing.T) sqlBarrier { return f.rowBarrier(t, table, where) }
	}
	msg := &messagepb.Message{ChatId: 1, SenderId: 1, Context: f.content, EncryptionAlgorithm: messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_AES}
	update := &messagepb.Message{Id: int64(f.messageID), Context: f.content, EncryptionAlgorithm: messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_AES}
	cp := &cppb.ChatParticipant{ChatId: 1, UserId: 2, UserRole: cppb.UserRoleType_USER_ROLE_MEMBER}
	cases := []cancellationCase{
		{name: "users_list", method: "GET", path: "/users/all", route: "/users/all", http: uc.GetUsers, stage: "FROM users", barrier: read("users", "FROM users"), grpc: func(ctx context.Context) error { _, e := ug.GetAllUsers(ctx, &emptypb.Empty{}); return e }, defect: "GetAllUsers -> Query without context"},
		{name: "user_id", method: "GET", path: "/users?id=1", route: "/users", http: uc.GetUser, stage: "FROM users WHERE id", barrier: read("users", "FROM users WHERE id"), grpc: func(ctx context.Context) error {
			_, e := ug.GetUser(ctx, &userpb.GetUserRequest{Identifier: &userpb.GetUserRequest_Id{Id: 1}})
			return e
		}, defect: "GetUser -> GetUserByID without context", deadline: true},
		{name: "user_email", method: "GET", path: "/users?email=alice@example.com", route: "/users", http: uc.GetUser, stage: "FROM users WHERE email", barrier: read("users", "FROM users WHERE email"), grpc: func(ctx context.Context) error {
			_, e := ug.GetUser(ctx, &userpb.GetUserRequest{Identifier: &userpb.GetUserRequest_Email{Email: "alice@example.com"}})
			return e
		}, defect: "GetUser -> GetUserByEmail without context"},
		{name: "user_username", method: "GET", path: "/users?username=alice", route: "/users", http: uc.GetUser, stage: "FROM users WHERE username", barrier: read("users", "FROM users WHERE username"), grpc: func(ctx context.Context) error {
			_, e := ug.GetUser(ctx, &userpb.GetUserRequest{Identifier: &userpb.GetUserRequest_Username{Username: "alice"}})
			return e
		}, defect: "GetUser -> GetUserByUsername without context"},
		{name: "user_update", method: "PUT", path: "/users/1", route: "/users/:id", body: `{"first_name":"changed"}`, http: uc.UpdateUser, stage: "UPDATE users", barrier: write("users", "UPDATE"), grpc: func(ctx context.Context) error {
			_, e := ug.UpdateUser(ctx, &userpb.UpdateUserRequest{User: &userpb.User{Id: 1, FirstName: "changed"}})
			return e
		}, defect: "UpdateUser -> Exec without context"},
		{name: "user_delete", method: "DELETE", path: "/users/1", route: "/users/:id", http: uc.DeleteUser, stage: "DELETE FROM users", barrier: write("users", "DELETE"), grpc: func(ctx context.Context) error {
			_, e := ug.DeleteUser(ctx, &userpb.DeleteUserRequest{Id: 1})
			return e
		}, defect: "DeleteUser -> Exec without context"},
		{name: "chat_create_partial", method: "POST", path: "/chat", route: "/chat", body: `{"name":"cancel-me","type":"group","participant_ids":[2]}`, http: cc.CreateChat, stage: "INSERT INTO chat_participants", barrier: write("chat_participants", "INSERT"), grpc: func(ctx context.Context) error {
			_, e := cg.CreateChat(ctx, &chatpb.CreateChatRequest{Chat: &chatpb.Chat{Name: "cancel-me", Type: chatpb.ChatType_CHAT_TYPE_GROUP}})
			return e
		}, defect: "CreateChat Beginx/INSERT after chat insert without context", deadline: true},
		{name: "chat_read", method: "GET", path: "/chat/1", route: "/chat/:id", http: cc.GetChat, stage: "SELECT id, name, type, created_by, created_at FROM chats", barrier: read("chats", "SELECT id, name, type, created_by, created_at FROM chats"), grpc: func(ctx context.Context) error { _, e := cg.GetChat(ctx, &chatpb.GetChatRequest{Id: 1}); return e }, defect: "GetChatByID -> Get without context"},
		{name: "chat_update", method: "PUT", path: "/chat/1", route: "/chat/:id", body: `{"name":"changed"}`, http: cc.UpdateChat, stage: "UPDATE chats", barrier: write("chats", "UPDATE"), grpc: func(ctx context.Context) error {
			_, e := cg.UpdateChat(ctx, &chatpb.UpdateChatRequest{Chat: &chatpb.Chat{Id: 1, Name: "changed"}})
			return e
		}, defect: "UpdateChat -> Exec without context"},
		{name: "chat_delete", method: "DELETE", path: "/chat/1", route: "/chat/:id", http: cc.DeleteChat, stage: "DELETE FROM chats", barrier: write("chats", "DELETE"), grpc: func(ctx context.Context) error {
			_, e := cg.DeleteChat(ctx, &chatpb.DeleteChatRequest{Id: 1})
			return e
		}, defect: "DeleteChatByID -> Exec without context"},
		{name: "participants_list", method: "GET", path: "/chat_participant/1", route: "/chat_participant/:chat_id", http: pc.GetChatParticipantsByChatID, stage: "SELECT chat_id, user_id, role, joined_at", barrier: read("chat_participants", "SELECT chat_id, user_id, role, joined_at"), grpc: func(ctx context.Context) error {
			_, e := pg.GetChatParticipants(ctx, &cppb.GetChatParticipantsByChatIDRequest{ChatId: 1})
			return e
		}, defect: "GetAllChatParticipants -> Select without context"},
		{name: "user_chats", method: "GET", path: "/chat_participant/chats/1", route: "/chat_participant/chats/:user_id", http: pc.GetAllChatsByUserID, stage: "SELECT c.id, c.name", barrier: read("chats", "SELECT c.id, c.name"), defect: "GetChatsIdsByUserID -> Select without context"},
		{name: "participant_create_lock", method: "POST", path: "/chat_participant", route: "/chat_participant", body: `{"chat_id":1,"user_id":2,"role":"member"}`, http: pc.CreateChatParticipant, stage: "SELECT id FROM chats", barrier: row("chats", "id=1"), grpc: func(ctx context.Context) error {
			_, e := pg.CreateChatParticipant(ctx, &cppb.CreateChatParticipantRequest{ChatParticipant: cp})
			return e
		}, defect: "CreateParticipant Beginx -> LockChat without context"},
		{name: "participant_delete_lock", method: "DELETE", path: "/chat_participant/1/2", route: "/chat_participant/:chat_id/:user_id", http: pc.DeleteChatParticipantByUserID, stage: "SELECT id FROM chats", barrier: row("chats", "id=1"), grpc: func(ctx context.Context) error {
			_, e := pg.DeleteChatParticipant(ctx, &cppb.DeleteChatParticipantRequest{ChatParticipant: cp})
			return e
		}, defect: "DeleteParticipant Beginx -> LockChat without context"},
		{name: "participant_delete_write", method: "DELETE", path: "/chat_participant/1/2", route: "/chat_participant/:chat_id/:user_id", http: pc.DeleteChatParticipantByUserID, stage: "DELETE FROM chat_participants", barrier: write("chat_participants", "DELETE"), grpc: func(ctx context.Context) error {
			_, e := pg.DeleteChatParticipant(ctx, &cppb.DeleteChatParticipantRequest{ChatParticipant: cp})
			return e
		}, defect: "DeleteParticipant Exec after LockChat without context"},
		{name: "message_create_lock", method: "POST", path: "/messages", route: "/messages", body: jsonBody(models.Message{ChatID: 1, Content: f.content}), http: mc.CreateMessage, stage: "SELECT id FROM chats", barrier: row("chats", "id=1"), grpc: func(ctx context.Context) error {
			_, e := mg.CreateMessage(ctx, &messagepb.CreateMessageRequest{Message: msg})
			return e
		}, defect: "CreateMessage Beginx -> LockChat without context"},
		{name: "message_create_keys", method: "POST", path: "/messages", route: "/messages", body: jsonBody(models.Message{ChatID: 1, Content: f.content}), http: mc.CreateMessage, stage: "FOR SHARE OF p", barrier: row("chat_participants", "chat_id=1 AND user_id=2"), grpc: func(ctx context.Context) error {
			_, e := mg.CreateMessage(ctx, &messagepb.CreateMessageRequest{Message: msg})
			return e
		}, defect: "validateEncryption Select FOR SHARE without context", deadline: true},
		{name: "message_create_write", method: "POST", path: "/messages", route: "/messages", body: jsonBody(models.Message{ChatID: 1, Content: f.content}), http: mc.CreateMessage, stage: "INSERT INTO messages", barrier: write("messages", "INSERT"), grpc: func(ctx context.Context) error {
			_, e := mg.CreateMessage(ctx, &messagepb.CreateMessageRequest{Message: msg})
			return e
		}, defect: "CreateMessage QueryRowx without context"},
		{name: "message_update_lock", method: "PUT", path: fmt.Sprintf("/messages/%d", f.messageID), route: "/messages/:id", body: jsonBody(map[string]string{"content": f.content}), http: mc.UpdateMessage, stage: "SELECT id FROM chats", barrier: row("chats", "id=1"), grpc: func(ctx context.Context) error {
			_, e := mg.UpdateMessage(ctx, &messagepb.UpdateMessageRequest{Message: update})
			return e
		}, defect: "UpdateMessage Beginx -> LockChat without context"},
		{name: "message_update_keys", method: "PUT", path: fmt.Sprintf("/messages/%d", f.messageID), route: "/messages/:id", body: jsonBody(map[string]string{"content": f.content}), http: mc.UpdateMessage, stage: "FOR SHARE OF p", barrier: row("chat_participants", "chat_id=1 AND user_id=2"), grpc: func(ctx context.Context) error {
			_, e := mg.UpdateMessage(ctx, &messagepb.UpdateMessageRequest{Message: update})
			return e
		}, defect: "UpdateMessage -> validateEncryption Select without context"},
		{name: "message_update_write", method: "PUT", path: fmt.Sprintf("/messages/%d", f.messageID), route: "/messages/:id", body: jsonBody(map[string]string{"content": f.content}), http: mc.UpdateMessage, stage: "UPDATE messages", barrier: write("messages", "UPDATE"), grpc: func(ctx context.Context) error {
			_, e := mg.UpdateMessage(ctx, &messagepb.UpdateMessageRequest{Message: update})
			return e
		}, defect: "UpdateMessage Exec after locks without context"},
		{name: "message_read", stage: "SELECT id, chat_id, sender_id, content", barrier: read("messages", "SELECT id, chat_id, sender_id, content"), grpc: func(ctx context.Context) error {
			_, e := mg.GetMessage(ctx, &messagepb.GetMessageRequest{Id: int64(f.messageID)})
			return e
		}, defect: "GetMessageByID -> sqlx.Get without context"},
		{name: "history", method: "GET", path: "/messages/1", route: "/messages/:chat_id", http: mc.GetMessagesByChatID, stage: "SELECT id, chat_id, sender_id, content", barrier: read("messages", "SELECT id, chat_id, sender_id, content"), grpc: func(ctx context.Context) error {
			_, e := mg.GetMessagesByChatID(ctx, &messagepb.GetMessagesByChatIDRequest{ChatId: 1})
			return e
		}, defect: "regression: history already forwards context", deadline: true},
		{name: "longpoll_sql", method: "GET", path: "/messages/longpoll/1", route: "/messages/longpoll/:chat_id", http: mc.LongPolling, stage: "SELECT id, chat_id, sender_id, content", barrier: read("messages", "SELECT id, chat_id, sender_id, content"), defect: "regression: longpoll SQL already forwards context"},
	}

	// Preliminary reads are distinct from later writes: cancellation must reach both.
	for _, original := range append([]cancellationCase(nil), cases...) {
		c := original
		switch c.name {
		case "user_update", "user_delete":
			c.name += "_read"
			c.stage = "FROM users WHERE id"
			c.barrier = read("users", c.stage)
			c.defect = "preliminary GetUserByID loses context"
		case "chat_update", "chat_delete":
			c.name += "_read"
			c.stage = "SELECT id, name, type, created_by, created_at FROM chats"
			c.barrier = read("chats", c.stage)
			c.defect = "preliminary GetChatByID loses context"
		case "participant_delete_lock":
			c.name = "participant_delete_read"
			c.stage = "SELECT * FROM chat_participants"
			c.barrier = read("chat_participants", c.stage)
			c.defect = "GetParticipant loses context"
		case "participant_create_lock":
			c.name = "participant_create_write"
			c.stage = "INSERT INTO chat_participants"
			c.barrier = write("chat_participants", "INSERT")
			c.defect = "CreateParticipant Exec after LockChat loses context"
		case "message_update_lock":
			c.name = "message_update_read"
			c.stage = "SELECT id, chat_id, sender_id, content"
			c.barrier = read("messages", c.stage)
			c.defect = "UpdateMessage preliminary GetMessageByID loses context"
		default:
			continue
		}
		cases = append(cases, c)
	}
	for _, original := range append([]cancellationCase(nil), cases...) {
		if original.name != "message_update_lock" {
			continue
		}
		c := original
		c.name = "message_update_transaction_read"
		c.stage = "SELECT id, chat_id, sender_id, content"
		c.barrier = func(t *testing.T) sqlBarrier {
			return f.readBarrierCondition(t, "messages", c.stage, `EXISTS(SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND relation='chats'::regclass AND mode='RowShareLock')`)
		}
		c.defect = "second GetMessageByID inside transaction ignores context after LockChat"
		cases = append(cases, c)
	}
	cryptoController := controllers.NewCryptoController(f.operationDB, f.access)
	var identity e2ee.Identity
	ctx, done := context.WithTimeout(context.Background(), testWaitTimeout)
	err := f.db.GetContext(ctx, &identity, `SELECT user_id,key_id,exchange_key,signing_key,salt,nonce,encrypted_private_keys,iterations FROM crypto_identities WHERE user_id=1`)
	done()
	if err != nil {
		panic(err)
	}
	ac := controllers.NewAuthController(f.access, false)
	cases = append(cases,
		cancellationCase{name: "access_check", method: "GET", path: "/chat/1", route: "/chat/:id", http: cc.GetChat, grpc: func(ctx context.Context) error { _, e := cg.GetChat(ctx, &chatpb.GetChatRequest{Id: 1}); return e }, stage: "SELECT EXISTS(SELECT 1 FROM user_sessions", barrier: read("user_sessions", "SELECT EXISTS(SELECT 1 FROM user_sessions"), defect: "SQL respects context; error classification at RequireChat boundary", deadline: true},
		cancellationCase{name: "crypto_identity", method: "GET", path: "/crypto/identity", route: "/crypto/identity", http: cryptoController.Identity, stage: "FROM crypto_identities", barrier: read("crypto_identities", "FROM crypto_identities"), defect: "regression: context SQL; REST cancellation error handling", deadline: true},
		cancellationCase{name: "crypto_public", method: "GET", path: "/crypto/keys", route: "/crypto/keys", http: cryptoController.PublicKeys, stage: "FROM crypto_identities", barrier: read("crypto_identities", "FROM crypto_identities"), defect: "regression: public keys context SQL; REST cancellation error handling"},
		cancellationCase{name: "crypto_chat", method: "GET", path: "/crypto/chat/1", route: "/crypto/chat/:id", http: cryptoController.ChatKeys, stage: "LEFT JOIN crypto_identities", barrier: read("crypto_identities", "LEFT JOIN crypto_identities"), defect: "regression: chat keys context SQL; REST cancellation error handling"},
		cancellationCase{name: "crypto_create", method: "POST", path: "/crypto/identity", route: "/crypto/identity", body: jsonBody(identity), http: cryptoController.CreateIdentity, stage: "INSERT INTO crypto_identities", barrier: write("crypto_identities", "INSERT"), defect: "regression: identity INSERT cancels; REST cancellation error handling"},
		cancellationCase{name: "register_partial", method: "POST", path: "/auth/register", route: "/auth/register", body: `{"username":"cancel_register","email":"cancel@example.com","password":"context-stage-c-password"}`, http: ac.Register, stage: "INSERT INTO user_sessions", barrier: write("user_sessions", "INSERT"), defect: "regression: registration transaction rolls back inserted user; REST error handling"},
		cancellationCase{name: "login_session", method: "POST", path: "/auth/login", route: "/auth/login", body: `{"email":"alice@example.com","password":"context-stage-c-password"}`, http: ac.Login, stage: "INSERT INTO user_sessions", barrier: write("user_sessions", "INSERT"), defect: "regression: session transaction cancels; REST error handling"},
		cancellationCase{name: "logout", method: "POST", path: "/auth/logout", route: "/auth/logout", http: ac.Logout, stage: "DELETE FROM user_sessions", barrier: write("user_sessions", "DELETE"), defect: "regression: logout SQL cancels; REST error handling"},
	)

	digest := sha256.Sum256([]byte("legacy"))
	migration := jsonBody(map[string]string{"content": f.migrated, "previous_hash": base64.StdEncoding.EncodeToString(digest[:])})
	for _, stage := range []string{"chat_lock", "message_row", "keys", "write"} {
		c := cancellationCase{name: "migration_" + stage, method: "POST", path: fmt.Sprintf("/messages/%d/encrypt", f.messageID), route: "/messages/:id/encrypt", body: migration, http: mc.EncryptLegacyMessage, defect: "regression: migration SQL must cancel and roll back"}
		switch stage {
		case "chat_lock":
			c.stage = "SELECT id FROM chats"
			c.barrier = row("chats", "id=1")
			c.defect = "EncryptLegacyMessage -> LockChat ignores transaction context"
			c.deadline = true
		case "message_row":
			c.stage = "FOR UPDATE"
			c.barrier = row("messages", fmt.Sprintf("id=%d", f.messageID))
		case "keys":
			c.stage = "FOR SHARE OF p"
			c.barrier = row("chat_participants", "chat_id=1 AND user_id=2")
		case "write":
			c.stage = "UPDATE messages"
			c.barrier = write("messages", "UPDATE")
		}
		cases = append(cases, c)
	}
	return cases
}

func TestContextStageCActiveAPI(t *testing.T) {
	f := newCancellationFixture(t)
	for _, c := range cancellationCases(f) {
		transports := []string{}
		if c.http != nil {
			transports = append(transports, "REST")
		}
		if c.grpc != nil {
			transports = append(transports, "gRPC")
		}
		for _, transport := range transports {
			modes := []string{"cancel"}
			if c.deadline {
				modes = append(modes, "deadline")
			}
			for _, mode := range modes {
				t.Run(c.name+"/"+transport+"/"+mode, func(t *testing.T) {
					t.Log("contract:", c.defect)
					before := f.snapshot(t)
					barrier := c.barrier(t)
					var ctx context.Context
					var cancel context.CancelFunc
					if mode == "deadline" {
						ctx, cancel = context.WithTimeout(f.principal, 2*time.Second)
					} else {
						ctx, cancel = context.WithCancel(f.principal)
					}
					defer cancel()
					var writes *handlerResponseWrites
					var run func() error
					if transport == "REST" {
						router := gin.New()
						handler, w := observeHandlerWrites(c.http)
						writes = w
						router.Handle(c.method, c.route, handler)
						request := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)).WithContext(ctx)
						request.Header.Set("Content-Type", "application/json")
						request.Header.Set("Authorization", "Bearer "+f.token)
						recorder := httptest.NewRecorder()
						run = func() error { router.ServeHTTP(recorder, request); return nil }
					} else {
						run = func() error { return c.grpc(ctx) }
					}
					pid := 0
					forceCancel := func() {
						if pid == 0 {
							return
						}
						cc, done := context.WithTimeout(context.Background(), testWaitTimeout)
						defer done()
						var ok bool
						if e := f.observer.GetContext(cc, &ok, "SELECT pg_cancel_backend($1)", pid); e != nil {
							t.Error("cleanup pg_cancel_backend:", e)
						}
					}
					result := startTestOperation(t, func() { cancel(); forceCancel(); barrier.release() }, run)
					pid = f.waitBlocked(t, barrier.pid, c.stage)
					if mode == "cancel" {
						cancel()
					} else {
						select {
						case <-ctx.Done():
						case <-time.After(3 * time.Second):
							t.Fatal("request deadline did not expire")
						}
					}
					var err error
					select {
					case err = <-result:
					case <-time.After(time.Second):
						t.Error("DEFECT: server operation remains blocked after context completion (before 15s statement_timeout)")
						forceCancel()
						err = receiveTestError(t, result)
					}
					if transport == "REST" {
						writes.assertNone(t)
					} else {
						want := codes.Canceled
						if mode == "deadline" {
							want = codes.DeadlineExceeded
						}
						// This is a direct handler call, before the production mapper.
						// Accept a driver cancellation cause; mapper statuses are tested separately.
						var pg *pq.Error
						driverCanceled := errors.As(err, &pg) && pg.Code == "57014" && ctx.Err() != nil
						if status.Code(err) != want && !errors.Is(err, ctx.Err()) && !driverCanceled {
							t.Errorf("DEFECT: gRPC error=%v, want %s", err, want)
						}
					}
					f.waitStopped(t, pid)
					// Release the blocker only AFTER proving server termination. Otherwise a
					// broken operation could complete/commit and turn a cancellation test green.
					barrier.release()
					if after := f.snapshot(t); after != before {
						t.Error("DEFECT: data changed after canceled operation; rollback/unchanged-state contract violated")
					}
				})
			}
		}
	}
}
