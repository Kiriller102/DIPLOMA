package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const MessageHistory = "message.get_by_chat"

type operationKey struct{}

var operationSequence atomic.Uint64
var operationEpoch = time.Now().UnixNano()

// Completion owns one terminal record. It must be finished by the entrypoint,
// not by services. Its logger and classifier are injected, never global.
type Completion struct {
	mu         sync.Mutex
	finished   bool
	err        error
	logger     *slog.Logger
	classifier Classifier
	started    time.Time
	id         string
	transport  string
	operation  string
}

func Begin(ctx context.Context, logger *slog.Logger, classifier Classifier, transport, operation string) (context.Context, *Completion) {
	o := &Completion{logger: logger, classifier: classifier, started: time.Now(),
		id:        fmt.Sprintf("%x-%x", operationEpoch, operationSequence.Add(1)),
		transport: label(transport), operation: label(operation)}
	return context.WithValue(ctx, operationKey{}, o), o
}

func FromContext(ctx context.Context) *Completion {
	o, _ := ctx.Value(operationKey{}).(*Completion)
	return o
}

// Capture keeps the first original error before transport mapping. Re-observing
// that failure in respondError/mapErrors must not replace it with a status error.
// With no explicit operation this is a no-op (including other routes/RPCs).
func Capture(ctx context.Context, err error, stage string) {
	if o := FromContext(ctx); o != nil && err != nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		if !o.finished && o.err == nil {
			o.err = Annotate(err, o.operation, stage)
		}
	}
}

// Finish is idempotent, including concurrent calls. Extra attributes must be
// allowlisted transport metadata, never raw errors, headers, URLs or payloads.
// Capture all original errors before calling Finish.
func (o *Completion) Finish(ctx context.Context, attrs ...slog.Attr) {
	o.mu.Lock()
	if o.finished {
		o.mu.Unlock()
		return
	}
	o.finished = true
	err := o.err
	o.mu.Unlock()
	if err == nil {
		err = ctx.Err()
	}
	d := o.classifier.Classify(ctx, err, Options{})
	fields := []slog.Attr{slog.String("event", "operation.finished"), slog.String("operation_id", o.id),
		slog.String("transport", o.transport), slog.String("operation", o.operation),
		slog.Duration("duration", time.Since(o.started)), slog.Any("diagnosis", d)}
	fields = append(fields, attrs...)
	o.logger.LogAttrs(ctx, d.Level(), "operation.finished", fields...)
}

// Run is an explicit owner for a direct service call. Core services stay silent.
// It returns the original callback error unchanged.
func Run(ctx context.Context, logger *slog.Logger, classifier Classifier, operation string, call func(context.Context) error) (err error) {
	ctx, completion := Begin(ctx, logger, classifier, "direct", operation)
	returned := false
	defer func() {
		if !returned {
			Capture(ctx, errors.New("direct operation interrupted"), "direct")
		}
		completion.Finish(ctx)
	}()
	err = call(ctx)
	Capture(ctx, err, "direct")
	returned = true
	return err
}
