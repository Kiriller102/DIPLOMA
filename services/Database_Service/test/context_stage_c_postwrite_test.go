package test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"project/services/Database_Service/api/rest/controllers"
)

// These reads happen AFTER an autocommit UPDATE/committed transaction. The
// cancellation must stop the reread, but must not undo the acknowledged SQL write.
func TestContextStageCPostWriteRead(t *testing.T) {
	for _, name := range []string{"user", "chat", "message"} {
		t.Run(name, func(t *testing.T) {
			f := newCancellationFixture(t)
			var handler gin.HandlerFunc
			var table, stage, condition, body, route, path, column, want string
			switch name {
			case "user":
				table = "users"
				stage = "FROM users WHERE id"
				column = "first_name"
				want = "changed"
				condition = "EXISTS(SELECT 1 FROM context_backing WHERE id=1 AND first_name='changed')"
				handler = controllers.NewUserController(f.users, f.access).UpdateUser
				body = `{"first_name":"changed"}`
				route = "/users/:id"
				path = "/users/1"
			case "chat":
				table = "chats"
				stage = "SELECT id, name, type, created_by, created_at FROM chats"
				column = "name"
				want = "changed"
				condition = "EXISTS(SELECT 1 FROM context_backing WHERE id=1 AND name='changed')"
				handler = controllers.NewChatController(f.chats, f.access).UpdateChat
				body = `{"name":"changed"}`
				route = "/chat/:id"
				path = "/chat/1"
			case "message":
				table = "messages"
				stage = "SELECT id, chat_id, sender_id, content"
				column = "content"
				want = f.content
				condition = "EXISTS(SELECT 1 FROM context_backing WHERE content=" + quoteSQL(f.content) + ")"
				handler = controllers.NewMessageController(f.messages, f.access).UpdateMessage
				body = jsonBody(map[string]string{"content": f.content})
				route = "/messages/:id"
				path = fmt.Sprintf("/messages/%d", f.messageID)
			}
			b := f.readBarrierCondition(t, table, stage, condition)
			ctx, cancel := context.WithCancel(f.principal)
			defer cancel()
			router := gin.New()
			h, writes := observeHandlerWrites(handler)
			router.PUT(route, h)
			req := httptest.NewRequest("PUT", path, strings.NewReader(body)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			pid := 0
			force := func() {
				if pid == 0 {
					return
				}
				c, done := context.WithTimeout(context.Background(), testWaitTimeout)
				defer done()
				var ok bool
				if err := f.observer.GetContext(c, &ok, "SELECT pg_cancel_backend($1)", pid); err != nil {
					t.Error(err)
				}
			}
			done := startTestOperation(t, func() { cancel(); force(); b.release() }, func() error { router.ServeHTTP(httptest.NewRecorder(), req); return nil })
			pid = f.waitBlocked(t, b.pid, stage)
			// Independent observation proves the earlier write is committed, before cancel.
			check, cancelCheck := context.WithTimeout(context.Background(), testWaitTimeout)
			targetID := 1
			if name == "message" {
				targetID = f.messageID
			}
			var saved string
			err := f.db.GetContext(check, &saved, "SELECT "+column+" FROM context_backing WHERE id=$1", targetID)
			cancelCheck()
			if err != nil || saved != want {
				t.Fatalf("did not reach post-commit read: value=%q err=%v", saved, err)
			}
			committed := f.snapshot(t)
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("DEFECT: REST reread after committed write ignores cancellation")
				force()
				receiveTestError(t, done)
			}
			writes.assertNone(t)
			f.waitStopped(t, pid)
			b.release()
			if f.snapshot(t) != committed {
				t.Error("canceling post-commit reread modified committed data")
			}
		})
	}
}
