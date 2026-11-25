package test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"project/services/Database_Service/api/rest"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/internal/logging"
	"project/services/Database_Service/services"
)

// Exercise the production router, authentication, access checks and completion
// middleware. Only the SQL driver is replaced, as in the existing logging tests.
func paginationRESTServer(t *testing.T, read messageQuery) (*rest.HTTPServer, *pilotSink) {
	t.Helper()
	db, svc := contextMessageService(t, func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "SELECT u.id,u.username") {
			now := time.Now()
			return &messageRows{columns: []string{"id", "username", "email", "first_name", "last_name", "created_at", "updated_at", "is_active", "expires_at"},
				values: [][]driver.Value{{int64(1), "user", "user@example.org", "", "", now, now, true, now.Add(time.Hour)}}}, nil
		}
		if strings.Contains(query, "FROM chat_participants") || strings.Contains(query, "SELECT EXISTS(SELECT 1 FROM user_sessions") {
			return &messageRows{columns: []string{"exists"}, values: [][]driver.Value{{true}}}, nil
		}
		if !strings.Contains(query, "FROM messages") {
			t.Fatalf("unexpected SQL: %s", query)
		}
		return read(ctx, query, args)
	})
	access, err := auth.New(db)
	if err != nil {
		t.Fatal(err)
	}
	sink := newPilotSink()
	server := rest.NewHTTPServer(controllers.NewUserController(nil, access), controllers.NewMessageController(svc, access),
		controllers.NewChatController(nil, access), controllers.NewChatParticipantController(nil, access), db, access, false, logging.New(sink, slog.LevelDebug))
	t.Cleanup(server.CancelPolls)
	return server, sink
}

func paginationRESTRequest(server *rest.HTTPServer, path string, authenticated bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+strings.Repeat("A", 43))
	}
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	return w
}

func TestMessagePaginationRESTPages(t *testing.T) {
	for _, tc := range []struct {
		name, query   string
		count, newest int
	}{{"empty", "", 0, 101}, {"last page", "", 50, 101}, {"latest page", "", 51, 101}, {"previous page", "?before_id=151", 51, 150}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server, sink := paginationRESTServer(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				calls++
				if args[0].Value != int64(7) || args[len(args)-1].Value != int64(51) {
					t.Fatalf("unexpected query arguments: %v", args)
				}
				if tc.query != "" {
					if len(args) != 3 || args[1].Value != int64(151) || !strings.Contains(query, "id < $2") {
						t.Fatalf("cursor not forwarded: %s %v", query, args)
					}
				} else if len(args) != 2 {
					t.Fatalf("unexpected cursor: %v", args)
				}
				ids := make([]int, tc.count)
				for i := range ids {
					ids[i] = tc.newest - i
				}
				return paginationRows(ids...), nil
			})
			w := paginationRESTRequest(server, "/messages/7"+tc.query, true)
			if w.Code != http.StatusOK || calls != 1 {
				t.Fatalf("status=%d, calls=%d, body=%s", w.Code, calls, w.Body.String())
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
				t.Fatal(err)
			}
			if len(fields) != 2 || fields["messages"] == nil || fields["next_cursor"] == nil {
				t.Fatalf("unexpected contract: %s", w.Body.String())
			}
			var page services.MessageHistoryPage
			if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			wantCount := min(50, tc.count)
			if page.Messages == nil || len(page.Messages) != wantCount {
				t.Fatalf("unexpected page: %+v", page)
			}
			for i, message := range page.Messages {
				if message.ID != tc.newest-wantCount+1+i {
					t.Fatalf("wrong order or lookahead included: %+v", page)
				}
			}
			if tc.count > 50 {
				if page.NextCursor == nil || *page.NextCursor != tc.newest-49 {
					t.Fatalf("unexpected cursor: %+v", page)
				}
			} else if string(fields["next_cursor"]) != "null" {
				t.Fatalf("expected null cursor: %s", w.Body.String())
			}
			event := sink.wait(t)
			if event.HTTPStatus != 200 || event.Diagnosis["outcome"] != "ok" {
				t.Fatalf("unexpected completion: %+v", event)
			}
		})
	}
}

func TestMessagePaginationRESTValidation(t *testing.T) {
	for _, query := range []string{"before_id=", "before_id=abc", "before_id=0", "before_id=-1", "before_id=1.5", "before_id=999999999999999999999999", "before_id=1&before_id=2"} {
		t.Run(query, func(t *testing.T) {
			server, sink := paginationRESTServer(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				t.Fatal("invalid cursor reached message query")
				return nil, nil
			})
			w := paginationRESTRequest(server, "/messages/7?"+query, true)
			if w.Code != 400 {
				t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
			}
			event := sink.wait(t)
			if event.HTTPStatus != 400 || event.Diagnosis["stage"] != "validation" || event.Diagnosis["error_class"] != "invalid_input" {
				t.Fatalf("unexpected completion: %+v", event)
			}
		})
	}
}

func TestMessagePaginationRESTAccess(t *testing.T) {
	for _, tc := range []struct {
		mode          string
		authenticated bool
		status        int
	}{{"success", false, 401}, {"forbidden", true, 403}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			f := newPilotFixture(t, tc.mode)
			w := paginationRESTRequest(f.http, "/messages/7?before_id=151", tc.authenticated)
			if w.Code != tc.status || f.reads.Load() != 0 {
				t.Fatalf("status=%d, reads=%d", w.Code, f.reads.Load())
			}
			if event := f.sink.wait(t); event.HTTPStatus != tc.status {
				t.Fatalf("unexpected completion: %+v", event)
			}
		})
	}
}
