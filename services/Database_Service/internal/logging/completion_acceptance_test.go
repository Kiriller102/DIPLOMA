package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"
)

type acceptanceRecord struct {
	Event     string `json:"event"`
	Message   string `json:"msg"`
	ID        string `json:"operation_id"`
	Transport string `json:"transport"`
	Operation string `json:"operation"`
	Level     string `json:"level"`
	Index     int    `json:"test_index"`
	Duration  *int64 `json:"duration"`
	Diagnosis struct {
		Class     string `json:"error_class"`
		Type      string `json:"error_type"`
		SQLState  string `json:"sqlstate"`
		Stage     string `json:"stage"`
		Operation string `json:"operation"`
		Outcome   string `json:"outcome"`
	} `json:"diagnosis"`
}

func acceptanceRecords(t *testing.T, output []byte) []acceptanceRecord {
	t.Helper()
	if bytes.Contains(output, []byte("PRIVATE")) {
		t.Fatal("unsafe error text was serialized")
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	var records []acceptanceRecord
	for {
		var record acceptanceRecord
		err := decoder.Decode(&record)
		if err == io.EOF {
			return records
		}
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
}

func acceptanceWait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent logging did not finish within 5s")
	}
}

func assertAcceptanceRecord(t *testing.T, r acceptanceRecord, id, stage string, index int) {
	t.Helper()
	if r.ID != id || r.Index != index || r.Event != "operation.finished" || r.Message != "operation.finished" ||
		r.Transport != "direct" || r.Operation != MessageHistory || r.Level != "ERROR" || r.Duration == nil || *r.Duration < 0 ||
		r.Diagnosis.SQLState != "XX000" || r.Diagnosis.Class != "database" || r.Diagnosis.Type != "*pq.Error" ||
		r.Diagnosis.Outcome != "failed" || r.Diagnosis.Operation != MessageHistory || r.Diagnosis.Stage != stage {
		t.Fatalf("unexpected record for operation %s/index %d: %+v", id, index, r)
	}
}

func TestAcceptanceFinish100Concurrent(t *testing.T) {
	var output bytes.Buffer
	ctx, operation := Begin(context.Background(), New(&output, slog.LevelDebug), Classifier{}, "direct", MessageHistory)
	Capture(ctx, &pq.Error{Code: "XX000", Message: "PRIVATE", Detail: "PRIVATE"}, "query")
	id := operation.id // Known before emission, not inferred from the observed log.
	start, done := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			operation.Finish(ctx, slog.Int("test_index", 7))
		}()
	}
	go func() { wg.Wait(); close(done) }()
	close(start)
	acceptanceWait(t, done)
	operation.Finish(ctx, slog.Int("test_index", 99))
	records := acceptanceRecords(t, output.Bytes())
	if len(records) != 1 {
		t.Fatalf("100 concurrent Finish + repeated Finish emitted %d records, want 1", len(records))
	}
	assertAcceptanceRecord(t, records[0], id, "query", 7)
	t.Log("100 concurrent Finish calls and one repeated Finish: exactly 1 expected terminal record")
}

func TestAcceptance100IndependentOperations(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output, slog.LevelDebug)
	type expected struct {
		index int
		stage string
	}
	want := make(map[string]expected, 100)
	prepared := make([]struct {
		ctx       context.Context
		operation *Completion
		index     int
	}, 100)
	start, done := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	// The same error instance is intentional: no deduplication across operations.
	failure := &pq.Error{Code: "XX000", Message: "PRIVATE", Detail: "PRIVATE"}
	for i := 0; i < 100; i++ {
		ctx, operation := Begin(context.Background(), logger, Classifier{}, "direct", MessageHistory)
		stage := fmt.Sprintf("query_%03d", i)
		Capture(ctx, failure, stage)
		if operation.id == "" {
			t.Fatal("empty operation ID")
		}
		if _, exists := want[operation.id]; exists {
			t.Fatal("duplicate generated operation ID")
		}
		want[operation.id] = expected{i, stage}
		prepared[i].ctx = ctx
		prepared[i].operation = operation
		prepared[i].index = i
	}
	// No goroutine is started until every operation has been prepared and validated.
	for _, item := range prepared {
		wg.Add(1)
		go func(ctx context.Context, operation *Completion, index int) {
			defer wg.Done()
			<-start
			operation.Finish(ctx, slog.Int("test_index", index))
		}(item.ctx, item.operation, item.index)
	}
	go func() { wg.Wait(); close(done) }()
	close(start)
	acceptanceWait(t, done)
	records := acceptanceRecords(t, output.Bytes())
	if len(records) != 100 {
		t.Fatalf("got %d terminal records, want exactly 100", len(records))
	}
	seen := make(map[string]int, 100)
	for _, record := range records {
		expected, exists := want[record.ID]
		if !exists {
			t.Fatalf("unexpected operation ID %q", record.ID)
		}
		seen[record.ID]++
		assertAcceptanceRecord(t, record, record.ID, expected.stage, expected.index)
	}
	for id := range want {
		if seen[id] != 1 {
			t.Fatalf("expected operation %s occurred %d times", id, seen[id])
		}
	}
	t.Log("100 expected operation IDs: each appeared exactly once with its own index, stage and diagnosis")
}
