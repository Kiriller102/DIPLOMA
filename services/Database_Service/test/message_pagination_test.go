package test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"project/services/Database_Service/internal/logging"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
	"project/services/Database_Service/services"
)

func paginationRows(ids ...int) *messageRows {
	r := &messageRows{columns: []string{"id", "chat_id", "sender_id", "content", "encryption_algorithm", "sent_at", "crypto_version", "client_id"}}
	for _, id := range ids {
		r.values = append(r.values, []driver.Value{int64(id), int64(7), int64(3), "e2ee:v1:test", "AES", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), int64(1), "client"})
	}
	return r
}

func TestMessagePaginationPages(t *testing.T) {
	for _, count := range []int{0, 1, 49, 50, 51} {
		for _, withCursor := range []bool{false, true} {
			t.Run(fmt.Sprintf("rows=%d/cursor=%t", count, withCursor), func(t *testing.T) {
				ctx := context.Background()
				var beforeID *int
				if withCursor {
					id := 1000
					beforeID = &id
				}
				calls := 0
				_, svc := contextMessageService(t, func(got context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
					calls++
					if got != ctx {
						t.Fatal("request context was replaced")
					}
					query = strings.Join(strings.Fields(query), " ")
					wantArgs := []driver.NamedValue{{Ordinal: 1, Value: int64(7)}, {Ordinal: 2, Value: int64(51)}}
					wantTail := "WHERE chat_id = $1 ORDER BY id DESC LIMIT $2"
					if withCursor {
						wantArgs = []driver.NamedValue{{Ordinal: 1, Value: int64(7)}, {Ordinal: 2, Value: int64(1000)}, {Ordinal: 3, Value: int64(51)}}
						wantTail = "WHERE chat_id = $1 AND id < $2 ORDER BY id DESC LIMIT $3"
					}
					if !strings.HasSuffix(query, wantTail) || !reflect.DeepEqual(args, wantArgs) {
						t.Fatalf("unexpected page query: %s args=%v", query, args)
					}
					ids := make([]int, count)
					for i := range ids {
						ids[i] = 999 - 2*i // Gaps in IDs must not affect page boundaries.
					}
					return paginationRows(ids...), nil
				})
				page, err := svc.GetMessagesPageByChatID(ctx, 7, beforeID)
				if err != nil {
					t.Fatal(err)
				}
				wantCount := min(count, 50)
				if calls != 1 || page.Messages == nil || len(page.Messages) != wantCount {
					t.Fatalf("calls=%d, page=%+v", calls, page)
				}
				for i, message := range page.Messages {
					wantID := 999 - 2*(wantCount-1-i)
					if message.ID != wantID {
						t.Fatalf("message[%d]=%d, want %d", i, message.ID, wantID)
					}
				}
				if count == 51 {
					if page.NextCursor == nil || *page.NextCursor != 901 {
						t.Fatalf("cursor=%v, want oldest returned ID 901 (not lookahead ID 899)", page.NextCursor)
					}
				} else if page.NextCursor != nil {
					t.Fatalf("unexpected cursor: %d", *page.NextCursor)
				}
				if count == 0 {
					encoded, err := json.Marshal(page)
					if err != nil || string(encoded) != `{"messages":[],"next_cursor":null}` {
						t.Fatalf("empty page JSON=%s, err=%v", encoded, err)
					}
				}
			})
		}
	}
}

func TestMessagePaginationValidation(t *testing.T) {
	zero, negative := 0, -1
	for _, tc := range []struct {
		name   string
		chat   int
		before *int
	}{{"zero chat", 0, nil}, {"negative chat", -1, nil}, {"zero cursor", 7, &zero}, {"negative cursor", 7, &negative}} {
		t.Run(tc.name, func(t *testing.T) {
			_, svc := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				t.Fatal("invalid input reached database")
				return nil, nil
			})
			page, err := svc.GetMessagesPageByChatID(context.Background(), tc.chat, tc.before)
			if !errors.Is(err, services.ErrInvalidInput) || page.Messages != nil || page.NextCursor != nil {
				t.Fatalf("page=%+v, error=%v", page, err)
			}
		})
	}
}

