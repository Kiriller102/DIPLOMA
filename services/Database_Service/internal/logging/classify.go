package logging

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Sentinels are supplied by the application boundary. Keeping domain packages
// out of logging avoids import cycles when services start annotating errors.
type Sentinels struct {
	InvalidInput    error
	Unauthenticated error
	Forbidden       error
}

type Classifier struct{ sentinels Sentinels }

func NewClassifier(s Sentinels) Classifier { return Classifier{sentinels: s} }

type Source string

const (
	SourceUnknown  Source = "unknown"
	SourceClient   Source = "client"
	SourceServer   Source = "server"
	SourceShutdown Source = "shutdown"
)

type Kind uint8

const (
	Operation Kind = iota
	HealthCheck
	LongPollTimeout
	Startup
	ShutdownTimeout
)

type Options struct {
	Kind   Kind
	Source Source
}

// Diagnosis is an allowlisted error representation. It deliberately carries no
// error message, pq.Detail, query, payload or request headers. Log it via slog.Any.
// Fields are private so arbitrary user values cannot be serialized as labels.
type Diagnosis struct {
	level        slog.Level
	outcome      string
	class        string
	errorType    string
	sqlstate     string
	source       Source
	operation    string
	stage        string
	contextState string
	contexts     []diagnosticContext
	causes       []Diagnosis
}

type diagnosticContext struct{ operation, stage string }

func (d Diagnosis) Level() slog.Level { return d.level }

func (d Diagnosis) LogValue() slog.Value {
	attrs := []slog.Attr{slog.String("outcome", d.outcome)}
	for _, field := range []struct{ name, value string }{
		{"error_class", d.class}, {"error_type", d.errorType},
		{"sqlstate", d.sqlstate}, {"source", string(d.source)},
		{"operation", d.operation}, {"stage", d.stage},
		{"context_state", d.contextState},
	} {
		if field.value != "" {
			attrs = append(attrs, slog.String(field.name, field.value))
		}
	}
	if len(d.contexts) != 0 {
		frames := make([]map[string]string, len(d.contexts))
		for i, frame := range d.contexts {
			frames[i] = make(map[string]string)
			if frame.operation != "" {
				frames[i]["operation"] = frame.operation
			}
			if frame.stage != "" {
				frames[i]["stage"] = frame.stage
			}
		}
		attrs = append(attrs, slog.Any("contexts", frames))
	}
	if len(d.causes) > 1 {
		causes := make([]map[string]any, len(d.causes))
		for i, cause := range d.causes {
			causes[i] = map[string]any{"level": cause.level.String()}
			for _, attr := range cause.LogValue().Group() {
				causes[i][attr.Key] = attr.Value.Any()
			}
		}
		attrs = append(attrs, slog.Any("causes", causes))
	}
	return slog.GroupValue(attrs...)
}

// Classify considers the actual cause first. A canceled context alone never
// turns an unrelated database/internal failure into an expected cancellation.
// For joined errors the most severe cause wins (first cause breaks ties),
// and all leaf diagnoses remain available in the serialized causes field.
// The original error stays with the caller; this function neither logs nor maps it.
func (c Classifier) Classify(ctx context.Context, err error, opts Options) Diagnosis {
	if err == nil {
		level := slog.LevelInfo
		if opts.Kind == HealthCheck || opts.Kind == LongPollTimeout {
			level = slog.LevelDebug
		}
		if opts.Kind == ShutdownTimeout {
			return Diagnosis{level: slog.LevelWarn, outcome: "timeout", class: "shutdown_timeout"}
		}
		return Diagnosis{level: level, outcome: "ok"}
	}
	d := c.cause(ctx, err, opts)
	if opts.Kind == Startup {
		d.level = slog.LevelError
	}
	if opts.Kind == HealthCheck && d.class != "canceled" {
		d.level = slog.LevelWarn
	}
	if opts.Kind == ShutdownTimeout && d.class == "deadline_exceeded" {
		d.level = slog.LevelWarn
		d.class, d.outcome = "shutdown_timeout", "timeout"
	}
	return d
}

