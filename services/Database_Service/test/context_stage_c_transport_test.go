package test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"project/services/Database_Service/api/rest"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/models"
)

type networkResponseWrites struct {
	http.ResponseWriter
	status *atomic.Int32
	bytes  *atomic.Int64
}

func (w networkResponseWrites) WriteHeader(code int) {
	w.status.Store(int32(code))
	w.ResponseWriter.WriteHeader(code)
}
func (w networkResponseWrites) Write(p []byte) (int, error) {
	w.bytes.Add(int64(len(p)))
	return w.ResponseWriter.Write(p)
}

func TestContextStageCHTTPClient(t *testing.T) {
	f := newCancellationFixture(t)
	for _, tc := range []struct{ name, path, table, stage string }{
		{"users_list", "/users/all", "users", "FROM users"},
		{"history", "/messages/1", "messages", "SELECT id, chat_id, sender_id, content"},
		{"authentication", "/messages/1", "user_sessions", "SELECT u.id,u.username"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := f.snapshot(t)
			barrier := f.readBarrier(t, tc.table, tc.stage)
			server := rest.NewHTTPServer(controllers.NewUserController(f.users, f.access), controllers.NewMessageController(f.messages, f.access), controllers.NewChatController(f.chats, f.access), controllers.NewChatParticipantController(f.participants, f.access), f.operationDB, f.access, false)
			finished := make(chan struct{})
			var code atomic.Int32
			var bytes atomic.Int64
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				server.Handler().ServeHTTP(networkResponseWrites{w, &code, &bytes}, r)
			}))
			t.Cleanup(ts.Close)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, "GET", ts.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+f.token)
			client := ts.Client()
			client.Timeout = 8 * time.Second
			pid := 0
			cleanup := func() {
				cancel()
				if pid != 0 {
					c, done := context.WithTimeout(context.Background(), testWaitTimeout)
					defer done()
					var ok bool
					if err := f.observer.GetContext(c, &ok, "SELECT pg_cancel_backend($1)", pid); err != nil {
						t.Error(err)
					}
				}
			}
			result := startTestOperation(t, func() { cleanup(); barrier.release() }, func() error {
				response, err := client.Do(request)
				if response != nil {
					io.Copy(io.Discard, response.Body)
					response.Body.Close()
				}
				return err
			})
			pid = f.waitBlocked(t, barrier.pid, tc.stage)
			cancel()
			if err := receiveTestError(t, result); !errors.Is(err, context.Canceled) {
				t.Errorf("HTTP client got %v, want canceled", err)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Error("DEFECT: client canceled but server is still running")
				cleanup()
				select {
				case <-finished:
				case <-time.After(testWaitTimeout):
					t.Fatal("server did not finish after cleanup cancellation")
				}
			}
			f.waitStopped(t, pid)
			if code.Load() >= 400 || bytes.Load() != 0 {
				t.Errorf("DEFECT: application generated error response after transport cancellation: status=%d bytes=%d", code.Load(), bytes.Load())
			}
			barrier.release()
			if f.snapshot(t) != before {
				t.Error("data changed after HTTP cancellation")
			}
		})
	}
}

// Explicitly verify that cancellation AFTER an acknowledged commit is not rollback.
func TestContextStageCAfterCommit(t *testing.T) {
	f := newCancellationFixture(t)
	ctx, cancel := context.WithCancel(f.principal)
	defer cancel()
	chat := models.Chat{Name: "committed-before-cancel", Type: models.ChatTypeGroup, CreatedBy: 1, ParticipantIDs: []int{2}}
	done := startTestOperation(t, cancel, func() error { return f.chats.CreateChat(ctx, &chat) })
	if err := receiveTestError(t, done); err != nil {
		t.Fatal(err)
	}
	committed := f.snapshot(t)
	cancel()
	if f.snapshot(t) != committed {
		t.Fatal("cancellation after successful commit changed data")
	}
	check, cancelCheck := context.WithTimeout(context.Background(), testWaitTimeout)
	defer cancelCheck()
	var count int
	if err := f.db.GetContext(check, &count, `SELECT count(*) FROM chat_participants WHERE chat_id=$1`, chat.ID); err != nil || count != 2 {
		t.Fatalf("committed chat participants=%d err=%v", count, err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("test context not canceled")
	}
}

// Empty polling has no SQL blocked: confirm the first empty SELECT finished,
// then cancel a real HTTP client and independently await the server's return.
func TestContextStageCLongPollWaiting(t *testing.T) {
	f := newCancellationFixture(t)
	before := f.snapshot(t)
	server := rest.NewHTTPServer(controllers.NewUserController(f.users, f.access), controllers.NewMessageController(f.messages, f.access), controllers.NewChatController(f.chats, f.access), controllers.NewChatParticipantController(f.participants, f.access), f.operationDB, f.access, false)
	finished := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(finished); server.Handler().ServeHTTP(w, r) }))
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/messages/longpoll/1?last_id=%d", ts.URL, f.messageID), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	client := ts.Client()
	client.Timeout = 8 * time.Second
	done := startTestOperation(t, cancel, func() error {
		response, err := client.Do(req)
		if response != nil {
			response.Body.Close()
		}
		return err
	})
	var pid int
	waitForTestState(t, "first empty polling SELECT completed", func(c context.Context) (bool, error) {
		e := f.observer.GetContext(c, &pid, `SELECT COALESCE((SELECT pid FROM pg_stat_activity WHERE application_name=$1 AND state='idle' AND position('id >' in query)>0 LIMIT 1),0)`, f.app)
		return pid != 0, e
	})
	cancel()
	if err := receiveTestError(t, done); !errors.Is(err, context.Canceled) {
		t.Errorf("client error=%v", err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("empty polling server did not stop on cancellation")
	}
	f.waitStopped(t, pid)
	if f.snapshot(t) != before {
		t.Error("empty polling changed data")
	}
}
