// Package observability holds the small ports for the
// cross-cutting concerns each rule engine adapter is expected to
// emit -- structured logs first, metrics later.
//
// The interfaces are intentionally tiny so callers can adapt their
// own logger (zerolog, zap, log/slog when 1.21 arrives) by writing
// one shim type. The library never reaches for a logging framework.
package observability

// Logger receives structured log events from an engine adapter.
// Implementations should be safe for concurrent use by multiple
// goroutines.
//
// Fields are key/value pairs. By convention keys are lower-snake
// case and values are concrete types (string, int, bool, error).
// An adapter that does not want to log anything passes a NopLogger.
type Logger interface {
	Info(msg string, fields ...Field)
	Error(msg string, fields ...Field)
}

// Field is one key/value pair attached to a log line.
type Field struct {
	Key   string
	Value interface{}
}

// String constructs a string-typed Field.
func String(key, value string) Field { return Field{Key: key, Value: value} }

// Int constructs an int-typed Field.
func Int(key string, value int) Field { return Field{Key: key, Value: value} }

// Bool constructs a bool-typed Field.
func Bool(key string, value bool) Field { return Field{Key: key, Value: value} }

// Err constructs an error-typed Field. The key is fixed to "err"
// so structured-log consumers can index it consistently.
func Err(value error) Field { return Field{Key: "err", Value: value} }

// NopLogger is the default Logger used when none is supplied. It
// discards every message; safe for concurrent use.
type NopLogger struct{}

// Info on a NopLogger discards the message.
func (NopLogger) Info(string, ...Field) {}

// Error on a NopLogger discards the message.
func (NopLogger) Error(string, ...Field) {}