func TestMessagePaginationRepositoryMapping(t *testing.T) {
	db, _ := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return paginationRows(9, 5), nil
	})
	repo := repository.MessageRepository{}
	messages, err := repo.GetMessagesBefore(context.Background(), db, 7, nil, 2)
	if err != nil || len(messages) != 2 {
		t.Fatalf("messages=%v, err=%v", messages, err)
	}
	want := models.Message{ID: 9, ChatID: 7, SenderID: 3, Content: "e2ee:v1:test", EncryptionAlgorithm: models.EncryptionAlgorithmAES,
		SentAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), CryptoVersion: 1, ClientID: "client"}
	if messages[0] != want || messages[1].ID != 5 {
		t.Fatalf("mapping or descending order changed: %+v", messages)
	}
}

type paginationErrorRows struct {
	*messageRows
	err    error
	closed bool
}

func (r *paginationErrorRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return r.err
	}
	return r.messageRows.Next(dest)
}

func (r *paginationErrorRows) Close() error { r.closed = true; return nil }

func TestMessagePaginationReadErrors(t *testing.T) {
	for _, target := range []string{"repository", "service"} {
		for _, stage := range []string{"query", "scan", "iteration"} {
			t.Run(target+"/"+stage, func(t *testing.T) {
				cause := &pq.Error{Code: "XX000", Message: "read failed"}
				rows := &paginationErrorRows{messageRows: paginationRows(9), err: cause}
				if stage == "scan" {
					rows.values[0][0] = "invalid ID"
				}
				db, svc := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
					if stage == "query" {
						return nil, cause
					}
					return rows, nil
				})
				var err error
				if target == "repository" {
					repo := repository.MessageRepository{}
					var messages []models.Message
					messages, err = repo.GetMessagesBefore(context.Background(), db, 7, nil, 51)
					if messages != nil {
						t.Fatal("partial result returned after failure")
					}
				} else {
					var page services.MessageHistoryPage
					page, err = svc.GetMessagesPageByChatID(context.Background(), 7, nil)
					if page.Messages != nil || page.NextCursor != nil {
						t.Fatal("partial page returned after failure")
					}
				}
				if err == nil {
					t.Fatal("expected read error")
				}
				if stage != "scan" {
					var pg *pq.Error
					if !errors.Is(err, cause) || !errors.As(err, &pg) || pg != cause {
						t.Fatalf("original error lost: %v", err)
					}
				}
				diagnosis := (logging.Classifier{}).Classify(context.Background(), err, logging.Options{}).LogValue()
				var gotStage, operation string
				for _, attr := range diagnosis.Group() {
					if attr.Key == "stage" {
						gotStage = attr.Value.String()
					}
					if attr.Key == "operation" {
						operation = attr.Value.String()
					}
				}
				if gotStage != stage || operation != logging.MessageHistory {
					t.Fatalf("diagnosis=%v", diagnosis)
				}
				if stage != "query" && !rows.closed {
					t.Fatal("rows were not closed")
				}
			})
		}
	}
}

func TestMessagePaginationCancellation(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		_, svc := contextMessageService(t, func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
			t.Error("canceled query reached driver")
			return nil, nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := svc.GetMessagesPageByChatID(ctx, 7, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("running query", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel()
		started := make(chan struct{})
		_, svc := contextMessageService(t, func(got context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
			if got != ctx {
				t.Error("request context was replaced")
			}
			close(started)
			<-got.Done()
			return nil, got.Err()
		})
		done := startTestOperation(t, cancel, func() error { _, err := svc.GetMessagesPageByChatID(ctx, 7, nil); return err })
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("query did not start")
		}
		cancel()
		if err := receiveTestError(t, done); !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	})
}
