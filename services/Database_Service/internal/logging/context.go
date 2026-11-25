package logging

// contextError attaches static diagnostic labels without changing public errors.
type contextError struct {
	cause     error
	operation string
	stage     string
}

func (e *contextError) Error() string { return e.cause.Error() }
func (e *contextError) Unwrap() error { return e.cause }

// Annotate preserves Error(), errors.Is and errors.As. Labels must be static
// operation/stage names, never user data, URLs, SQL, credentials or error text.
// Invalid labels are replaced with "unknown"; empty labels are omitted.
func Annotate(err error, operation, stage string) error {
	if err == nil {
		return nil
	}
	return &contextError{cause: err, operation: label(operation), stage: label(stage)}
}

func label(value string) string {
	if len(value) > 64 {
		return "unknown"
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return "unknown"
		}
	}
	return value
}
