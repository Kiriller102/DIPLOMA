package test

import (
	"context"
	"fmt"
	"testing"

	"project/services/Database_Service/repository"
	"project/services/Database_Service/services"
)

func TestMessagePaginationPostgres(t *testing.T) {
	db := testDB(t) // All writes are confined to this helper's temporary schema.
	svc, err := services.NewMessageService(db)
	if err != nil {
		t.Fatal(err)
	}
	var otherChat int
	if err := db.Get(&otherChat, `INSERT INTO chats(name,type,created_by) VALUES ('pagination other chat','group',1) RETURNING id`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		count  int
		mutate bool
	}{{0, false}, {1, false}, {49, false}, {50, false}, {51, false}, {100, false}, {101, false}, {101, true}} {
		t.Run(fmt.Sprintf("messages=%d/mutate=%t", tc.count, tc.mutate), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
			defer cancel()
			if _, err := db.ExecContext(ctx, `DELETE FROM messages`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO messages(id,chat_id,sender_id,content) SELECT n*2,1,1,'page fixture' FROM generate_series(1,$1) AS n`, tc.count); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO messages(id,chat_id,sender_id,content) VALUES (1001,$1,1,'other chat')`, otherChat); err != nil {
				t.Fatal(err)
			}

			seen := make(map[int]bool)
			var cursor *int
			requests := 0
			for {
				page, err := svc.GetMessagesPageByChatID(ctx, 1, cursor)
				if err != nil {
					t.Fatal(err)
				}
				requests++
				remaining := tc.count - len(seen)
				if page.Messages == nil || len(page.Messages) != min(50, remaining) {
					t.Fatalf("unexpected page size: %+v, remaining=%d", page, remaining)
				}
				for i, message := range page.Messages {
					if message.ChatID != 1 || message.ID <= 0 || message.ID > tc.count*2 || message.ID%2 != 0 || seen[message.ID] {
						t.Fatalf("foreign, new or duplicate message: %+v", message)
					}
					if cursor != nil && message.ID >= *cursor {
						t.Fatalf("cursor boundary included: %d >= %d", message.ID, *cursor)
					}
					if i > 0 && page.Messages[i-1].ID >= message.ID {
						t.Fatal("page is not ascending")
					}
					seen[message.ID] = true
				}
				if (page.NextCursor != nil) != (len(seen) < tc.count) {
					t.Fatalf("wrong continuation: %+v", page)
				}
				if page.NextCursor == nil {
					break
				}
				if *page.NextCursor != page.Messages[0].ID {
					t.Fatal("cursor is not oldest returned ID")
				}
				if tc.mutate && requests == 1 {
					// Removing the boundary row must not break a numeric cursor.
					if _, err := db.ExecContext(ctx, `DELETE FROM messages WHERE id=$1`, *page.NextCursor); err != nil {
						t.Fatal(err)
					}
					// New messages belong to the forward polling path, not older pages.
					if _, err := db.ExecContext(ctx, `INSERT INTO messages(id,chat_id,sender_id,content) VALUES (10000,1,1,'new message')`); err != nil {
						t.Fatal(err)
					}
				}
				cursor = page.NextCursor
				if requests >= 3 {
					t.Fatal("pagination did not terminate")
				}
			}
			if len(seen) != tc.count || requests != max(1, (tc.count+49)/50) {
				t.Fatalf("seen=%d, requests=%d", len(seen), requests)
			}
			for n := 1; n <= tc.count; n++ {
				if !seen[2*n] {
					t.Fatalf("missing ID %d", 2*n)
				}
			}

			// The existing full-history method remains usable by gRPC.
			all, err := svc.GetMessagesByChatID(ctx, 1)
			if err != nil || len(all) != tc.count {
				t.Fatalf("full history: len=%d, err=%v", len(all), err)
			}
		})
	}

	t.Run("repository cursor and limit", func(t *testing.T) {
		repo := repository.MessageRepository{}
		before := 10
		messages, err := repo.GetMessagesBefore(context.Background(), db, 1, &before, 3)
		if err != nil || len(messages) != 3 {
			t.Fatalf("messages=%v, err=%v", messages, err)
		}
		for i, want := range []int{8, 6, 4} {
			if messages[i].ID != want {
				t.Fatalf("message[%d]=%d, want %d", i, messages[i].ID, want)
			}
		}
		before = 1
		page, err := svc.GetMessagesPageByChatID(context.Background(), 1, &before)
		if err != nil || page.Messages == nil || len(page.Messages) != 0 || page.NextCursor != nil {
			t.Fatalf("end page=%+v, err=%v", page, err)
		}
	})
}