func (c Classifier) cause(ctx context.Context, err error, opts Options) Diagnosis {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var causes []Diagnosis
		for _, child := range joined.Unwrap() {
			if child == nil {
				continue
			}
			d := c.cause(ctx, child, opts)
			if len(d.causes) != 0 {
				causes = append(causes, d.causes...)
			} else {
				causes = append(causes, d)
			}
		}
		if len(causes) != 0 {
			best := causes[0]
			for _, d := range causes[1:] {
				if d.level > best.level {
					best = d
				}
			}
			best.causes = causes
			return best
		}
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok && wrapped.Unwrap() != nil {
		d := c.cause(ctx, wrapped.Unwrap(), opts)
		if meta, ok := err.(*contextError); ok {
			d = withContext(d, meta)
		}
		return d
	}
	d := Diagnosis{level: slog.LevelError, outcome: "failed", class: "internal", errorType: fmt.Sprintf("%T", err)}
	for _, rule := range []struct {
		target error
		class  string
	}{
		{c.sentinels.InvalidInput, "invalid_input"},
		{c.sentinels.Unauthenticated, "unauthenticated"},
		{c.sentinels.Forbidden, "forbidden"},
		{sql.ErrNoRows, "not_found"},
	} {
		if rule.target != nil && errors.Is(err, rule.target) {
			d.level, d.outcome, d.class = slog.LevelInfo, "rejected", rule.class
			return d
		}
	}
	if errors.Is(err, context.Canceled) {
		return cancellation(d, false, opts.Source)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return cancellation(d, true, opts.Source)
	}
	var pg *pq.Error
	if errors.As(err, &pg) {
		d.class, d.sqlstate = "database", safeSQLState(pg.Code)
		switch pg.Code {
		case "23505":
			d.level, d.outcome, d.class = slog.LevelInfo, "rejected", "conflict"
		case "23503", "22P02", "22001", "23502":
			d.level, d.outcome, d.class = slog.LevelInfo, "rejected", "invalid_input"
		case "57014":
			// Neither ctx.Err nor Source proves that this particular SQL
			// failure was caused by the request cancellation. Keep it visible.
			if errors.Is(ctx.Err(), context.Canceled) {
				d.contextState = "canceled"
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				d.contextState = "deadline_exceeded"
			}
			d.class = "query_canceled"
		}
		return d
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Canceled:
			return cancellation(d, false, opts.Source)
		case codes.DeadlineExceeded:
			return cancellation(d, true, opts.Source)
		case codes.InvalidArgument, codes.OutOfRange:
			d.class = "invalid_input"
		case codes.NotFound:
			d.class = "not_found"
		case codes.AlreadyExists:
			d.class = "conflict"
		case codes.Unauthenticated:
			d.class = "unauthenticated"
		case codes.PermissionDenied:
			d.class = "forbidden"
		case codes.ResourceExhausted:
			d.class = "resource_exhausted"
		default:
			d.class = "rpc"
			return d
		}
		d.level, d.outcome = slog.LevelInfo, "rejected"
	}
	return d
}

func withContext(d Diagnosis, meta *contextError) Diagnosis {
	// Keep the existing independent fallback semantics for summary fields.
	// Frames preserve the actual pairs when those summaries span two scopes.
	if d.operation == "" {
		d.operation = meta.operation
	}
	if d.stage == "" {
		d.stage = meta.stage
	}
	if meta.operation != "" || meta.stage != "" {
		frames := append([]diagnosticContext(nil), d.contexts...)
		d.contexts = append(frames, diagnosticContext{meta.operation, meta.stage})
	}
	for i := range d.causes {
		d.causes[i] = withContext(d.causes[i], meta)
	}
	return d
}

func cancellation(d Diagnosis, deadline bool, source Source) Diagnosis {
	switch source {
	case SourceClient, SourceServer, SourceShutdown:
		d.source = source
	default:
		d.source = SourceUnknown
	}
	d.level, d.outcome, d.class = slog.LevelDebug, "canceled", "canceled"
	if deadline {
		d.level, d.outcome, d.class = slog.LevelWarn, "deadline_exceeded", "deadline_exceeded"
		if d.source == SourceClient {
			d.level = slog.LevelInfo
		}
	}
	return d
}

func safeSQLState(code pq.ErrorCode) string {
	if len(code) != 5 {
		return "unknown"
	}
	for _, ch := range code {
		if !(ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z') {
			return "unknown"
		}
	}
	return string(code)
}
